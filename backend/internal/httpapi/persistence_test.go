package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Каждый тест получает отдельную схему: данные соседних тестов и приложений не трогаем.
func postgresSchema(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: PostgreSQL integration")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := db.Exec("CREATE SCHEMA " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP SCHEMA " + name + " CASCADE"); _ = db.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	return u.String()
}

func persistentServer(t *testing.T, dsn string) (*Server, http.Handler) {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s, err := store.OpenPostgres(context.Background(), d, today, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := &Server{Store: s, Agent: agent.New(s, llm.NewStub("stub_ok", today), time.Second)}
	return srv, srv.Handler()
}

func keyedCall(h http.Handler, method, path, user, body, key string) (int, map[string]any) {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-User", user)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func parallelCalls(t *testing.T, handlers []http.Handler, path, user, body, key string) []map[string]any {
	t.Helper()
	const n = 12
	out := make([]map[string]any, n)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], out[i] = keyedCall(handlers[i%len(handlers)], "POST", path, user, body, key)
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != 200 {
			t.Fatalf("request %d: %d %v", i, code, out[i])
		}
	}
	return out
}

// SC-29: черновик и квитанция переживают новый Store/Agent. Два экземпляра
// backend получают одну квитанцию на 12 одновременных подтверждений.
func TestPostgresRestartAndConcurrentWrites(t *testing.T) {
	dsn := postgresSchema(t)
	s1, h1 := persistentServer(t, dsn)
	_, reply := call(t, h1, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Мне нужен отгул в пятницу"}`)
	draftID := reply["drafts"].([]any)[0].(map[string]any)["id"].(string)
	_ = s1.Store.Close()
	s2, h2 := persistentServer(t, dsn)
	_, h3 := persistentServer(t, dsn)
	_, drafts := call(t, h2, "GET", "/api/drafts", "intern.sokolov", "")
	if len(drafts["drafts"].([]any)) != 1 {
		t.Fatal("черновик потерян")
	}
	if code, _ := call(t, h2, "POST", "/api/drafts/"+draftID+"/confirm", "intern.egorova", ""); code != 404 {
		t.Fatal("чужой черновик")
	}
	results := parallelCalls(t, []http.Handler{h2, h3}, "/api/drafts/"+draftID+"/confirm", "intern.sokolov", "{}", "")
	id := results[0]["receipt"].(map[string]any)["id"]
	for _, r := range results {
		if r["receipt"].(map[string]any)["id"] != id {
			t.Fatal("разные квитанции")
		}
	}
	_, list := call(t, h2, "GET", "/api/requests", "intern.sokolov", "")
	if len(list["requests"].([]any)) != 1 {
		t.Fatal("дубликаты заявок")
	}
	// Форма сервиса: ключ сохраняется в БД и связан с телом и пользователем.
	body := `{"kind":"access","system":"sys-redash","reason":"шаг плана"}`
	results = parallelCalls(t, []http.Handler{h2, h3}, "/api/requests", "intern.sokolov", body, "form-1")
	accessID := results[0]["receipt"].(map[string]any)["id"].(string)
	if code, _ := keyedCall(h2, "POST", "/api/requests", "intern.sokolov", strings.ReplaceAll(body, "шаг плана", "другое"), "form-1"); code != 422 {
		t.Fatal("ключ с другим телом")
	}
	if code, _ := keyedCall(h2, "POST", "/api/requests", "intern.sokolov", body, ""); code != 422 {
		t.Fatal("форма без ключа")
	}
	parallelCalls(t, []http.Handler{h2, h3}, "/api/requests/"+accessID+"/decision", "lead.kovalev", `{"decision":"approve"}`, "")
	if code, _ := call(t, h2, "POST", "/api/requests/"+accessID+"/decision", "lead.kovalev", `{"decision":"reject","reason":"OTHER","comment":"x"}`); code != 422 {
		t.Fatal("противоречащее решение")
	}
	call(t, h2, "PATCH", "/api/plan-steps/s-01", "intern.sokolov", "{}")
	_ = s2.Store.Close()
	_, h4 := persistentServer(t, dsn)
	if code, r := keyedCall(h4, "POST", "/api/requests", "intern.sokolov", body, "form-1"); code != 200 || r["receipt"].(map[string]any)["id"] != accessID {
		t.Fatal("результат формы потерян")
	}
	_, list = call(t, h4, "GET", "/api/requests", "intern.sokolov", "")
	if len(list["requests"].([]any)) != 2 {
		t.Fatal("данные потеряны или продублированы")
	}
	for _, raw := range list["requests"].([]any) {
		r := raw.(map[string]any)
		if r["id"] == accessID && (r["status"] != "granted" || len(r["history"].([]any)) != 4) {
			t.Fatal("решение/история продублированы")
		}
	}
	_, plan := call(t, h4, "GET", "/api/plans/u-sokolov", "intern.sokolov", "")
	for _, raw := range plan["data"].(map[string]any)["steps"].([]any) {
		st := raw.(map[string]any)
		if st["id"] == "s-01" && st["status"] != "Done" {
			t.Fatal("прогресс потерян")
		}
	}
}

// SC-04 → SC-12: повтор ответа и публикации не создаёт новые страницы,
// результат виден другому экземпляру и после перезапуска.
func TestPostgresAnswerAndPublish(t *testing.T) {
	dsn := postgresSchema(t)
	_, h1 := persistentServer(t, dsn)
	_, r := call(t, h1, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Где взять шаблон презентации для экватора?"}`)
	d := r["drafts"].([]any)[0].(map[string]any)["id"].(string)
	_, receipt := call(t, h1, "POST", "/api/drafts/"+d+"/confirm", "intern.sokolov", "{}")
	qid := receipt["receipt"].(map[string]any)["id"].(string)
	_, h2 := persistentServer(t, dsn)
	results := parallelCalls(t, []http.Handler{h1, h2}, "/api/questions/"+qid+"/answer", "buddy.belova", `{"text":"Шаблон присылает куратор за месяц до экватора."}`, "")
	pageID := results[0]["draft"].(map[string]any)["id"].(string)
	for _, r := range results {
		if r["draft"].(map[string]any)["id"] != pageID {
			t.Fatal("лишние страницы")
		}
	}
	if code, _ := call(t, h2, "POST", "/api/questions/"+qid+"/answer", "buddy.belova", `{"text":"Другой ответ"}`); code != 422 {
		t.Fatal("перезаписан ответ")
	}
	if code, _ := call(t, h2, "POST", "/api/kb/pages/"+pageID+"/approve", "expert.orlov", "{}"); code != 403 {
		t.Fatal("публикация чужим автором")
	}
	parallelCalls(t, []http.Handler{h1, h2}, "/api/kb/pages/"+pageID+"/approve", "buddy.belova", "{}", "")
	_, h3 := persistentServer(t, dsn)
	_, pages := call(t, h3, "GET", "/api/kb/pages", "intern.egorova", "")
	if len(pages["pages"].([]any)) != 20 {
		t.Fatal("публикация потеряна или продублирована")
	}
}

// Ошибка COMMIT/записи не должна оставлять заявку только в памяти и не должна
// поглощать черновик. Проверяем реальным отказом записи PostgreSQL.
func TestPostgresWriteFailureCanBeRetried(t *testing.T) {
	dsn := postgresSchema(t)
	s, h := persistentServer(t, dsn)
	_, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Мне нужен отгул в пятницу"}`)
	id := reply["drafts"].([]any)[0].(map[string]any)["id"].(string)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE FUNCTION reject_write() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test write failure'; END $$;
		CREATE TRIGGER reject_write BEFORE UPDATE ON naparnik_state FOR EACH ROW EXECUTE FUNCTION reject_write()`)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", "{}"); code != 503 {
		t.Fatal("ошибка записи скрыта")
	}
	if len(s.Store.Requests("")) != 0 {
		t.Fatal("частичная запись в память")
	}
	if _, ok := s.Agent.Draft(id); !ok {
		t.Fatal("черновик потерян")
	}
	if _, err := db.Exec("DROP TRIGGER reject_write ON naparnik_state"); err != nil {
		t.Fatal(err)
	}
	if code, r := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", "{}"); code != 200 || r["receipt"].(map[string]any)["id"] != "ABS-1042" {
		t.Fatal("повтор не восстановился", code, r)
	}
}
