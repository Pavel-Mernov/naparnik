package httpapi

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/telegram"
)

// Сквозной путь: действие → событие в журнале → уведомление в центре → метрики у руководителя.
func TestNotificationsAndMetricsAPI(t *testing.T) {
	srv, h := newServer(t)
	kv := cache.NewMemory()
	srv.KV, srv.ChatLimit = kv, 2
	srv.TG = telegram.New(telegram.Config{Mode: "log", WebURL: "http://localhost:3000"}, srv.Store, kv)
	h = srv.Handler()

	// заявка стажёра → руководителю «ждёт решения»; решение → стажёру статус
	if code, _ := call(t, h, "POST", "/api/requests", "intern.sokolov", `{"kind":"absence","type":"day_off","date":"2026-11-16"}`); code != 200 {
		t.Fatal(code)
	}
	_, lead := call(t, h, "GET", "/api/notifications", "lead.kovalev", "")
	list := lead["notifications"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["type"] != "approval_needed" || lead["unread"].(float64) != 1 {
		t.Fatalf("руководителю: %v", lead)
	}
	call(t, h, "POST", "/api/requests/ABS-1042/decision", "lead.kovalev", `{"decision":"approve"}`)
	_, me := call(t, h, "GET", "/api/notifications", "intern.sokolov", "")
	n := me["notifications"].([]any)[0].(map[string]any)
	if n["type"] != "request_status" || !strings.Contains(n["text"].(string), "согласовано") {
		t.Fatalf("стажёру: %v", n)
	}
	if me["telegram"].(map[string]any)["connected"] != false || me["telegram"].(map[string]any)["mode"] != "log" {
		t.Fatalf("telegram: %v", me["telegram"])
	}
	if code, _ := call(t, h, "POST", "/api/notifications/read", "intern.sokolov", ""); code != 200 {
		t.Fatal("прочитать")
	}
	if _, me = call(t, h, "GET", "/api/notifications", "intern.sokolov", ""); me["unread"].(float64) != 0 {
		t.Fatal("прочитано")
	}

	// привязка Telegram: код из веба, отвязка
	code, link := call(t, h, "POST", "/api/telegram/link", "intern.sokolov", "")
	if code != 200 || !strings.HasPrefix(link["code"].(string), "LINK-") || link["expires_in_min"].(float64) != 10 {
		t.Fatalf("код: %v", link)
	}
	srv.TG.Handle(t.Context(), 55, "/start "+link["code"].(string))
	if _, me = call(t, h, "GET", "/api/notifications", "intern.sokolov", ""); me["telegram"].(map[string]any)["connected"] != true {
		t.Fatal("подключён")
	}
	if code, _ := call(t, h, "DELETE", "/api/telegram/link", "intern.sokolov", ""); code != 200 {
		t.Fatal("отключить")
	}

	// отзыв: «устарело» → автору страницы (H11), кривое значение — 422
	if code, _ := call(t, h, "POST", "/api/feedback", "intern.sokolov", `{"page_id":"kb-03","value":"maybe"}`); code != 422 {
		t.Fatal("валидация")
	}
	call(t, h, "POST", "/api/feedback", "intern.sokolov", `{"page_id":"kb-03","value":"stale"}`)
	call(t, h, "POST", "/api/feedback", "intern.sokolov", `{"page_id":"kb-03","value":"helped"}`)
	_, orlov := call(t, h, "GET", "/api/notifications", "expert.orlov", "")
	if orlov["notifications"].([]any)[0].(map[string]any)["type"] != "page_stale" {
		t.Fatalf("автору: %v", orlov)
	}

	// чат: ответ из базы → событие; третий запрос в минуту — 429
	call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Что такое витрина?"}`)
	call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Что такое витрина?"}`)
	if code, r := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"ещё"}`); code != 429 || r["error"].(map[string]any)["code"] != "rate_limited" {
		t.Fatalf("лимит: %d %v", code, r)
	}

	// метрики — только руководителю
	if code, _ := call(t, h, "GET", "/api/metrics", "intern.sokolov", ""); code != 403 {
		t.Fatal("метрики стажёру")
	}
	_, m := call(t, h, "GET", "/api/metrics", "lead.kovalev", "")
	if m["requests_created"].(float64) != 1 || m["requests_decided"].(float64) != 1 || m["kb_answers"].(float64) < 1 || m["csat"].(float64) != 0.5 {
		t.Fatalf("метрики: %v", m)
	}
}

func TestTelegramLinkWithoutService(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "POST", "/api/telegram/link", "intern.sokolov", ""); code != 503 {
		t.Fatal("без Telegram на сервере — 503")
	}
	_, me := call(t, h, "GET", "/api/notifications", "intern.sokolov", "")
	if me["telegram"].(map[string]any)["mode"] != "off" {
		t.Fatal("режим off")
	}
}

// Медиана ожидания ответа и доля вопросов, закрытых базой (M-01, M-09).
func TestMetricsMath(t *testing.T) {
	ev := func(typ, obj, at string, meta map[string]string) store.LogEvent {
		return store.LogEvent{Type: typ, Object: obj, At: at, Meta: meta}
	}
	m := Metrics([]store.LogEvent{
		ev(store.EvQuestionSent, "Q-1", "2026-11-11T10:00:00+03:00", nil),
		ev(store.EvQuestionAnswered, "Q-1", "2026-11-11T10:30:00+03:00", nil),
		ev(store.EvKBAnswer, "kb-03", "2026-11-11T10:00:00+03:00", nil),
		ev(store.EvKBAnswer, "kb-04", "2026-11-11T10:00:00+03:00", nil),
		ev(store.EvKBAnswer, "kb-04", "2026-11-11T10:00:00+03:00", nil),
	})
	if m["median_answer_minutes"].(float64) != 30 || m["kb_share"].(float64) != 0.75 || m["csat"].(float64) != 0 {
		t.Fatalf("%v", m)
	}
}
