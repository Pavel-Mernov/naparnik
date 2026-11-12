package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/flags"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// При привязке накопленное не рассылается пачкой: одно сообщение со счётчиком и последним уведомлением.
func TestLinkSendsOneSummary(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	ctx := context.Background()
	for i, text := range []string{"📝 Заявка отправлена: доступ к Redash", "✅ Доступ к Redash — доступ выдан!\nЧто дальше: отметь шаг"} {
		s.Notify(store.Notification{ID: "n" + string(rune('0'+i)), UserID: "u-sokolov", Type: "request_status", Text: text, TG: "skipped"})
	}
	code, _, _ := svc.LinkCode(ctx, "u-sokolov")
	reply := svc.Handle(ctx, 77, "/start "+code)
	if !strings.Contains(reply, "накопилось уведомлений: 2") || !strings.Contains(reply, "доступ выдан!") || strings.Contains(reply, "Что дальше") {
		t.Fatal(reply)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 0 {
		t.Fatal("накопленное ушло пачкой", api.sent)
	}
}

// Одновременные несрочные — одним сообщением, срочное — отдельно.
func TestFlushBundles(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	svc.cfg.IgnoreHours = true
	s.LinkTelegram("u-sokolov", 9)
	for _, n := range []store.Notification{
		{ID: "a", UserID: "u-sokolov", Text: "первое", TG: "pending", At: "2026-11-12T10:00:00Z"},
		{ID: "b", UserID: "u-sokolov", Text: "второе\nдетали", TG: "pending", At: "2026-11-12T10:01:00Z"},
		{ID: "c", UserID: "u-sokolov", Text: "срочно", TG: "pending", Urgent: true},
	} {
		s.Notify(n)
	}
	if err := svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 2 {
		t.Fatal(len(api.sent), api.sent)
	}
	b := api.sent[1]["text"].(string)
	if !strings.Contains(b, "Новых уведомлений: 2") || strings.Index(b, "второе") > strings.Index(b, "первое") || strings.Contains(b, "детали") {
		t.Fatal(b)
	}
	var many []store.Notification
	for i := 0; i < 7; i++ {
		many = append(many, store.Notification{Text: "x"})
	}
	if !strings.Contains(bundle(many), "ещё 2") {
		t.Fatal(bundle(many))
	}
}

// Сбросили состояние — бот не «забывает» людей: привязка возвращается из Redis.
func TestRestoreLinks(t *testing.T) {
	kv := cache.NewMemory()
	svc, s := newService(t, nil, "log")
	svc.kv = kv
	ctx := context.Background()
	code, _, _ := svc.LinkCode(ctx, "u-sokolov")
	svc.Handle(ctx, 42, "/start "+code)
	s.LinkTelegram("u-kovalev", 7) // была только в состоянии — копируется в Redis

	fresh, s2 := newService(t, nil, "log") // новое состояние, тот же Redis
	fresh.kv = kv
	if n := fresh.Restore(ctx); n != 1 { // Даниил — из копии после /start; у Ковалёва копии ещё нет
		t.Fatal(n)
	}
	svc.Restore(ctx) // старый экземпляр докопировал Ковалёва
	if n := fresh.Restore(ctx); n != 1 {
		t.Fatal(n)
	}
	if chat, ok := s2.TelegramChat("u-sokolov"); !ok || chat != 42 {
		t.Fatal(chat, ok)
	}
	// /stop стирает и копию.
	_ = fresh.Unlink("u-sokolov")
	third, s3 := newService(t, nil, "log")
	third.kv = kv
	third.Restore(ctx)
	if _, ok := s3.TelegramChat("u-sokolov"); ok {
		t.Fatal("после /stop привязка не возвращается")
	}
}

// Флаг telegram=off: ничего не отправляется, уведомления ждут в вебе и уйдут после включения.
func TestTelegramFlagOff(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	svc.cfg.IgnoreHours = true
	s.LinkTelegram("u-sokolov", 9)
	s.Notify(store.Notification{ID: "x", UserID: "u-sokolov", Text: "привет", TG: "pending"})
	t.Setenv("FEATURE_FLAGS", "telegram=off")
	old := flags.Default
	flags.Default = flags.FromEnv()
	_ = svc.Flush(context.Background())
	if len(api.sent) != 0 {
		t.Fatal("выключено, но отправлено")
	}
	flags.Default = old
	_ = svc.Flush(context.Background())
	if len(api.sent) != 1 {
		t.Fatal("после включения — отправлено", len(api.sent))
	}
}
