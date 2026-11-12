package httpapi

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
)

func questionDraft(t *testing.T, h http.Handler) string {
	t.Helper()
	code, r := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Как выбрать длительность A/B-теста для низкого трафика?"}`)
	if code != 200 {
		t.Fatal(code, r)
	}
	drafts := r["drafts"].([]any)
	if len(drafts) != 1 {
		t.Fatal(r)
	}
	d := drafts[0].(map[string]any)
	if d["tool"] != "send_question" || d["version"] != float64(1) {
		t.Fatal(d)
	}
	return d["id"].(string)
}

func currentDraft(t *testing.T, h http.Handler, id string) map[string]any {
	t.Helper()
	code, r := call(t, h, "GET", "/api/drafts/"+id, "intern.sokolov", "")
	if code != 200 {
		t.Fatal(code, r)
	}
	return r["draft"].(map[string]any)
}

// BR-14, BR-43: исправленный текст и срочность отправляются только после нового подтверждения.
func TestQuestionDraftEditConfirm(t *testing.T) {
	srv, h := newServer(t)
	id := questionDraft(t, h)
	before := currentDraft(t, h, id)
	if code, r := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", jsonBody(map[string]any{"version": 1, "text": before["args"].(map[string]any)["text"], "urgent": false})); code != 200 || r["draft"].(map[string]any)["version"] != float64(1) {
		t.Fatal("неизменённый текст получил новую версию", code, r)
	}
	text := "Как исключить ботов из выборки эксперимента?\nПроверил фильтры и застрял."
	code, r := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", jsonBody(map[string]any{"version": 1, "text": text, "urgent": true}))
	if code != 200 {
		t.Fatal(code, r)
	}
	d := r["draft"].(map[string]any)
	args := d["args"].(map[string]any)
	if d["version"] != float64(2) || d["status"] != "pending" || args["text"] != text || args["urgent"] != true || args["title"] == before["args"].(map[string]any)["title"] {
		t.Fatal(d)
	}
	if args["addressee_id"] != before["args"].(map[string]any)["addressee_id"] || len(srv.Store.Questions()) != 0 {
		t.Fatal("правка изменила адресата или отправила вопрос")
	}
	// Все маршруты подтверждения используют ту же проверку версии.
	for _, path := range []string{"/api/drafts/", "/api/questions/", "/api/requests/"} {
		suffix := "/confirm"
		if path == "/api/questions/" {
			suffix = "/confirm-send"
		}
		for _, body := range []string{`{}`, `{"version":1}`} {
			if code, _ := call(t, h, "POST", path+id+suffix, "intern.sokolov", body); code != 409 {
				t.Fatal("устаревшее подтверждение", path, body, code)
			}
		}
	}
	if code, _ := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", `{"version":1,"urgent":false}`); code != 409 {
		t.Fatal("устаревшая правка")
	}
	code, sent := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":2}`)
	if code != 200 {
		t.Fatal(code, sent)
	}
	code, again := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":2}`)
	if code != 200 || !reflect.DeepEqual(sent, again) || len(srv.Store.Questions()) != 1 {
		t.Fatal("повтор отправки", code, again)
	}
	q := srv.Store.Questions()[0]
	if q.Text != text || !q.Urgent || q.Title != "Как исключить ботов из выборки эксперимента?" {
		t.Fatal(q)
	}
	if d := currentDraft(t, h, id); d["status"] != "confirmed" || d["result"] == nil {
		t.Fatal(d)
	}
	if code, _ := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", `{"version":2,"urgent":false}`); code != 409 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{"version":2}`); code != 409 {
		t.Fatal("отмена отправленного", code)
	}
}

