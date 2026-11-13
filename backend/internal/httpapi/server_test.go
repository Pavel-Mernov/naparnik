package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func newServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s := store.New(d, today)
	srv := &Server{Agent: agent.New(s, llm.NewStub("stub_ok", today), time.Second), Store: s}
	return srv, srv.Handler()
}

func call(t *testing.T, h http.Handler, method, path, user, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
		if path == "/api/requests" && method == "POST" {
			req.Header.Set("Idempotency-Key", "test:"+body)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHealthAndAuth(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "GET", "/api/health", "", ""); code != 200 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "GET", "/api/me", "", ""); code != 401 {
		t.Fatal("без учётки — 401")
	}
	if code, me := call(t, h, "GET", "/api/me", "intern.sokolov", ""); code != 200 || me["name"] != "Даниил Соколов" {
		t.Fatal(me)
	}
}

// Путь «от запроса до результата»: чат → черновик → подтверждение → квитанция в заявках и в ленте руководителя (SC-14, SC-29).
func TestChatDraftConfirmReceipt(t *testing.T) {
	_, h := newServer(t)
	code, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Мне нужен отгул в пятницу"}`)
	drafts, _ := reply["drafts"].([]any)
	if code != 200 || len(drafts) != 1 {
		t.Fatalf("чат: %d %v", code, reply)
	}
	id := drafts[0].(map[string]any)["id"].(string)
	code, res := call(t, h, "POST", "/api/requests/"+id+"/confirm", "intern.sokolov", "")
	if code != 200 || res["receipt"].(map[string]any)["id"] != "ABS-1042" {
		t.Fatalf("подтверждение: %d %v", code, res)
	}
	for _, login := range []string{"intern.sokolov", "lead.kovalev"} {
		_, list := call(t, h, "GET", "/api/requests", login, "")
		if len(list["requests"].([]any)) != 1 {
			t.Fatalf("%s: квитанция не видна: %v", login, list)
		}
	}
	if _, list := call(t, h, "GET", "/api/requests", "buddy.belova", ""); len(list["requests"].([]any)) != 0 {
		t.Fatal("бадди заявки на отгул не видит")
	}
	if code, _ := call(t, h, "POST", "/api/requests/"+id+"/confirm", "intern.sokolov", ""); code != 200 {
		t.Fatal("повторное подтверждение")
	}
}

