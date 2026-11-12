package httpapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// BR-02, BR-40: закрытая, отсутствующая и неопубликованная страницы не оцениваются.
// Отказ не раскрывает существование страницы и не меняет события, метрики и уведомления.
func TestFeedbackAccess(t *testing.T) {
	for _, tc := range []struct {
		name, user, body string
		want             int
	}{
		{"missing", "intern.sokolov", `{"page_id":"missing","value":"stale"}`, 404},
		{"restricted", "intern.sokolov", `{"page_id":"kb-R1","value":"stale"}`, 404},
		{"draft", "intern.sokolov", `{"page_id":"private-draft","value":"stale"}`, 404},
		{"own-draft", "expert.orlov", `{"page_id":"private-draft","value":"helped"}`, 404},
		{"archived", "intern.sokolov", `{"page_id":"archived","value":"helped"}`, 404},
		{"invalid-value", "intern.sokolov", `{"page_id":"kb-03","value":"other"}`, 422},
		{"empty-page", "intern.sokolov", `{"value":"helped"}`, 422},
		{"broken-json", "intern.sokolov", `{`, 422},
		{"no-user", "", `{"page_id":"kb-03","value":"helped"}`, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, h := newServer(t)
			for _, p := range []mockdata.Page{
				{ID: "private-draft", Space: "space-analytics", Status: "Draft", AuthorID: "u-orlov", Title: "PRIVATE"},
				{ID: "archived", Space: "space-analytics", Status: "Archived", AuthorID: "u-orlov", Title: "PRIVATE"},
			} {
				srv.Store.AddPage(p)
			}
			before := Metrics(srv.Store.Events())
			code, r := call(t, h, "POST", "/api/feedback", tc.user, tc.body)
			if code != tc.want {
				t.Fatalf("%d: %v", code, r)
			}
			if code == 404 && !reflect.DeepEqual(r, map[string]any{"ok": false, "error": map[string]any{"code": "not_found", "message": "Страница не найдена."}}) {
				t.Fatalf("отказ раскрывает страницу: %v", r)
			}
			if !reflect.DeepEqual(before, Metrics(srv.Store.Events())) || len(srv.Store.Events()) != 0 {
				t.Fatal("отказ изменил события или метрики")
			}
			for _, u := range srv.Store.Data.Team.Users {
				if len(srv.Store.NotificationsFor(u.ID)) != 0 {
					t.Fatal("отказ создал уведомление")
				}
			}
		})
	}
}

func TestFeedbackPublishedPage(t *testing.T) {
	for _, value := range []string{"helped", "not_helped", "stale"} {
		t.Run(value, func(t *testing.T) {
			srv, h := newServer(t)
			code, r := call(t, h, "POST", "/api/feedback", "intern.sokolov", `{"page_id":"kb-03","value":"`+value+`"}`)
			events := srv.Store.Events()
			if code != 200 || r["ok"] != true || len(events) != 1 {
				t.Fatalf("оценка: %d %v; события %v", code, r, events)
			}
			e := events[0]
			if e.Type != store.EvAnswerFeedback || e.Actor != "u-sokolov" || e.Object != "kb-03" || e.Meta["value"] != value || e.Meta["author"] != "u-orlov" {
				t.Fatal(e)
			}
			n := srv.Store.NotificationsFor("u-orlov")
			if value == "stale" {
				if len(n) != 1 || n[0].Type != "page_stale" || !strings.Contains(n[0].Text, e.Meta["page_title"]) {
					t.Fatal(n)
				}
			} else if len(n) != 0 {
				t.Fatal("лишнее уведомление")
			}
		})
	}
}
