package tools

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// BR-42: «сделай сам» — стажёр оформляет заявку в форме сервиса, проверки те же, что у агента.
func TestCreateSelf(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	res := CreateSelf(c, "absence", map[string]any{"type": "day_off", "date": "2026-11-16", "comment": "семейные дела"})
	r, isReq := res.Data.(store.Request)
	if !res.OK || !isReq || r.ID != "ABS-1042" || r.Source != SourceSelf || r.Status != "pending" || r.ApproverID != "u-kovalev" {
		t.Fatalf("отгул сам: %+v", res)
	}
	if !strings.Contains(r.History[0].Text, "сам") || len(r.History) != 2 {
		t.Fatalf("история: %+v", r.History)
	}
	acc := CreateSelf(c, "access", map[string]any{"system": "sys-redash", "reason": "шаг плана"})
	if a := acc.Data.(store.Request); !acc.OK || a.ID != "ACC-2002" || a.StepID != "s-04" {
		t.Fatalf("доступ сам: %+v", acc)
	}
	cases := []struct {
		login, kind string
		args        map[string]any
		code        string
	}{
		{"intern.sokolov", "car", nil, "validation"},
		{"intern.sokolov", "absence", map[string]any{"type": "day_off", "date": "2026-11-01"}, "validation"},
		{"intern.sokolov", "access", map[string]any{"system": "sys-dwh-pii"}, "forbidden"}, // BR-44
		{"lead.kovalev", "absence", map[string]any{"type": "day_off", "date": "2026-11-16"}, "forbidden"},
	}
	for _, tc := range cases {
		if r := CreateSelf(ctxFor(t, s, tc.login), tc.kind, tc.args); r.OK || r.Error.Code != tc.code {
			t.Errorf("%s %s: %+v", tc.login, tc.kind, r.Error)
		}
	}
}

// Согласование по status_flow сервисов: решает только согласующий, отказ — с кодом причины.
func TestDecide(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	abs := CreateSelf(c, "absence", map[string]any{"type": "day_off", "date": "2026-11-16"}).Data.(store.Request)
	acc := Run(c, "request_access", map[string]any{"system": "sys-redash", "reason": "s-04"}).Data.(*store.Request)
	if acc.Source != SourceAgent {
		t.Fatal("через Напарника — источник agent")
	}
	lead := ctxFor(t, s, "lead.kovalev")

	if _, e := Decide(c, abs.ID, "approve", "", ""); e == nil || e.Code != "forbidden" {
		t.Fatal("стажёр сам себе не согласует")
	}
	bad := []struct{ id, decision, reason, comment, code string }{
		{"ABS-1", "approve", "", "", "not_found"},
		{abs.ID, "maybe", "", "", "validation"},
		{abs.ID, "reject", "", "", "validation"},
		{abs.ID, "reject", "NOT_NEEDED_YET", "", "validation"}, // код другого сервиса
		{abs.ID, "reject", "OTHER", "", "validation"},          // «Другое» без комментария
	}
	for _, tc := range bad {
		if _, e := Decide(lead, tc.id, tc.decision, tc.reason, tc.comment); e == nil || e.Code != tc.code {
			t.Errorf("%+v: %+v", tc, e)
		}
	}
	r, e := Decide(lead, abs.ID, "reject", "DATE_CONFLICT", "перенеси на вторник")
	if e != nil || r.Status != "rejected" || r.ReasonCode != "DATE_CONFLICT" || !strings.Contains(r.History[2].Text, "вторник") {
		t.Fatalf("отказ: %+v %+v", r, e)
	}
	if _, e := Decide(lead, abs.ID, "approve", "", ""); e == nil || e.Code != "validation" {
		t.Fatal("повторное решение")
	}
	g, e := Decide(lead, acc.ID, "approve", "", "")
	if e != nil || g.Status != "granted" || len(g.History) != 4 {
		t.Fatalf("выдача доступа: %+v %+v", g, e)
	}
	// после выдачи доступ виден стажёру как выданный
	sys := Run(c, "get_required_accesses", nil).Data.(map[string]any)["systems"].([]map[string]string)
	for _, x := range sys {
		if x["system"] == "sys-redash" && x["status"] != "granted" {
			t.Fatalf("статус доступа: %v", x)
		}
	}
}

func TestCatalog(t *testing.T) {
	s := newStore(t)
	in := Catalog(ctxFor(t, s, "intern.sokolov"))
	if sys := in["access"].(map[string]any)["systems"].([]map[string]string); len(sys) == 0 {
		t.Fatal("стажёру — системы его недели")
	}
	for _, x := range in["access"].(map[string]any)["systems"].([]map[string]string) {
		if x["system"] == "sys-dwh-pii" {
			t.Fatal("BR-40: чужая система не показывается")
		}
	}
	lead := Catalog(ctxFor(t, s, "lead.kovalev"))
	if len(lead["access"].(map[string]any)["systems"].([]map[string]string)) != 0 {
		t.Fatal("руководителю системы стажёра не нужны")
	}
	if len(s.Data.Absence.ReasonCodes) != 4 || len(s.Data.Access.ReasonCodes) != 3 {
		t.Fatal("коды причин из мок-данных")
	}
}
