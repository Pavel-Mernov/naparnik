package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// fakeAPI — поддельный Bot API: запоминает отправленные сообщения.
type fakeAPI struct {
	mu   sync.Mutex
	sent []map[string]any
	fail bool
}

func (f *fakeAPI) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"naparnik_test_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.fail {
				_, _ = w.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
				return
			}
			f.sent = append(f.sent, m)
			_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":5,"message":{"text":"/stop","chat":{"id":99}}}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func newService(t *testing.T, api *httptest.Server, mode string) (*Service, *store.Store) {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s := store.New(d, today)
	base := ""
	if api != nil {
		base = api.URL
	}
	svc := New(Config{Token: "TEST", Mode: mode, WebURL: "http://localhost:3000", APIBase: base}, s, cache.NewMemory())
	return svc, s
}

func at(h, m int) time.Time { return time.Date(2026, 11, 11, h, m, 0, 0, store.MSK) }

// BR-50: код → /start в боте → чат привязан; код одноразовый; /stop отвязывает.
func TestLinkFlow(t *testing.T) {
	svc, s := newService(t, nil, "log")
	ctx := context.Background()
	if r := svc.Handle(ctx, 1, "/start"); !strings.Contains(r, "Подключить Telegram") {
		t.Fatal(r)
	}
	code, link, err := svc.LinkCode(ctx, "u-sokolov")
	if err != nil || !strings.HasPrefix(code, "LINK-") || link != "" {
		t.Fatalf("код: %q %q %v", code, link, err)
	}
	if r := svc.Handle(ctx, 77, "/start "+strings.ToLower(code)); !strings.Contains(r, "Готово, Даниил") {
		t.Fatal(r)
	}
	if id, ok := s.TelegramChat("u-sokolov"); !ok || id != 77 {
		t.Fatal("привязка")
	}
	if r := svc.Handle(ctx, 78, "/start "+code); !strings.Contains(r, "не подошёл") {
		t.Fatal("код одноразовый")
	}
	code2, _, _ := svc.LinkCode(ctx, "u-orlov")
	if r := svc.Handle(ctx, 80, code2); !strings.Contains(r, "Готово, Михаил") {
		t.Fatal("код без /start тоже принимается")
	}
	// BR-51: бот не отвечает на рабочие вопросы — только отправляет в веб
	if r := svc.Handle(ctx, 77, "Что такое витрина?"); !strings.Contains(r, "http://localhost:3000") {
		t.Fatal(r)
	}
	if r := svc.Handle(ctx, 77, "/stop"); !strings.Contains(r, "Отключил") {
		t.Fatal(r)
	}
	if r := svc.Handle(ctx, 77, "/stop"); !strings.Contains(r, "не подключён") {
		t.Fatal(r)
	}
	if svc.Mode() != "log" || svc.Bot() != "" {
		t.Fatal("режим")
	}
	if err := svc.Unlink("u-orlov"); err != nil {
		t.Fatal(err)
	}
}

// Доставка: неподключённым — пропуск; вне рабочих часов — откладываем; срочное — сразу; обычные вопросы — сводкой.
func TestFlush(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	ctx := context.Background()
	store.Now = func() time.Time { return at(12, 0) }
	defer func() { store.Now = time.Now }()
	s.LinkTelegram("u-orlov", 500) // рабочие часы 11:00-20:00

	s.Emit(store.EvQuestionSent, "u-sokolov", "Q-1", map[string]string{"to": "u-orlov", "urgent": "true"})
	s.Emit(store.EvQuestionSent, "u-sokolov", "Q-2", map[string]string{"to": "u-orlov"})
	s.Emit(store.EvQuestionSent, "u-egorova", "Q-3", map[string]string{"to": "u-orlov"})
	s.Emit(store.EvRequestDecided, "u-kovalev", "ABS-1", map[string]string{"employee": "u-sokolov", "status_ru": "согласовано"}) // не подключён

	svc.now = func() time.Time { return at(12, 0) }
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0]["text"].(string), "Срочный вопрос от стажёра · Даниил Соколов") {
		t.Fatalf("сразу только срочное: %+v", api.sent)
	}
	for _, x := range api.sent {
		if strings.Contains(x["text"].(string), "Q-") || x["chat_id"].(float64) != 500 {
			t.Fatalf("без содержания и в свой чат: %+v", x)
		}
	}
	if s.NotificationsFor("u-sokolov")[0].TG != "skipped" {
		t.Fatal("не подключён — пропуск, остаётся в вебе")
	}
	// 16:00 — одна сводка на два обычных вопроса (BR-20)
	svc.now = func() time.Time { return at(16, 1) }
	svc.kv = cache.NewMemory()
	_ = svc.Flush(ctx)
	if len(api.sent) != 2 || !strings.Contains(api.sent[1]["text"].(string), "новых вопросов от стажёров — 2") {
		t.Fatalf("сводка: %+v", api.sent)
	}
	// вне рабочих часов — откладываем до начала рабочего дня (BR-52)
	s.Emit(store.EvQuestionAnswered, "u-belova", "Q-9", map[string]string{"asker": "u-orlov"})
	svc.now = func() time.Time { return at(21, 0) }
	svc.kv = cache.NewMemory()
	_ = svc.Flush(ctx)
	if len(api.sent) != 2 {
		t.Fatal("ночью не беспокоим")
	}
	n := s.NotificationsFor("u-orlov")[0]
	if n.TG != "pending" || n.DeliverAt != "2026-11-12T11:00:00+03:00" {
		t.Fatalf("отложено до утра: %+v", n)
	}
	// демо-режим: всё сразу — и отложенное до утра, и сводки
	svc.cfg.IgnoreHours = true
	svc.kv = cache.NewMemory()
	_ = svc.Flush(ctx)
	if len(api.sent) != 3 || !strings.Contains(api.sent[2]["text"].(string), "Пришёл ответ на твой вопрос") {
		t.Fatalf("демо — сразу: %+v", api.sent)
	}
	// сбой Telegram — помечаем failed, веб не страдает (BR-53)
	api.fail = true
	s.Emit(store.EvRequestCreated, "u-sokolov", "ACC-1", map[string]string{"approver": "u-orlov", "employee": "u-sokolov"})
	svc.kv = cache.NewMemory()
	_ = svc.Flush(ctx)
	if s.NotificationsFor("u-orlov")[0].TG != "failed" {
		t.Fatal("сбой отправки")
	}
	// второй экземпляр не отправляет одновременно — блокировка
	if ok, _ := svc.kv.Lock(ctx, "tg:flush", time.Second); ok {
		t.Fatal("блокировка должна быть занята")
	}
}

