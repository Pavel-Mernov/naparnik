package store

import (
	"strings"
	"time"
)

// Правила «событие → уведомления». Тексты — из шаблонов docs/04-mock-data/notifications.json:
// только тип события и имена, без текста вопросов, ответов и страниц (BR-51), — их можно отправлять в Telegram.

// MSK — часовой пояс отдела в мок-данных (+03:00).
var MSK = time.FixedZone("MSK", 3*3600)

// DigestSlots — время сводок обычных вопросов (BR-20).
var DigestSlots = []int{11, 16}

func (s *Store) notificationsFor(e LogEvent) []Notification {
	m := e.Meta
	name := func(id string) string { return s.Name(id) }
	switch e.Type {
	case EvQuestionSent:
		to := m["to"]
		if to == "" || to == e.Actor {
			return nil
		}
		if m["urgent"] == "true" { // BR-21: срочный — сразу
			return []Notification{{UserID: to, Type: "urgent_question", Text: "Новый срочный вопрос · " + name(e.Actor) + ". Открыть →", View: "questions", Urgent: true, TG: "pending"}}
		}
		// BR-20: обычный — в веб сразу, в Telegram — сводкой в 11:00 или 16:00
		return []Notification{{UserID: to, Type: "question_new", Text: "Новый вопрос · " + name(e.Actor) + ". Открыть →", View: "questions", TG: "pending",
			DeliverAt: NextDigest(Now()).Format(time.RFC3339)}}
	case EvQuestionAnswered:
		var out []Notification
		if asker := m["asker"]; asker != "" {
			out = append(out, Notification{UserID: asker, Type: "answer_ready", Text: name(e.Actor) + " ответил(а) на твой вопрос. Открыть →", View: "questions", TG: "pending"})
		}
		if m["page"] != "" {
			out = append(out, Notification{UserID: e.Actor, Type: "draft_review", Text: "Черновик страницы по вашему ответу ждёт проверки. Открыть →", View: "questions", TG: "pending"})
		}
		return out
	case EvRequestCreated:
		if m["approver"] == "" {
			return nil
		}
		return []Notification{{UserID: m["approver"], Type: "approval_needed", Text: "Заявка " + e.Object + " ждёт вашего решения · " + name(m["employee"]) + ". Открыть →", View: "requests", TG: "pending"}}
	case EvRequestDecided:
		return []Notification{{UserID: m["employee"], Type: "request_status", Text: "Статус заявки " + e.Object + " изменился: " + m["status_ru"] + ". Открыть →", View: "requests", TG: "pending"}}
	case EvAnswerFeedback:
		// H11: «устарело» → владельцу страницы. Название страницы в Telegram не уходит.
		if m["value"] == "stale" && m["author"] != "" && m["author"] != e.Actor {
			return []Notification{{UserID: m["author"], Type: "page_stale", Text: "Одну из ваших страниц в базе отметили как устаревшую. Открыть →", View: "knowledge", TG: "pending"}}
		}
	}
	return nil
}

// NextDigest — ближайший слот сводки (11:00 / 16:00 МСК, по будням).
func NextDigest(now time.Time) time.Time {
	t := now.In(MSK)
	for d := 0; d < 8; d++ {
		day := time.Date(t.Year(), t.Month(), t.Day()+d, 0, 0, 0, 0, MSK)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for _, h := range DigestSlots {
			slot := day.Add(time.Duration(h) * time.Hour)
			if slot.After(t) {
				return slot
			}
		}
	}
	return t
}

// WorkingWindow — рабочие часы пользователя «10:00-19:00» в МСК на дату t.
func WorkingWindow(hours string, t time.Time) (time.Time, time.Time, bool) {
	from, to, ok := strings.Cut(hours, "-")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	parse := func(hm string) (time.Time, bool) {
		x, err := time.Parse("15:04", strings.TrimSpace(hm))
		if err != nil {
			return time.Time{}, false
		}
		d := t.In(MSK)
		return time.Date(d.Year(), d.Month(), d.Day(), x.Hour(), x.Minute(), 0, 0, MSK), true
	}
	a, ok1 := parse(from)
	b, ok2 := parse(to)
	return a, b, ok1 && ok2
}

// NextWorkingTime — когда можно беспокоить (BR-52): сейчас, если рабочее время, иначе начало следующего рабочего дня.
func NextWorkingTime(hours string, now time.Time) time.Time {
	t := now.In(MSK)
	for d := 0; d < 8; d++ {
		day := t.AddDate(0, 0, d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		start, end, ok := WorkingWindow(hours, day)
		if !ok {
			return t
		}
		if d == 0 && !t.Before(start) && t.Before(end) {
			return t
		}
		if start.After(t) {
			return start
		}
	}
	return t
}
