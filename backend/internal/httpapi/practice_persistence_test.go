package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// SC-30/31: повтор на двух backend, перезапуск и приватность на настоящем PostgreSQL.
func TestPostgresPracticeRestartAndConcurrentAttempts(t *testing.T) {
	dsn := postgresSchema(t)
	_, h1 := persistentServer(t, dsn)
	version := preparePracticeTest(t, h1)
	trainingStage(t, h1)
	_, h2 := persistentServer(t, dsn)
	body := jsonBody(map[string]any{"answer": "PRIVATE-FIRST", "task_version": version})
	parallelCalls(t, []http.Handler{h1, h2}, practicePath+"attempts", "intern.sokolov", body, "same-attempt")
	_, h3 := persistentServer(t, dsn)
	if practiceItem(t, h3, "intern.sokolov")["attempts_count"] != float64(1) {
		t.Fatal("дубли попыток")
	}
	if code, r := attemptPractice(t, h3, version, goodPracticeAnswer, "second"); code != 200 {
		t.Fatal(code, r)
	}
	_, h4 := persistentServer(t, dsn)
	v := practiceItem(t, h4, "buddy.belova")
	if v["attempts"] != nil || v["attempts_count"] != float64(2) || strings.Contains(jsonBody(v), "PRIVATE-FIRST") {
		t.Fatal(v)
	}
}

// Существующая БД без практики и kb-35 дополняется только нужным источником, заявки сохраняются.
func TestPostgresPracticeBootstrapOldState(t *testing.T) {
	dsn := postgresSchema(t)
	d, e := mockdata.Load("../../../docs/04-mock-data")
	if e != nil {
		t.Fatal(e)
	}
	pages := d.KB.Pages[:0]
	for _, p := range d.KB.Pages {
		if p.ID != "kb-35" {
			pages = append(pages, p)
		}
	}
	d.KB.Pages = pages
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s, e := store.OpenPostgres(context.Background(), d, today, dsn)
	if e != nil {
		t.Fatal(e)
	}
	if e := s.Transaction(func(w *store.Store) error {
		w.AddRequest(&store.Request{ID: "KEEP", EmployeeID: "u-sokolov"})
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	_ = s.Close()
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e := db.Exec(`UPDATE naparnik_state SET payload=jsonb_set(payload-'practices','{version}','1'::jsonb)`); e != nil {
		t.Fatal(e)
	}
	srv, h := persistentServer(t, dsn)
	if code, r := call(t, h, "POST", "/api/practices/sync", "intern.sokolov", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	if len(srv.Store.Requests("")) != 1 {
		t.Fatal("заявки стёрты")
	}
	found := false
	for _, p := range srv.Store.Pages() {
		if p.ID == "kb-35" {
			found = true
		}
	}
	if !found {
		t.Fatal("нет источника для практики")
	}
}

type delayedPracticeProvider struct {
	delegate       llm.Provider
	ready, release chan struct{}
}

func (p delayedPracticeProvider) Start(system, context string, spec []llm.ToolSpec) llm.Session {
	return delayedPracticeSession{p.delegate.Start(system, context, spec), p.ready, p.release}
}

type delayedPracticeSession struct {
	llm.Session
	ready, release chan struct{}
}

func (s delayedPracticeSession) SendUser(ctx context.Context, text string) (llm.Step, error) {
	close(s.ready)
	select {
	case <-s.release:
		return s.Session.SendUser(ctx, text)
	case <-ctx.Done():
		return llm.Step{}, ctx.Err()
	}
}

// Стажёр пропустил практику, пока LLM проверяла ответ: старый результат больше не записывается.
func TestPracticeSkipDuringReview(t *testing.T) {
	s, h := newServer(t)
	version := preparePracticeTest(t, h)
	trainingStage(t, h)
	ready, release := make(chan struct{}), make(chan struct{})
	s.Agent = agent.New(s.Store, delayedPracticeProvider{llm.NewStub("stub_ok", s.Store.Today), ready, release}, 5*time.Second)
	done := make(chan int, 1)
	go func() {
		code, _ := keyedCall(h, "POST", practicePath+"attempts", "intern.sokolov", jsonBody(map[string]any{"answer": goodPracticeAnswer, "task_version": version}), "delayed")
		done <- code
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("проверка не запущена")
	}
	advancePracticeTest(t, h, "skip_all")
	close(release)
	if code := <-done; code != 422 {
		t.Fatal(code)
	}
	if practiceItem(t, h, "intern.sokolov")["attempts_count"] != float64(0) {
		t.Fatal("записана устаревшая попытка")
	}
}
