package httpapi

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/telegram"
	"github.com/worker4556/naparnik/backend/internal/tools"
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

// BR-10a: подсказка про директора видна только адресату-бадди, стажёру — нет.
func TestSeniorHintVisibility(t *testing.T) {
	srv, h := newServer(t)
	u, _ := srv.Store.UserByLogin("intern.sokolov")
	tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{"addressee_id": "u-belova",
		"text": "Планируют ли менять North Star раздела или добавлять метрику качества контактов?"})
	_, intern := call(t, h, "GET", "/api/questions", "intern.sokolov", "")
	_, buddy := call(t, h, "GET", "/api/questions", "buddy.belova", "")
	if intern["questions"].([]any)[0].(map[string]any)["hint"] != nil {
		t.Fatal("стажёр не видит подсказку")
	}
	if !strings.Contains(buddy["questions"].([]any)[0].(map[string]any)["hint"].(string), "Сергей Волков") {
		t.Fatal("бадди видит подсказку")
	}
}

// Встреча через API: слоты → назначение → «записи нет» → оставить открытым; владелец базы видит вопрос.
func TestMeetingAPI(t *testing.T) {
	srv, h := newServer(t)
	u, _ := srv.Store.UserByLogin("intern.sokolov")
	q := tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{"addressee_id": "u-smirnova",
		"text": "Можно ли смотреть результаты эксперимента по регионам?"}).Data.(*store.Question)
	if code, _ := call(t, h, "GET", "/api/questions/"+q.ID+"/meeting-slots", "expert.orlov", ""); code != 403 {
		t.Fatal("чужой вопрос")
	}
	_, slots := call(t, h, "GET", "/api/questions/"+q.ID+"/meeting-slots", "expert.smirnova", "")
	slot := slots["slots"].([]any)[0].(string)
	if code, _ := call(t, h, "POST", "/api/questions/"+q.ID+"/meeting", "expert.smirnova", `{"slot":"bad"}`); code != 422 {
		t.Fatal("кривой слот")
	}
	_, m := call(t, h, "POST", "/api/questions/"+q.ID+"/meeting", "expert.smirnova", `{"slot":"`+slot+`"}`)
	id := m["meeting"].(map[string]any)["id"].(string)
	_, mine := call(t, h, "GET", "/api/meetings", "intern.sokolov", "")
	if len(mine["meetings"].([]any)) != 1 {
		t.Fatal("встреча у стажёра")
	}
	if _, other := call(t, h, "GET", "/api/meetings", "expert.orlov", ""); len(other["meetings"].([]any)) != 0 {
		t.Fatal("чужие встречи не видны")
	}
	if code, _ := call(t, h, "POST", "/api/meetings/"+id+"/finish", "intern.sokolov", `{"recorded":false}`); code != 403 {
		t.Fatal("итог — организатор")
	}
	call(t, h, "POST", "/api/meetings/"+id+"/finish", "expert.smirnova", `{"recorded":false}`)
	if code, r := call(t, h, "POST", "/api/questions/"+q.ID+"/leave-open", "expert.smirnova", ""); code != 200 || r["status"] != "Open" {
		t.Fatalf("открыт: %v", r)
	}
	if code, _ := call(t, h, "POST", "/api/questions/"+q.ID+"/leave-open", "expert.smirnova", ""); code != 422 {
		t.Fatal("повтор")
	}
	_, lead := call(t, h, "GET", "/api/questions", "lead.kovalev", "")
	if len(lead["questions"].([]any)) != 1 || lead["questions"].([]any)[0].(map[string]any)["status"] != "Open" {
		t.Fatalf("владелец базы видит открытый вопрос: %v", lead)
	}
}

