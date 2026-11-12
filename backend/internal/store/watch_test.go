package store

import (
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

func watchStore(t *testing.T) *Store {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	return New(d, today)
}

func at(s string) time.Time { t, _ := time.ParseInLocation("2006-01-02 15:04", s, MSK); return t }

func notes(s *Store, user, typ string) []Notification {
	var out []Notification
	for _, n := range s.NotificationsFor(user) {
		if n.Type == typ {
			out = append(out, n)
		}
	}
	return out
}

func TestAddWorking(t *testing.T) {
	cases := []struct {
		from, want string
		d          time.Duration
	}{
		{"2026-11-12 11:00", "2026-11-12 13:00", 2 * time.Hour},
		{"2026-11-12 17:30", "2026-11-13 10:30", 2 * time.Hour}, // остаток переносится на утро
		{"2026-11-13 18:00", "2026-11-16 11:00", 2 * time.Hour}, // пятница → понедельник
		{"2026-11-12 07:00", "2026-11-12 12:00", 2 * time.Hour}, // до начала дня — отсчёт с 10:00
		{"2026-11-12 11:00", "2026-11-13 11:00", 9 * time.Hour}, // рабочий день 10–19
	}
	for _, c := range cases {
		if got := AddWorking("10:00-19:00", at(c.from), c.d); !got.Equal(at(c.want)) {
			t.Errorf("%s + %v = %s, ждали %s", c.from, c.d, got.Format("2006-01-02 15:04"), c.want)
		}
	}
}

// SC-23, BR-18: не ответил за срок → напоминание адресату; ещё через срок → бадди с предложением переадресовать.
func TestQuestionSLA(t *testing.T) {
	defer func(old func() time.Time) { Now = old }(Now)
	s := watchStore(t)
	Now = func() time.Time { return at("2026-11-12 11:00") }
	q := &Question{ID: "Q-900", AskedBy: "u-sokolov", AddresseeID: "u-orlov", Title: "Расхождение конверсии", Status: "Sent"}
	s.AddQuestion(q)
	s.Emit(EvQuestionSent, "u-sokolov", q.ID, map[string]string{"to": "u-orlov"})
	orlov, _ := s.User("u-orlov")
	day := WorkingDay(orlov.WorkingHours)
	due := AddWorking(orlov.WorkingHours, at("2026-11-12 11:00"), day)

	s.Watch(due.Add(-time.Minute))
	if len(notes(s, "u-orlov", "sla_expired")) != 0 {
		t.Fatal("рано напоминать")
	}
	s.Watch(due)
	s.Watch(due.Add(time.Hour)) // повтор не дублирует
	n := notes(s, "u-orlov", "sla_expired")
	if len(n) != 1 || !strings.Contains(n[0].Text, "Расхождение конверсии") || n[0].TG != "pending" {
		t.Fatal(n)
	}
	if len(notes(s, "u-belova", "sla_expired")) != 0 {
		t.Fatal("бадди — только после второго срока")
	}
	s.Watch(AddWorking(orlov.WorkingHours, due, day))
	if b := notes(s, "u-belova", "sla_expired"); len(b) != 1 || !strings.Contains(b[0].Text, "переадресуй") {
		t.Fatal(b)
	}
	// Ответил — сторож молчит; переадресация начинает отсчёт заново.
	s.UpdateQuestion(q.ID, func(q *Question) { q.Status = "Answered" })
	before := len(s.NotificationsFor("u-orlov"))
	s.Watch(due.AddDate(0, 0, 7))
	if len(s.NotificationsFor("u-orlov")) != before {
		t.Fatal("отвеченный вопрос не напоминаем")
	}
}

func TestUrgentSLAIsTwoWorkingHours(t *testing.T) {
	defer func(old func() time.Time) { Now = old }(Now)
	s := watchStore(t)
	Now = func() time.Time { return at("2026-11-12 11:00") }
	q := &Question{ID: "Q-901", AskedBy: "u-sokolov", AddresseeID: "u-smirnova", Title: "Срочно: гардрейл", Status: "Sent", Urgent: true}
	s.AddQuestion(q)
	s.Emit(EvQuestionSent, "u-sokolov", q.ID, nil)
	_, due, _ := s.QuestionSLA(*q)
	smirnova, _ := s.User("u-smirnova")
	if want := AddWorking(smirnova.WorkingHours, at("2026-11-12 11:00"), 2*time.Hour); !due.Equal(want) {
		t.Fatal(due, want)
	}
	s.Watch(due)
	if n := notes(s, "u-smirnova", "sla_expired"); len(n) != 1 || !n[0].Urgent {
		t.Fatal(n)
	}
}

// SC-19, BR-85: 2 рабочих дня без активности → сигнал бадди только в вебе; активность сбрасывает отсчёт.
func TestIdleSignal(t *testing.T) {
	defer func(old func() time.Time) { Now = old }(Now)
	s := watchStore(t)
	// Последняя активность Даниила в трекере — 11.11 10:12 (четверг 12.11 — ещё рано).
	s.Watch(at("2026-11-12 18:00"))
	if len(notes(s, "u-belova", "idle_signal")) != 0 {
		t.Fatal("рано")
	}
	s.Watch(at("2026-11-13 10:30"))
	n := notes(s, "u-belova", "idle_signal")
	if len(n) != 1 || n[0].TG != "web" || !strings.Contains(n[0].Text, "Даниил Соколов") {
		t.Fatal(n)
	}
	// Даниил что-то сделал — новый отсчёт, старый сигнал не повторяется.
	Now = func() time.Time { return at("2026-11-13 11:00") }
	s.Emit(EvKBAnswer, "u-sokolov", "kb-02", nil)
	s.Watch(at("2026-11-16 18:00"))
	if len(notes(s, "u-belova", "idle_signal")) != 1 {
		t.Fatal("после активности сигнал не нужен")
	}
	s.Watch(at("2026-11-17 11:30"))
	if len(notes(s, "u-belova", "idle_signal")) != 2 {
		t.Fatal("новый простой — новый сигнал")
	}
}
