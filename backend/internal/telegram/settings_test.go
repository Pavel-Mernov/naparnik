package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// «Не беспокоить» (SCR-I8, BR-52): несрочное ждёт конца паузы, срочное приходит сразу.
func TestQuietHours(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	svc.cfg.IgnoreHours = true
	s.LinkTelegram("u-sokolov", 9)
	until := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	s.UpdateSettings("u-sokolov", func(e *store.SettingsEdit) { e.QuietUntil = until.Format(time.RFC3339) })
	s.Notify(store.Notification{ID: "a", UserID: "u-sokolov", Text: "ответ на вопрос", TG: "pending"})
	s.Notify(store.Notification{ID: "b", UserID: "u-sokolov", Text: "срочно", TG: "pending", Urgent: true})
	if err := svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0]["text"].(string), "срочно") {
		t.Fatalf("при паузе — только срочное: %+v", api.sent)
	}
	for _, n := range s.NotificationsFor("u-sokolov") {
		if n.ID == "a" && (n.TG != "pending" || n.DeliverAt != until.Format(time.RFC3339)) {
			t.Fatalf("несрочное ждёт конца паузы: %+v", n)
		}
	}
	// Пауза снята — уведомление уходит со следующей отправкой.
	s.UpdateSettings("u-sokolov", func(e *store.SettingsEdit) { e.QuietUntil = "" })
	s.UpdateSettings("u-sokolov", func(e *store.SettingsEdit) {})
	if !s.QuietUntil("u-sokolov", time.Now()).IsZero() {
		t.Fatal("пауза снята")
	}
}

// Настройки из экрана меняют расписание: Даниил выключил утренний план и напоминания.
func TestScheduleFollowsSettings(t *testing.T) {
	svc, s := newService(t, nil, "log")
	svc.now = func() time.Time { return at(14, 10) }
	s.UpdateSettings("u-sokolov", func(e *store.SettingsEdit) { e.Proactivity = "off"; e.Reminders = []string{} })
	if err := svc.Schedule(); err != nil {
		t.Fatal(err)
	}
	if n := s.NotificationsFor("u-sokolov"); len(n) != 0 {
		t.Fatalf("утренний план и напоминания выключены: %+v", n)
	}
}