// SC-21: нагрузка бадди и экспертов; SC-22: поддержка базы; SC-08/09 через API.
func TestLoadMaintenanceRerouteAPI(t *testing.T) {
	srv, h := newServer(t)
	u, _ := srv.Store.UserByLogin("intern.sokolov")
	q := tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{"addressee_id": "u-lebedeva",
		"text": "Как посчитать retention по когортам?"}).Data.(*store.Question)
	if code, r := call(t, h, "POST", "/api/questions/"+q.ID+"/not-mine", "expert.lebedeva", `{"to":"u-orlov"}`); code != 200 || r["addressee"] != "Михаил Орлов" {
		t.Fatalf("не ко мне: %d %v", code, r)
	}
	if code, _ := call(t, h, "POST", "/api/questions/"+q.ID+"/redirect", "expert.orlov", `{"to":"u-smirnova"}`); code != 403 {
		t.Fatal("переадресует бадди")
	}
	if code, _ := call(t, h, "POST", "/api/questions/"+q.ID+"/redirect", "buddy.belova", `{"to":"u-smirnova"}`); code != 200 {
		t.Fatal("бадди переадресовал")
	}
	_, mine := call(t, h, "GET", "/api/questions", "intern.sokolov", "")
	if len(mine["questions"].([]any)[0].(map[string]any)["route"].([]any)) != 2 {
		t.Fatal("маршрут виден стажёру")
	}
	if code, _ := call(t, h, "GET", "/api/kb/maintenance", "intern.sokolov", ""); code != 403 {
		t.Fatal("поддержка базы — владельцу")
	}
	_, m := call(t, h, "GET", "/api/kb/maintenance", "lead.kovalev", "")
	if len(m["duplicates"].([]any)) != 2 || len(m["open_questions"].([]any)) != 1 {
		t.Fatalf("поддержка базы: %v", m)
	}
	if code, _ := call(t, h, "POST", "/api/kb/duplicates/dp-02", "lead.kovalev", `{"decision":"not_duplicate"}`); code != 200 {
		t.Fatal("не дубль")
	}
	if code, _ := call(t, h, "POST", "/api/kb/duplicates/dp-02", "lead.kovalev", `{"decision":"merge"}`); code != 422 {
		t.Fatal("повтор")
	}
	if code, _ := call(t, h, "POST", "/api/questions/q-seed-open/assign", "lead.kovalev", `{"to":"u-smirnova"}`); code != 200 {
		t.Fatal("назначить")
	}
	if code, _ := call(t, h, "POST", "/api/kb/drafts/kb-draft-01/remind", "lead.kovalev", ""); code != 200 {
		t.Fatal("напомнить")
	}
	if code, _ := call(t, h, "POST", "/api/kb/drafts/nope/remind", "lead.kovalev", ""); code != 404 {
		t.Fatal("нет черновика")
	}
	_, metrics := call(t, h, "GET", "/api/metrics", "lead.kovalev", "")
	load := map[string]map[string]any{}
	for _, r := range metrics["load"].([]any) {
		row := r.(map[string]any)
		load[row["id"].(string)] = row
	}
	if load["u-lebedeva"]["received"].(float64) != 1 || load["u-orlov"]["received"].(float64) != 1 || load["u-smirnova"]["received"].(float64) != 2 ||
		load["u-belova"]["role"] != "бадди" || load["u-kovalev"] != nil {
		t.Fatalf("нагрузка: %v", load)
	}
}

// SC-20: черновик плана — бадди правит, руководитель утверждает, стажёру черновик не виден.
func TestPlanDraftAPI(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "GET", "/api/plans/u-egorova/draft", "intern.egorova", ""); code != 403 {
		t.Fatal(code)
	}
	code, v := call(t, h, "GET", "/api/plans/u-egorova/draft", "buddy.belova", "")
	if code != 200 || v["status"] != "draft" || v["can_edit"] != true {
		t.Fatal(code, v)
	}
	code, v = call(t, h, "PATCH", "/api/plans/u-egorova/draft", "buddy.belova", `{"steps":[{"step_id":"s-14","removed":true}],"comment":"без A/B"}`)
	if code != 200 || v["comment"] != "без A/B" {
		t.Fatal(code, v)
	}
	if code, _ := call(t, h, "POST", "/api/plans/u-egorova/approve", "buddy.belova", ""); code != 403 {
		t.Fatal(code)
	}
	code, v = call(t, h, "POST", "/api/plans/u-egorova/approve", "lead.kovalev", "")
	if code != 200 || v["status"] != "approved" {
		t.Fatal(code, v)
	}
	if code, p := call(t, h, "GET", "/api/plans/u-egorova", "intern.egorova", ""); code != 200 || p["data"] == nil && p["steps"] == nil {
		t.Fatal(code, p)
	}
}
