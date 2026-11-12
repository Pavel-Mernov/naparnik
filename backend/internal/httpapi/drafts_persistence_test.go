package httpapi

import (
	"database/sql"
	"net/http"
	"sync"
	"testing"
)

// Правка, версия, квитанция и отмена переживают перезапуск и два экземпляра backend.
func TestPostgresQuestionDraftLifecycle(t *testing.T) {
	dsn := postgresSchema(t)
	_, h1 := persistentServer(t, dsn)
	id := questionDraft(t, h1)
	_, h2 := persistentServer(t, dsn)
	text := "Нужен пример расчёта MDE для моего эксперимента."
	if code, r := call(t, h2, "PATCH", "/api/drafts/"+id, "intern.sokolov", jsonBody(map[string]any{"version": 1, "text": text, "urgent": true})); code != 200 {
		t.Fatal(code, r)
	}
	_, h3 := persistentServer(t, dsn)
	d := currentDraft(t, h3, id)
	if d["version"] != float64(2) || d["args"].(map[string]any)["text"] != text {
		t.Fatal(d)
	}
	if code, _ := call(t, h1, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":1}`); code != 409 {
		t.Fatal("старый процесс отправил незнакомую версию", code)
	}
	parallelCalls(t, []http.Handler{h1, h2, h3}, "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":2}`, "")
	srv4, h4 := persistentServer(t, dsn)
	if d := currentDraft(t, h4, id); d["status"] != "confirmed" || d["result"] == nil {
		t.Fatal(d)
	}
	qs := srv4.Store.Questions()
	if len(qs) != 1 || qs[0].Text != text || !qs[0].Urgent {
		t.Fatal(qs)
	}
	second := questionDraft(t, h4)
	if code, r := call(t, h1, "DELETE", "/api/drafts/"+second, "intern.sokolov", `{"version":1}`); code != 200 {
		t.Fatal(code, r)
	}
	srv5, h5 := persistentServer(t, dsn)
	if d := currentDraft(t, h5, second); d["status"] != "cancelled" {
		t.Fatal(d)
	}
	if code, _ := call(t, h5, "POST", "/api/drafts/"+second+"/confirm", "intern.sokolov", `{"version":2}`); code != 409 {
		t.Fatal(code)
	}
	if len(srv5.Store.Questions()) != 1 || len(srv5.Store.PendingDrafts("u-sokolov")) != 0 {
		t.Fatal("отмена потерялась")
	}
}

// Каждая гонка имеет ровно один исход: отправка либо правка/отмена, без частичного изменения.
func TestPostgresQuestionDraftRaces(t *testing.T) {
	dsn := postgresSchema(t)
	_, h1 := persistentServer(t, dsn)
	_, h2 := persistentServer(t, dsn)
	expectedQuestions := 0
	for _, method := range []string{"DELETE", "PATCH"} {
		for i := 0; i < 4; i++ {
			id := questionDraft(t, h1)
			body := `{"version":1}`
			if method == "PATCH" {
				body = `{"version":1,"text":"Уточнённый вопрос о длительности эксперимента"}`
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			codes := [2]int{}
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				codes[0], _ = keyedCall(h1, method, "/api/drafts/"+id, "intern.sokolov", body, "")
			}()
			go func() {
				defer wg.Done()
				<-start
				codes[1], _ = keyedCall(h2, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{"version":1}`, "")
			}()
			close(start)
			wg.Wait()
			d := currentDraft(t, h1, id)
			if codes[1] == 200 {
				expectedQuestions++
				if codes[0] != 409 || d["status"] != "confirmed" || d["version"] != float64(1) {
					t.Fatal(codes, d)
				}
			} else {
				if codes[0] != 200 || codes[1] != 409 || d["version"] != float64(2) {
					t.Fatal(codes, d)
				}
				want := "cancelled"
				if method == "PATCH" {
					want = "pending"
				}
				if d["status"] != want {
					t.Fatal(d)
				}
			}
		}
	}
	srv, _ := persistentServer(t, dsn)
	if len(srv.Store.Questions()) != expectedQuestions {
		t.Fatal("потеря или дубли вопросов", len(srv.Store.Questions()), expectedQuestions)
	}
}

// Старые черновики получают версию 1; новая запись использует формат 3, который старый backend отвергнет.
func TestPostgresLegacyDraftVersion(t *testing.T) {
	dsn := postgresSchema(t)
	_, h := persistentServer(t, dsn)
	id := questionDraft(t, h)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE naparnik_state SET payload=jsonb_set(payload #- ARRAY['drafts',$1,'version'],'{version}','2'::jsonb)`, id); err != nil {
		t.Fatal(err)
	}
	_, h2 := persistentServer(t, dsn)
	if d := currentDraft(t, h2, id); d["version"] != float64(1) || d["status"] != "pending" {
		t.Fatal(d)
	}
	if code, r := call(t, h2, "DELETE", "/api/drafts/"+id, "intern.sokolov", `{"version":1}`); code != 200 {
		t.Fatal(code, r)
	}
	var version int
	if err := db.QueryRow(`SELECT (payload->>'version')::int FROM naparnik_state`).Scan(&version); err != nil || version != 3 {
		t.Fatal(version, err)
	}
	_, h3 := persistentServer(t, dsn)
	if code, _ := call(t, h3, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", `{}`); code != 409 {
		t.Fatal("старое подтверждение после перезапуска", code)
	}
}