func TestChatValidation(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{}`); code != 422 {
		t.Fatal(code)
	}
}

func TestPlanAndSteps(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "GET", "/api/plans/u-sokolov", "buddy.belova", ""); code != 200 {
		t.Fatal("бадди видит план")
	}
	if code, _ := call(t, h, "GET", "/api/plans/u-sokolov", "expert.orlov", ""); code != 403 {
		t.Fatal("эксперт не видит план")
	}
	if code, _ := call(t, h, "PATCH", "/api/plan-steps/s-01", "intern.sokolov", `{}`); code != 200 {
		t.Fatal("стажёр отмечает шаг")
	}
	if code, _ := call(t, h, "PATCH", "/api/plan-steps/s-03", "buddy.belova", `{"intern_id":"u-sokolov"}`); code != 200 {
		t.Fatal("бадди отмечает шаг")
	}
	if code, _ := call(t, h, "PATCH", "/api/plan-steps/s-03", "lead.kovalev", `{"intern_id":"u-sokolov"}`); code != 403 {
		t.Fatal("руководитель шаги не отмечает")
	}
	if code, _ := call(t, h, "PATCH", "/api/plan-steps/s-99", "intern.sokolov", `{}`); code != 404 {
		t.Fatal("нет шага")
	}
}

// 02-ROLES, BR-86, SC-13: видимость вопросов.
func TestQuestionsVisibility(t *testing.T) {
	srv, h := newServer(t)
	ask := func(login, addressee string) {
		u, _ := srv.Store.UserByLogin(login)
		if r := tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{"text": "вопрос " + login, "addressee_id": addressee}); !r.OK {
			t.Fatal(r.Error)
		}
	}
	ask("intern.sokolov", "u-orlov")
	ask("intern.egorova", "u-orlov")
	_, b := call(t, h, "GET", "/api/questions", "buddy.belova", "")
	for _, q := range b["questions"].([]any) {
		q := q.(map[string]any)
		if q["asked_by"] == "u-sokolov" && q["text"] == nil {
			t.Error("у Даниила текст виден бадди")
		}
		if q["asked_by"] == "u-egorova" && (q["text"] != nil || q["text_hidden"] != true || q["addressee"] != "Михаил Орлов") {
			t.Errorf("SC-13: у Полины текст скрыт, адресат виден: %v", q)
		}
	}
	if _, o := call(t, h, "GET", "/api/questions", "expert.orlov", ""); len(o["questions"].([]any)) != 2 {
		t.Error("входящие эксперта")
	}
	_, l := call(t, h, "GET", "/api/questions", "lead.kovalev", "")
	if len(l["questions"].([]any)) != 0 || l["team_questions_count"].(float64) != 2 {
		t.Errorf("руководитель видит только количество: %v", l)
	}
	if _, s := call(t, h, "GET", "/api/questions", "expert.smirnova", ""); len(s["questions"].([]any)) != 0 {
		t.Error("чужие входящие не видны")
	}
}

func TestWriteResultUnknownCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeResult(rec, tools.Result{Error: &tools.Error{Code: "weird", Message: "x"}})
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestChatInternalError(t *testing.T) {
	srv, h := newServer(t)
	srv.Store.Data.Team.Users = append(srv.Store.Data.Team.Users, mockdata.User{ID: "u-ghost", Login: "ghost"})
	// пользователь есть для авторизации, но агент его не найдёт после удаления
	req := httptest.NewRequest("POST", "/api/chat/messages", strings.NewReader(`{"text":"x"}`))
	req.Header.Set("X-User", "ghost")
	rec := httptest.NewRecorder()
	srv.Agent.Store = store.New(&mockdata.Data{}, time.Now())
	h.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatal(rec.Code)
	}
}

func TestTodayUsersAndStatic(t *testing.T) {
	srv, _ := newServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>buddy</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.StaticDir = dir
	h := srv.Handler()
	if code, r := call(t, h, "GET", "/api/today", "intern.sokolov", ""); code != 200 || len(r["data"].(map[string]any)["steps"].([]any)) == 0 {
		t.Fatalf("today: %d %v", code, r)
	}
	if code, _ := call(t, h, "GET", "/api/today", "lead.kovalev", ""); code != 403 {
		t.Fatal("шаги на сегодня — только стажёру")
	}
	if _, r := call(t, h, "GET", "/api/users", "", ""); len(r["users"].([]any)) != len(srv.Store.Data.Team.Users) {
		t.Fatal("учётки для переключателя")
	}
	if _, me := call(t, h, "GET", "/api/me", "buddy.belova", ""); len(me["interns"].([]any)) != 2 {
		t.Fatalf("бадди видит двух стажёров: %v", me)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "buddy") || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("фронт не отдаётся или кэшируется: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

// Без STATIC_DIR backend — только API: на «/» ссылка на интерфейс, прочие пути — 404.
func TestRootLinksToFrontend(t *testing.T) {
	srv, _ := newServer(t)
	srv.FrontendURL = "http://demo.local:3000"
	h := srv.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `href="http://demo.local:3000"`) {
		t.Fatalf("корень: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))
	if rec.Code != 404 {
		t.Fatalf("прежний интерфейс больше не отдаётся: %d", rec.Code)
	}
	srv.FrontendURL = ""
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "http://localhost:3000") {
		t.Fatal("по умолчанию — localhost:3000")
	}
}

// BR-02, BR-40: в списке базы — только своё пространство.
func TestKBPages(t *testing.T) {
	srv, h := newServer(t)
	_, r := call(t, h, "GET", "/api/kb/pages", "intern.sokolov", "")
	pages := r["pages"].([]any)
	want := map[string]bool{}
	for _, p := range srv.Store.Data.KB.Pages {
		if p.Space == "space-analytics" && p.Status == "Published" {
			want[p.ID] = true
		}
	}
	if len(pages) != len(want) || len(r["folders"].([]any)) != 7 {
		t.Fatalf("страниц %d, папок %d", len(pages), len(r["folders"].([]any)))
	}
	for _, p := range pages {
		id := p.(map[string]any)["id"].(string)
		if !want[id] {
			t.Fatalf("лишняя или повторная страница: %s", id)
		}
		delete(want, id)
		if p := p.(map[string]any); p["id"] == "kb-30" && !strings.Contains(p["body"].(string), "партиции") {
			t.Fatal("полный текст страницы, а не только начало")
		}
		if p.(map[string]any)["id"] == "kb-R1" {
			t.Fatal("страница чужого пространства")
		}
	}
}

func TestChatReset(t *testing.T) {
	_, h := newServer(t)
	if code, r := call(t, h, "POST", "/api/chat/reset", "intern.sokolov", ""); code != 200 || r["ok"] != true {
		t.Fatal(code)
	}
}

// P2 через API: вопрос → ответ адресата → черновик у автора → «Ок» → страница в базе; видимость ответа (BR-86).
func TestAnswerFlowAPI(t *testing.T) {
	srv, h := newServer(t)
	_, initialKB := call(t, h, "GET", "/api/kb/pages", "intern.egorova", "")
	initialPages := len(initialKB["pages"].([]any))
	for _, login := range []string{"intern.sokolov", "intern.egorova"} {
		u, _ := srv.Store.UserByLogin(login)
		tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{"text": "Вопрос " + login, "addressee_id": "u-orlov"})
	}
	qs := srv.Store.Questions()
	if code, _ := call(t, h, "POST", "/api/questions/"+qs[0].ID+"/answer", "expert.smirnova", `{"text":"x"}`); code != 403 {
		t.Fatal("отвечает только адресат")
	}
	code, r := call(t, h, "POST", "/api/questions/"+qs[0].ID+"/answer", "expert.orlov", `{"text":"Ответ Михаила"}`)
	if code != 200 || r["draft"].(map[string]any)["status"] != "Draft" {
		t.Fatalf("ответ: %d %v", code, r)
	}
	call(t, h, "POST", "/api/questions/"+qs[1].ID+"/answer", "expert.orlov", `{"text":"Ответ Полине"}`)
	if _, d := call(t, h, "GET", "/api/kb/drafts", "expert.orlov", ""); len(d["drafts"].([]any)) != 2 {
		t.Fatalf("черновики: %v", d)
	}
	_, mine := call(t, h, "GET", "/api/questions", "intern.sokolov", "")
	if mine["questions"].([]any)[0].(map[string]any)["answer"] != "Ответ Михаила" {
		t.Fatal("стажёр видит ответ сразу")
	}
	_, buddy := call(t, h, "GET", "/api/questions", "buddy.belova", "")
	for _, q := range buddy["questions"].([]any) {
		q := q.(map[string]any)
		if q["asked_by"] == "u-egorova" && q["answer"] != nil {
			t.Fatal("BR-86: ответ скрыт вместе с текстом")
		}
	}
	pageID := r["draft"].(map[string]any)["id"].(string)
	if code, _ := call(t, h, "POST", "/api/kb/pages/"+pageID+"/approve", "buddy.belova", `{}`); code != 403 {
		t.Fatal("публикует только автор")
	}
	if code, p := call(t, h, "POST", "/api/kb/pages/"+pageID+"/approve", "expert.orlov", `{"title":"Заголовок"}`); code != 200 || p["page"].(map[string]any)["status"] != "Published" {
		t.Fatalf("публикация: %d %v", code, p)
	}
	_, kb := call(t, h, "GET", "/api/kb/pages", "intern.egorova", "")
	if len(kb["pages"].([]any)) != initialPages+1 {
		t.Fatal("опубликованная страница в базе, черновик — нет")
	}
}

// BR-42 и согласование: стажёр оформил сам → руководитель отклонил с причиной / выдал доступ → стажёр видит статус.
func TestSelfServiceAndDecisionAPI(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "POST", "/api/requests", "intern.sokolov", `не json`); code != 422 {
		t.Fatal("битое тело — 422")
	}
	code, r := call(t, h, "POST", "/api/requests", "intern.sokolov", `{"kind":"absence","type":"day_off","date":"2026-11-16"}`)
	if code != 200 || r["data"].(map[string]any)["source"] != "self" {
		t.Fatalf("сам: %d %v", code, r)
	}
	call(t, h, "POST", "/api/requests", "intern.sokolov", `{"kind":"access","system":"sys-redash","reason":"шаг s-04"}`)
	if code, _ := call(t, h, "POST", "/api/requests", "intern.sokolov", `{"kind":"access","system":"sys-dwh-pii"}`); code != 403 {
		t.Fatal("ПДн — 403")
	}
	if code, _ := call(t, h, "POST", "/api/requests/ABS-1042/decision", "intern.sokolov", `{"decision":"approve"}`); code != 403 {
		t.Fatal("решает только согласующий")
	}
	if code, _ := call(t, h, "POST", "/api/requests/ABS-1042/decision", "lead.kovalev", `{"decision":"reject"}`); code != 422 {
		t.Fatal("отказ без причины")
	}
	code, d := call(t, h, "POST", "/api/requests/ABS-1042/decision", "lead.kovalev", `{"decision":"reject","reason":"DATE_CONFLICT","comment":" релиз "}`)
	if code != 200 || d["request"].(map[string]any)["decision"] != "релиз" {
		t.Fatalf("отказ: %d %v", code, d)
	}
	call(t, h, "POST", "/api/requests/ACC-2002/decision", "lead.kovalev", `{"decision":"approve"}`)
	_, mine := call(t, h, "GET", "/api/requests", "intern.sokolov", "")
	st := map[string]any{}
	for _, x := range mine["requests"].([]any) {
		st[x.(map[string]any)["id"].(string)] = x.(map[string]any)["status"]
	}
	if st["ABS-1042"] != "rejected" || st["ACC-2002"] != "granted" {
		t.Fatalf("статусы у стажёра: %v", st)
	}
	_, cat := call(t, h, "GET", "/api/services", "intern.sokolov", "")
	if len(cat["absence"].(map[string]any)["types"].([]any)) != 3 {
		t.Fatalf("справочник: %v", cat)
	}
	_, pre := call(t, h, "GET", "/api/services/absence/precheck?type=day_off&date=2026-11-12", "intern.sokolov", "")
	if len(pre["data"].(map[string]any)["warnings"].([]any)) == 0 {
		t.Fatalf("предупреждения: %v", pre)
	}
}
