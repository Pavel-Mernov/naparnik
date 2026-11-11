package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// SC-33, BR-45: будущий доступ объясняется по каталогу без заявки и без закрытых страниц.
func TestAccessRulesFutureWeek(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	d := data(t, Run(c, "get_access_rules", map[string]any{"system": "sys-dwh-marts-ro"}))
	rules := d["systems"].([]accessRule)
	if len(rules) != 1 || rules[0].WeekFrom != 2 || rules[0].Now || rules[0].Status != "not_requested" ||
		rules[0].Step == nil || rules[0].Step.ID != "s-10" || !strings.Contains(rules[0].Step.Title, "витринам") {
		t.Fatal(rules)
	}
	if d["week"] != 1 || d["approver"].(map[string]string)["name"] != "Андрей Ковалёв" || d["self_service_in_app"] != SelfServiceHint || d["label"] != TestInstructionLabel {
		t.Fatal(d)
	}
	if len(s.Requests("")) != 0 || len(s.PendingDrafts(c.User.ID)) != 0 {
		t.Fatal("объяснение не должно создавать заявку или черновик")
	}
	// BR-45: срок по плану — не запрет ранней заявки; решает согласующий.
	req := CreateSelf(c, "access", map[string]any{"system": "sys-dwh-marts-ro", "reason": "Нужен раньше по задаче"})
	if !req.OK || req.Data.(store.Request).StepID != "s-10" {
		t.Fatal(req)
	}
	rules = data(t, Run(c, "get_access_rules", nil))["systems"].([]accessRule)
	for _, rule := range rules {
		if rule.System == "sys-dwh-marts-ro" && rule.Status != "pending" {
			t.Fatal(rule)
		}
	}
	if _, err := Decide(ctxFor(t, s, "lead.kovalev"), req.Data.(store.Request).ID, "approve", "", ""); err != nil {
		t.Fatal(err)
	}
	s.Today = s.Today.AddDate(0, 0, 7)
	rule := data(t, Run(c, "get_access_rules", map[string]any{"system": "sys-dwh-marts-ro"}))["systems"].([]accessRule)[0]
	if !rule.Now || rule.Status != "granted" {
		t.Fatal(rule)
	}
}

// BR-40: одинаковый фильтр для формы, списка текущей недели, пояснения и создания заявки.
func TestAccessRulesPrivacy(t *testing.T) {
	s := newStore(t)
	week, never := 1, false
	s.Data.Access.Systems = append(s.Data.Access.Systems,
		mockdata.System{ID: "sys-same-team", Title: "PRIVATE-SYSTEM", Department: s.Data.Team.Department.ID, WeekFrom: &week, RequestableByIntern: &never},
		mockdata.System{ID: "sys-no-week", Title: "PRIVATE-NO-WEEK", Department: s.Data.Team.Department.ID},
		mockdata.System{ID: "sys-other", Title: "PRIVATE-OTHER", Department: "other", WeekFrom: &week})
	c := ctxFor(t, s, "intern.sokolov")
	for _, id := range []string{"sys-dwh-pii", "sys-same-team", "sys-no-week", "sys-other"} {
		for _, tool := range []string{"get_access_rules", "request_access"} {
			r := Run(c, tool, map[string]any{"system": id})
			if r.OK || r.Error.Code != "forbidden" {
				t.Fatal(tool, id, r)
			}
		}
		if e := Check(c, "request_access", map[string]any{"system": id}); e == nil || e.Code != "forbidden" {
			t.Fatal("запрет до черновика", id, e)
		}
	}
	for _, value := range []any{Catalog(c), Run(c, "get_access_rules", nil), Run(c, "get_required_accesses", nil)} {
		raw, _ := json.Marshal(value)
		for _, secret := range []string{"PRIVATE-", "sys-dwh-pii", "space-dwh-pii", "kb-R1"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("закрытые метаданные", secret)
			}
		}
	}
	if r := Run(c, "get_access_rules", map[string]any{"system": "unknown"}); r.Error.Code != "not_found" {
		t.Fatal(r)
	}
	for _, login := range []string{"buddy.belova", "lead.kovalev", "expert.orlov"} {
		if r := Run(ctxFor(t, s, login), "get_access_rules", nil); r.Error.Code != "forbidden" {
			t.Fatal(login, r)
		}
	}
}

// SC-33: шаг берётся из своего плана. Черновика, чужого или несуществующего шага в ответе нет.
func TestAccessRulesMissingStepAndEmptyCatalog(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.egorova")
	s.Data.Access.CurrentState[c.User.ID] = []mockdata.AccessState{{System: "sys-dwh-marts-ro", RelatedStep: "s-10"}}
	r := data(t, Run(c, "get_access_rules", map[string]any{"system": "sys-dwh-marts-ro"}))["systems"].([]accessRule)[0]
	if r.Step != nil || relatedAccessStep(c, "sys-nope") != nil {
		t.Fatal(r)
	}
	// Повреждённая ссылка в каталоге не попадает в квитанцию.
	c = ctxFor(t, s, "intern.sokolov")
	s.Data.Access.CurrentState[c.User.ID] = []mockdata.AccessState{{System: "sys-dwh-marts-ro", RelatedStep: "missing"}}
	if req := CreateSelf(c, "access", map[string]any{"system": "sys-dwh-marts-ro"}); req.Data.(store.Request).StepID != "" {
		t.Fatal(req)
	}
	s.Data.Access.Systems = nil
	d := data(t, Run(c, "get_access_rules", nil))
	if len(d["systems"].([]accessRule)) != 0 {
		t.Fatal(d)
	}
}
