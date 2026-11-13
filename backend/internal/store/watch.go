package store

import (
	"fmt"
	"time"
)

// Сторожа сроков: SLA ответа на вопрос (BR-18, SC-23) и простой стажёра (BR-85, SC-19).
// Вызываются планировщиком уведомлений внутри его транзакции; каждое уведомление — один раз
// (ключ включает момент отсчёта, так что после переадресации отсчёт начинается заново).

const (
	UrgentSLA = 2 * time.Hour // BR-18: срочный вопрос — 2 рабочих часа; обычный — 1 рабочий день
	IdleDays  = 2             // BR-85: 2 рабочих дня без активности
)

// AddWorking — момент, когда пройдёт d рабочего времени с from по рабочим часам «10:00-19:00» (будни, МСК).
func AddWorking(hours string, from time.Time, d time.Duration) time.Time {
	t := from.In(MSK)
	for i := 0; i < 60 && d > 0; i++ {
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, MSK)
		start, end, ok := WorkingWindow(hours, day)
		if !ok {
			return t.Add(d)
		}
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday && t.Before(end) {
			if t.Before(start) {
				t = start
			}
			if left := end.Sub(t); d <= left {
				return t.Add(d)
			} else {
				d -= left
			}
		}
		t = day.AddDate(0, 0, 1)
	}
	return t
}

// WorkingDay — длина рабочего дня по часам «10:00-19:00»; по умолчанию 8 часов.
func WorkingDay(hours string) time.Duration {
	if a, b, ok := WorkingWindow(hours, time.Now()); ok && b.After(a) {
		return b.Sub(a)
	}
	return 8 * time.Hour
}

// QuestionSLA — срок ответа текущего адресата: отсчёт от отправки или последней смены адресата.
func (s *Store) QuestionSLA(q Question) (since, due time.Time, ok bool) {
	for _, e := range s.Events() {
		if e.Object == q.ID && (e.Type == EvQuestionSent || e.Type == EvQuestionRerouted) {
			if t, err := time.Parse(time.RFC3339, e.At); err == nil && t.After(since) {
				since = t
			}
		}
	}
	if since.IsZero() {
		return since, due, false
	}
	to, _ := s.User(q.AddresseeID)
	hours := ""
	if to != nil {
		hours = to.WorkingHours
	}
	sla := WorkingDay(hours)
	if q.Urgent {
		sla = UrgentSLA
	}
	return since, AddWorking(hours, since, sla), true
}

// Watch проверяет сроки и ставит уведомления.
func (s *Store) Watch(now time.Time) {
	for _, q := range s.Questions() {
		if q.Status != "Sent" {
			continue
		}
		since, due, ok := s.QuestionSLA(q)
		if !ok || now.Before(due) {
			continue
		}
		key := q.ID + ":" + since.Format(time.RFC3339)
		asker, to := s.Name(q.AskedBy), s.Name(q.AddresseeID)
		kind := "рабочего дня"
		if q.Urgent {
			kind = "двух рабочих часов (срочный)"
		}
		// Без темы вопроса: уведомление уходит и в Telegram (BR-51), саму тему адресат видит во «Вопросах».
		if s.Notify(Notification{ID: "sla1:" + key, UserID: q.AddresseeID, Type: "sla_expired", View: "questions", TG: "pending", Urgent: q.Urgent,
			Text: fmt.Sprintf("⏳ Вопрос от %s ждёт ответа дольше %s.\nЧто сделать: открой его во «Вопросах» — ответь текстом, назначь встречу или нажми «Не ко мне», я найду другого.",
				asker, kind)}) {
			s.Emit(EvQuestionOverdue, "system", q.ID, map[string]string{"to": q.AddresseeID, "stage": "addressee"})
		}
		// Ещё через SLA — бадди стажёра с предложением переадресовать (если бадди сам не адресат).
		to2, _ := s.User(q.AddresseeID)
		hours := ""
		if to2 != nil {
			hours = to2.WorkingHours
		}
		buddy := s.UserBuddy(q.AskedBy)
		// Второй срок — ещё один такой же рабочий срок после первого (BR-18), а не календарная разница:
		// иначе ночи и выходные считались рабочими часами и бадди узнавал о вопросе на дни позже.
		sla := WorkingDay(hours)
		if q.Urgent {
			sla = UrgentSLA
		}
		if buddy == "" || buddy == q.AddresseeID || now.Before(AddWorking(hours, due, sla)) {
			continue
		}
		// Без темы: бадди мог не видеть текст вопросов стажёра (BR-86), а уведомление уходит и в Telegram (BR-51).
		if s.Notify(Notification{ID: "sla2:" + key, UserID: buddy, Type: "sla_expired", View: "questions", TG: "pending",
			Text: fmt.Sprintf("⏳ Вопрос %s · %s без ответа уже два срока.\nЧто сделать: открой «Вопросы стажёров» — переадресуй его другому эксперту или ответь сам, стажёр ждёт.",
				asker, to)}) {
			s.Emit(EvQuestionOverdue, "system", q.ID, map[string]string{"to": buddy, "stage": "buddy"})
		}
	}
	s.watchIdle(now)
	s.watchPlans(now)
}

// watchIdle — BR-85: 2 рабочих дня без активности по плану → мягкий сигнал бадди (только в вебе).
func (s *Store) watchIdle(now time.Time) {
	last := map[string]time.Time{}
	for _, e := range s.Events() {
		if t, err := time.Parse(time.RFC3339, e.At); err == nil && t.After(last[e.Actor]) {
			last[e.Actor] = t
		}
	}
	for _, p := range s.Data.Tracker.Plans {
		u, ok := s.User(p.InternID)
		if st, has := s.Plan(p.InternID); !ok || !has || st.Status == PlanDraft {
			continue // простой считаем только по активному плану
		}
		since := last[u.ID]
		if t, err := time.Parse(time.RFC3339, p.LastActivityAt); err == nil && t.After(since) {
			since = t
		}
		if since.IsZero() {
			if t, err := time.ParseInLocation(time.DateOnly, u.StartDate, MSK); err == nil {
				since = t
			}
		}
		buddy := s.UserBuddy(u.ID)
		if since.IsZero() || buddy == "" || now.Before(AddWorking(u.WorkingHours, since, IdleDays*WorkingDay(u.WorkingHours))) {
			continue
		}
		if s.Notify(Notification{ID: "idle:" + u.ID + ":" + since.Format(time.RFC3339), UserID: buddy, Type: "idle_signal", View: "team", TG: "web",
			Text: fmt.Sprintf("💤 %s два рабочих дня без активности по плану (последняя — %s).\nЧто сделать: напиши пару слов или загляни в план — может, где-то застрял.",
				u.Name, since.In(MSK).Format("02.01 15:04"))}) {
			s.Emit(EvInternIdle, "system", u.ID, map[string]string{"since": since.Format(time.RFC3339), "buddy": buddy})
		}
	}
}
