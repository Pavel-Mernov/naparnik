package store

import (
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	return New(d, today)
}

// BR-73: событие пишется в журнал; BR-51: тексты уведомлений без содержания вопроса.
func TestEmitRules(t *testing.T) {
	s := newTestStore(t)
	Now = func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, MSK) }
	defer func() { Now = time.Now }()

	s.Emit(EvQuestionSent, "u-sokolov", "Q-1", map[string]string{"to": "u-orlov", "urgent": "true"})
	s.Emit(EvQuestionSent, "u-sokolov", "Q-2", map[string]string{"to": "u-orlov", "urgent": "false"})
	s.Emit(EvQuestionAnswered, "u-orlov", "Q-1", map[string]string{"asker": "u-sokolov", "page": "kb-20"})
	s.Emit(EvRequestCreated, "u-sokolov", "ABS-1042", map[string]string{"approver": "u-kovalev", "employee": "u-sokolov"})
	s.Emit(EvRequestDecided, "u-kovalev", "ABS-1042", map[string]string{"employee": "u-sokolov", "status_ru": "согласовано"})
	s.Emit(EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "stale", "author": "u-orlov"})
	s.Emit(EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "helped", "author": "u-orlov"})
	s.Emit(EvQuestionSent, "u-orlov", "Q-3", map[string]string{"to": "u-orlov"}) // сам себе — без уведомления

	if len(s.Events()) != 8 {
		t.Fatalf("журнал: %d", len(s.Events()))
	}
	orlov := s.NotificationsFor("u-orlov")
	types := map[string]Notification{}
	for _, n := range orlov {
		types[n.Type] = n
	}
	if !types["urgent_question"].Urgent || types["urgent_question"].DeliverAt != "" {
		t.Fatalf("срочный — сразу: %+v", types["urgent_question"])
	}
	if types["question_new"].DeliverAt != "2026-11-11T16:00:00+03:00" {
		t.Fatalf("обычный — в сводку 16:00: %+v", types["question_new"])
	}
	if types["draft_review"].Type == "" || types["page_stale"].Type == "" || len(orlov) != 4 {
		t.Fatalf("эксперту: %+v", orlov)
	}
	sokolov := s.NotificationsFor("u-sokolov")
	if len(sokolov) != 2 || sokolov[0].Type != "request_status" || sokolov[1].Type != "answer_ready" {
		t.Fatalf("стажёру: %+v", sokolov)
	}
	if s.NotificationsFor("u-kovalev")[0].Type != "approval_needed" {
		t.Fatal("руководителю — заявка ждёт решения")
	}
	for _, n := range append(orlov, sokolov...) {
		if n.TG != "pending" || n.Text == "" {
			t.Fatalf("в Telegram: %+v", n)
		}
	}
	// повтор с тем же ID не дублирует (напоминания)
	if !s.Notify(Notification{ID: "rem:1", UserID: "u-sokolov"}) || s.Notify(Notification{ID: "rem:1", UserID: "u-sokolov"}) {
		t.Fatal("дубль напоминания")
	}
	s.MarkRead("u-sokolov")
	for _, n := range s.NotificationsFor("u-sokolov") {
		if !n.Read {
			t.Fatal("прочитано")
		}
	}
	// очередь Telegram: срочное — сейчас, сводка — не раньше 16:00
	if p := s.PendingTelegram(Now()); len(p) != 6 {
		t.Fatalf("сейчас к отправке: %d", len(p))
	}
	if p := s.PendingTelegram(time.Date(2026, 11, 11, 16, 0, 1, 0, MSK)); len(p) != 7 {
		t.Fatalf("после 16:00: %d", len(p))
	}
	s.SetTelegramStatus([]string{orlov[0].ID}, "sent", "")
	if s.NotificationsFor("u-orlov")[0].TG != "sent" {
		t.Fatal("статус отправки")
	}
}

func TestTelegramLinks(t *testing.T) {
	s := newTestStore(t)
	s.LinkTelegram("u-sokolov", 42)
	if id, ok := s.TelegramChat("u-sokolov"); !ok || id != 42 {
		t.Fatal("привязка")
	}
	if u, ok := s.UserByTelegram(42); !ok || u != "u-sokolov" {
		t.Fatal("поиск по чату")
	}
	s.UnlinkTelegram("u-sokolov")
	if _, ok := s.TelegramChat("u-sokolov"); ok {
		t.Fatal("отвязка")
	}
	if _, ok := s.UserByTelegram(42); ok {
		t.Fatal("чат отвязан")
	}
}

// BR-20, BR-52: сводки по будням 11:00/16:00, рабочие часы пользователя.
func TestDigestAndWorkingHours(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, MSK) }
	cases := map[time.Time]time.Time{
		at(11, 9, 0): at(11, 11, 0), at(11, 12, 0): at(11, 16, 0), at(11, 17, 0): at(12, 11, 0), at(13, 17, 0): at(16, 11, 0), // пятница → понедельник
	}
	for now, want := range cases {
		if got := NextDigest(now); !got.Equal(want) {
			t.Errorf("сводка после %v: %v, ожидалось %v", now, got, want)
		}
	}
	work := map[time.Time]time.Time{
		at(11, 12, 0): at(11, 12, 0), at(11, 8, 0): at(11, 10, 0), at(11, 20, 0): at(12, 10, 0), at(14, 12, 0): at(16, 10, 0), // суббота
	}
	for now, want := range work {
		if got := NextWorkingTime("10:00-19:00", now); !got.Equal(want) {
			t.Errorf("рабочее время после %v: %v, ожидалось %v", now, got, want)
		}
	}
	if got := NextWorkingTime("кривые часы", at(11, 3, 0)); !got.Equal(at(11, 3, 0)) {
		t.Error("без часов — не задерживаем")
	}
	if _, _, ok := WorkingWindow("10:00-xx", at(11, 0, 0)); ok {
		t.Error("кривое время")
	}
}

// Сохранённое состояние переживает перезапуск: журнал, уведомления и привязки — в снимке.
func TestSnapshotKeepsEvents(t *testing.T) {
	s := newTestStore(t)
	s.Emit(EvRequestCreated, "u-sokolov", "ACC-1", map[string]string{"approver": "u-kovalev", "employee": "u-sokolov"})
	s.LinkTelegram("u-kovalev", 7)
	raw := s.snapshot()
	r := newTestStore(t)
	if err := r.restore(raw); err != nil {
		t.Fatal(err)
	}
	if len(r.Events()) != 1 || len(r.NotificationsFor("u-kovalev")) != 1 {
		t.Fatal("журнал и уведомления")
	}
	if _, ok := r.TelegramChat("u-kovalev"); !ok {
		t.Fatal("привязка")
	}
	old := raw
	old.TGLinks = nil
	if err := r.restore(old); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.TelegramChat("u-kovalev"); ok {
		t.Fatal("старое состояние без привязок")
	}
}