// SC-05: отмена сохраняется, повтор безопасен; все старые способы подтверждения закрыты.
func TestDraftCancellation(t *testing.T) {
	srv, h := newServer(t)
	id := questionDraft(t, h)
	code, r := call(t, h, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{"version":1}`)
	if code != 200 || r["draft"].(map[string]any)["status"] != "cancelled" {
		t.Fatal(code, r)
	}
	code, again := call(t, h, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{"version":1}`)
	if code != 200 || !reflect.DeepEqual(r, again) {
		t.Fatal(code, again)
	}
	for _, body := range []string{`{}`, `{"version":1}`, `{"version":2}`} {
		if code, _ := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", body); code != 409 {
			t.Fatal("отменённый отправился", code)
		}
	}
	if code, _ := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", `{"version":2,"text":"Другой вопрос"}`); code != 409 {
		t.Fatal(code)
	}
	if _, ok := srv.Agent.Draft(id); ok {
		t.Fatal("отменённый доступен как ожидающий")
	}
	_, pending := call(t, h, "GET", "/api/drafts", "intern.sokolov", "")
	if len(pending["drafts"].([]any)) != 0 || len(srv.Store.Questions()) != 0 {
		t.Fatal(pending)
	}
	count := 0
	for _, e := range srv.Store.Events() {
		if e.Type == store.EvDraftCancelled {
			count++
		}
	}
	if count != 1 {
		t.Fatal("дубль события отмены", count)
	}
}

func TestDraftOwnershipAndValidation(t *testing.T) {
	srv, h := newServer(t)
	id := questionDraft(t, h)
	initial := currentDraft(t, h, id)
	for _, user := range []string{"intern.egorova", "buddy.belova", "lead.kovalev"} {
		for _, method := range []string{"GET", "PATCH", "DELETE"} {
			body := ""
			if method != "GET" {
				body = `{"version":1}`
			}
			if code, _ := call(t, h, method, "/api/drafts/"+id, user, body); code != 404 {
				t.Fatal(method, user, code)
			}
		}
	}
	for _, body := range []string{
		`{`, `null`, `{"version":0,"text":"вопрос"}`, `{"version":1}`, `{"version":1,"text":"  "}`,
		`{"version":1,"urgent":"true"}`, `{"version":1,"text":"Паспорт 4510 123456"}`,
		`{"version":1,"text":"Вопрос","addressee_id":"u-volkov"}`, `{"version":1,"text":"Вопрос","tool":"request_access"}`,
		`{"version":1,"urgent":true} {}`, jsonBody(map[string]any{"version": 1, "text": strings.Repeat("я", 8001)}),
		`{"version":1,"urgent":true}` + strings.Repeat(" ", 65536) + `{}`,
	} {
		if code, r := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", body); code != 422 {
			t.Fatal(code, r)
		}
		if !reflect.DeepEqual(initial, currentDraft(t, h, id)) {
			t.Fatal("отклонённая правка изменила черновик")
		}
	}
	for _, method := range []string{"PATCH", "DELETE"} {
		if code, _ := call(t, h, method, "/api/drafts/absent", "intern.sokolov", `{"version":1}`); code != 404 {
			t.Fatal(code)
		}
		if code, _ := call(t, h, method, "/api/drafts/"+id, "", `{"version":1}`); code != 401 {
			t.Fatal(code)
		}
	}
	if code, _ := call(t, h, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{}`); code != 422 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":0}`); code != 422 {
		t.Fatal(code)
	}
	// Проверка прав выполняется снова при правке, даже если черновик был создан раньше.
	u, _ := srv.Store.UserByLogin("intern.sokolov")
	roles := u.Roles
	u.Roles = []string{"expert"}
	defer func() { u.Roles = roles }()
	if code, _ := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", `{"version":1,"urgent":true}`); code != 403 {
		t.Fatal(code)
	}
}

func TestOtherActionDraftCancellation(t *testing.T) {
	srv, h := newServer(t)
	_, r := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Мне нужен отгул в пятницу"}`)
	id := r["drafts"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := call(t, h, "PATCH", "/api/drafts/"+id, "intern.sokolov", `{"version":1,"text":"другой текст"}`); code != 422 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{"version":1}`); code != 200 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{}`); code != 409 || len(srv.Store.Requests("")) != 0 {
		t.Fatal(code)
	}
}