// BR-83, BR-84: утренний план стажёру с режимом «план на день», напоминания за день и за час.
func TestSchedule(t *testing.T) {
	svc, s := newService(t, nil, "log")
	svc.now = func() time.Time { return at(14, 10) } // 1:1 в 15:00, мастер-класс завтра в 17:00
	if err := svc.Schedule(); err != nil {
		t.Fatal(err)
	}
	_ = svc.Schedule() // повтор не дублирует
	got := map[string]int{}
	for _, n := range s.NotificationsFor("u-sokolov") {
		got[n.Type]++
		if n.Type == "event_reminder" && !strings.Contains(n.Text, "1:1") && !strings.Contains(n.Text, "Мастер-класс") {
			t.Fatalf("напоминание: %s", n.Text)
		}
	}
	if got["morning_plan"] != 1 || got["event_reminder"] != 1 {
		t.Fatalf("Даниил: %v", got)
	}
	if len(s.NotificationsFor("u-egorova")) != 0 {
		t.Fatal("Полина выбрала «только дедлайны и встречи» и не участвует в событиях сегодня")
	}
	belova := s.NotificationsFor("u-belova")
	if len(belova) != 1 || !strings.HasPrefix(belova[0].Text, "⏰ Через час, в 15:00 — 1:1") || !strings.Contains(belova[0].Text, "где стажёр застрял") {
		t.Fatalf("бадди — за час по умолчанию: %+v", belova)
	}
	// за день: мастер-класс 12 ноября в 17:00 → напоминание 11-го после 17:00
	svc.now = func() time.Time { return at(17, 30) }
	_ = svc.Schedule()
	if n := s.NotificationsFor("u-sokolov")[0]; !strings.HasPrefix(n.Text, "📅 Завтра в 17:00 — Мастер-класс") || !strings.Contains(n.Text, "подготовь вопросы") {
		t.Fatalf("за день: %s", n.Text)
	}
	svc.now = func() time.Time { return at(8, 0) } // до рабочего дня — плана нет
	s2 := svc.store
	before := len(s2.NotificationsFor("u-sokolov"))
	_ = svc.Schedule()
	if len(s2.NotificationsFor("u-sokolov")) != before {
		t.Fatal("до рабочих часов план не присылаем")
	}
}

// Run: live-режим берёт имя бота, обрабатывает /stop и останавливается по контексту.
func TestRunLive(t *testing.T) {
	api := &fakeAPI{}
	srv := api.server(t)
	defer srv.Close()
	svc, s := newService(t, srv, "live")
	s.LinkTelegram("u-sokolov", 99)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { svc.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.TelegramChat("u-sokolov"); !ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if svc.Bot() != "naparnik_test_bot" {
		t.Fatal("имя бота")
	}
	if _, ok := s.TelegramChat("u-sokolov"); ok {
		t.Fatal("/stop из getUpdates отвязал чат")
	}
	code, link, _ := svc.LinkCode(context.Background(), "u-sokolov")
	if link != "https://t.me/naparnik_test_bot?start="+code {
		t.Fatal(link)
	}
}

func TestRunLogAndBadToken(t *testing.T) {
	svc, _ := newService(t, nil, "log")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.Run(ctx) // log: без сети, сразу выходит по контексту
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	}))
	defer bad.Close()
	svc2, _ := newService(t, bad, "live")
	svc2.Run(ctx) // неверный токен — веб продолжает работать
	if svc2.Bot() != "" {
		t.Fatal("бот не подключён")
	}
	if err := svc2.send(context.Background(), 1, "x"); err == nil {
		t.Fatal("ошибка отправки")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "")
	t.Setenv("token_tg", " abc ")
	t.Setenv("TG_MODE", "")
	t.Setenv("WEB_URL", "http://demo/")
	t.Setenv("NOTIFY_IGNORE_HOURS", "true")
	c := ConfigFromEnv()
	if c.Token != "abc" || c.Mode != "live" || c.WebURL != "http://demo" || !c.IgnoreHours {
		t.Fatalf("%+v", c)
	}
	t.Setenv("token_tg", "")
	t.Setenv("WEB_URL", "")
	if c := ConfigFromEnv(); c.Mode != "log" || c.WebURL != "http://localhost:3000" {
		t.Fatalf("%+v", c)
	}
}
