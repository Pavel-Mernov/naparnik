package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// SC-33: API возвращает безопасные правила и свой шаг, проверяет учётку и роль.
func TestAccessRulesAPI(t *testing.T) {
	s, h := newServer(t)
	path := "/api/services/access/rules?system=sys-dwh-marts-ro"
	for _, tc := range []struct {
		login string
		code  int
	}{{"", 401}, {"buddy.belova", 403}, {"lead.kovalev", 403}, {"expert.orlov", 403}, {"intern.sokolov", 200}} {
		if code, _ := call(t, h, "GET", path, tc.login, ""); code != tc.code {
			t.Fatal(tc, code)
		}
	}
	_, body := call(t, h, "GET", path, "intern.sokolov", "")
	d := body["data"].(map[string]any)
	rule := d["systems"].([]any)[0].(map[string]any)
	if rule["week_from"] != float64(2) || rule["now"] != false || rule["step"].(map[string]any)["id"] != "s-10" || d["approver"].(map[string]any)["name"] != "Андрей Ковалёв" {
		t.Fatal(body)
	}
	for _, system := range []string{"sys-dwh-pii", "missing"} {
		want := 403
		if system == "missing" {
			want = 404
		}
		if code, _ := call(t, h, "GET", "/api/services/access/rules?system="+system, "intern.sokolov", ""); code != want {
			t.Fatal(system, code)
		}
	}
	_, body = call(t, h, "GET", "/api/services/access/rules", "intern.sokolov", "")
	if len(body["data"].(map[string]any)["systems"].([]any)) != 8 || len(s.Store.Requests("")) != 0 {
		t.Fatal(body)
	}
}

// SC-33 + BR-43: объяснение → выбор → черновик → подтверждение → обычная квитанция.
func TestAccessGuidanceToRequest(t *testing.T) {
	s, h := newServer(t)
	code, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Хочу сам посмотреть данные в витринах DWH, как туда попасть?"}`)
	if code != 200 || len(reply["drafts"].([]any)) != 0 || len(s.Store.Requests("")) != 0 {
		t.Fatal(code, reply)
	}
	text := reply["text"].(string)
	for _, part := range []string{"Витрины DWH отдела, только чтение", "со 2-й недели", "Андрей Ковалёв", "сделай сам", "сделаю за тебя", "тестовая инструкция"} {
		if !strings.Contains(text, part) {
			t.Fatal("не объяснено", part, reply)
		}
	}
	raw, _ := json.Marshal(reply)
	for _, secret := range []string{"kb-R1", "space-dwh-pii", "sys-dwh-pii", "Витрина клиентов с контактными данными"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("лишние метаданные", secret)
		}
	}
	_, reply = call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"сделай за меня"}`)
	drafts := reply["drafts"].([]any)
	if len(drafts) != 1 || len(s.Store.Requests("")) != 0 {
		t.Fatal(reply)
	}
	draft := drafts[0].(map[string]any)
	if draft["tool"] != "request_access" || draft["args"].(map[string]any)["system"] != "sys-dwh-marts-ro" {
		t.Fatal(draft)
	}
	path := "/api/drafts/" + draft["id"].(string) + "/confirm"
	if code, _ := call(t, h, "POST", path, "intern.egorova", "{}"); code != 404 {
		t.Fatal("чужое подтверждение", code)
	}
	for range 2 {
		code, receipt := call(t, h, "POST", path, "intern.sokolov", "{}")
		if code != 200 || receipt["receipt"].(map[string]any)["status"] != "pending" || len(s.Store.Requests("")) != 1 {
			t.Fatal(code, receipt)
		}
	}
	req := s.Store.Requests("")[0]
	if req.System != "sys-dwh-marts-ro" || req.StepID != "s-10" || req.ApproverID != "u-kovalev" {
		t.Fatal(req)
	}
	// Читаемое пояснение после заявки отражает pending, не предлагает ещё один черновик.
	_, reply = call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Нужен доступ к витринам DWH"}`)
	if len(reply["drafts"].([]any)) != 0 || !strings.Contains(reply["text"].(string), "уже на согласовании") {
		t.Fatal(reply)
	}
}

// SC-33: путь «сам» использует существующую форму и тот же сервис, будущая неделя не блокирует заявку.
func TestAccessGuidanceSelfService(t *testing.T) {
	s, h := newServer(t)
	call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Как попасть в витрины DWH?"}`)
	_, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Оформлю сам"}`)
	if len(reply["drafts"].([]any)) != 0 || !strings.Contains(reply["text"].(string), "Оформить самому") || len(s.Store.Requests("")) != 0 {
		t.Fatal(reply)
	}
	code, req := call(t, h, "POST", "/api/requests", "intern.sokolov", `{"kind":"access","system":"sys-dwh-marts-ro","reason":"Доступ по задаче"}`)
	if code != 200 || req["data"].(map[string]any)["step_id"] != "s-10" || len(s.Store.Requests("")) != 1 {
		t.Fatal(code, req)
	}
}
