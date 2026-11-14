package httpapi

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// SCR-L2: реальный API, перезапуск, два экземпляра и конфликт устаревшей версии.
func TestPostgresPlanRevisionRestartAndRace(t *testing.T) {
	dsn := postgresSchema(t)
	_, a := persistentServer(t, dsn)
	code, v := call(t, a, "POST", "/api/plans/u-sokolov/revisions/start", "buddy.belova", `{"version":1}`)
	if code != 200 {
		t.Fatal(code, v)
	}
	_, b := persistentServer(t, dsn)
	code, v = call(t, b, "GET", "/api/plans/u-sokolov/draft", "buddy.belova", "")
	if code != 200 || v["review_status"] != "editing" {
		t.Fatal(code, v)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i, h := range []http.Handler{a, b} {
		wg.Add(1)
		go func(i int, h http.Handler) {
			defer wg.Done()
			code, _ := keyedCall(h, "PATCH", "/api/plans/u-sokolov/draft", "buddy.belova", fmt.Sprintf(`{"version":2,"comment":"Правка %d"}`, i), "")
			codes <- code
		}(i, h)
	}
	wg.Wait()
	close(codes)
	ok, conflicts := 0, 0
	for code := range codes {
		if code == 200 {
			ok++
		}
		if code == 409 {
			conflicts++
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatal(ok, conflicts)
	}
	for _, x := range []struct {
		user, path, body string
		want             int
	}{
		{"buddy.belova", "revisions/submit", `{"version":3}`, 200},
		{"lead.kovalev", "revisions/return", `{"version":4,"comment":"Перепроверь сроки"}`, 200},
		{"lead.kovalev", "approve", `{"version":5}`, 422},
		{"buddy.belova", "revisions/submit", `{"version":5}`, 200},
		{"lead.kovalev", "approve", `{"version":6}`, 200},
		{"lead.kovalev", "approve", `{"version":6}`, 409},
	} {
		code, v = call(t, b, "POST", "/api/plans/u-sokolov/"+x.path, x.user, x.body)
		if code != x.want {
			t.Fatal(x, code, v)
		}
	}
	_, c := persistentServer(t, dsn)
	code, v = call(t, c, "GET", "/api/plans/u-sokolov/history", "intern.sokolov", "")
	if code != 200 || v["active_revision"].(float64) != 2 || len(v["history"].([]any)) != 2 {
		t.Fatal(code, v)
	}
}
func TestPlanVersionAndRoleContract(t *testing.T) {
	_, h := newServer(t)
	for _, body := range []string{"", `{}`, `{"version":0}`, `{"version":1,"unexpected":1}`, `{"version":1} {}`, `not json`} {
		if code, _ := call(t, h, "POST", "/api/plans/u-egorova/approve", "lead.kovalev", body); code != 422 {
			t.Fatal(body, code)
		}
	}
	for _, x := range []struct {
		path, user string
		want       int
	}{
		{"/api/plans/u-sokolov/revisions/start", "intern.sokolov", 403},
		{"/api/plans/u-sokolov/revisions/start", "expert.orlov", 403},
		{"/api/plans/missing/revisions/start", "buddy.belova", 404},
		{"/api/plans/u-sokolov/revisions/no-such-action", "buddy.belova", 404},
	} {
		if code, v := call(t, h, "POST", x.path, x.user, `{"version":1}`); code != x.want {
			t.Fatal(x, code, v)
		}
	}
	if code, _ := call(t, h, "GET", "/api/plans/u-sokolov/history", "expert.orlov", ""); code != 403 {
		t.Fatal(code)
	}
}
