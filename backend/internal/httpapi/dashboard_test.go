package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// SC-21: обзор только своих стажёров, без содержания вопросов и причин отсутствия.
func TestTeamDashboardScopeAndPrivacy(t *testing.T) {
	s, h := newServer(t)
	s.Store.Data.Team.Users = append(s.Store.Data.Team.Users,
		mockdata.User{ID: "outsider", Login: "outsider", Name: "Другой стажёр", Roles: []string{"intern"}, LeadID: "other-lead"})
	s.Store.AddQuestion(&store.Question{ID: "Q-test", AskedBy: "u-sokolov", Text: "PRIVATE-QUESTION", Answer: "PRIVATE-ANSWER", Status: "Sent"})
	s.Store.AddQuestion(&store.Question{ID: "Q-other", AskedBy: "outsider", Status: "Sent"})
	s.Store.AddRequest(&store.Request{ID: "ABS-own", EmployeeID: "u-sokolov", ApproverID: "u-kovalev", Kind: "absence", Status: "pending", Comment: "PRIVATE-COMMENT"})
	s.Store.AddRequest(&store.Request{ID: "ABS-other", EmployeeID: "outsider", ApproverID: "u-kovalev", Status: "pending"})
	code, body := call(t, h, "GET", "/api/dashboard/team", "lead.kovalev", "")
	if code != 200 {
		t.Fatal(code, body)
	}
	data := body["data"].(map[string]any)
	totals := data["totals"].(map[string]any)
	if totals["interns"] != float64(2) || totals["pending_requests"] != float64(1) || totals["open_questions"] != float64(1) {
		t.Fatal(totals)
	}
	if len(data["requests"].([]any)) != 1 {
		t.Fatal(data)
	}
	for _, raw := range data["interns"].([]any) {
		u := raw.(map[string]any)
		if u["id"] == "u-sokolov" && (u["overdue_steps"] != float64(1) || u["plan_status"] != "approved") {
			t.Fatal(u)
		}
		if u["id"] == "u-egorova" && u["plan_status"] != "draft" {
			t.Fatal(u)
		}
	}
	raw, _ := json.Marshal(body)
	for _, forbidden := range []string{"PRIVATE-", "outsider", "ABS-other"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("лишние данные в обзоре", forbidden)
		}
	}
}

// Руководитель может читать план подчинённого, но не получать личные задачи дня и не отмечать шаги.
func TestDashboardRolePermissions(t *testing.T) {
	s, h := newServer(t)
	for _, login := range []string{"intern.sokolov", "buddy.belova", "expert.orlov", ""} {
		expected := 403
		if login == "" {
			expected = 401
		}
		if code, _ := call(t, h, "GET", "/api/dashboard/team", login, ""); code != expected {
			t.Fatal(login, code)
		}
	}
	if code, _ := call(t, h, "GET", "/api/today", "lead.kovalev", ""); code != 403 {
		t.Fatal("личные задачи руководителю", code)
	}
	if code, _ := call(t, h, "GET", "/api/plans/u-sokolov", "lead.kovalev", ""); code != 200 {
		t.Fatal("прогресс подчинённого", code)
	}
	if code, _ := call(t, h, "PATCH", "/api/plan-steps/s-01", "lead.kovalev", `{"intern_id":"u-sokolov"}`); code != 403 {
		t.Fatal("руководитель меняет задачи", code)
	}
	// Руководитель без подчинённых получает пустые массивы, не чужую команду.
	s.Store.Data.Team.Users = append(s.Store.Data.Team.Users, mockdata.User{ID: "empty", Login: "empty", Roles: []string{"lead"}})
	_, body := call(t, h, "GET", "/api/dashboard/team", "empty", "")
	d := body["data"].(map[string]any)
	if len(d["interns"].([]any)) != 0 || len(d["requests"].([]any)) != 0 {
		t.Fatal(d)
	}
}
