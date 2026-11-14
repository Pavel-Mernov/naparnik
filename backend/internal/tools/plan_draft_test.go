package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// SC-20, BR-81: черновик из шаблона → Екатерина правит → Андрей утверждает → шаги у Полины.
func TestPlanDraftEditApprove(t *testing.T) {
	s := newStore(t)
	intern := ctxFor(t, s, "intern.egorova")
	buddy, lead := ctxFor(t, s, "buddy.belova"), ctxFor(t, s, "lead.kovalev")

	if r := Run(intern, "get_plan", nil); r.OK || !strings.Contains(r.Error.Message, "черновик у бадди") {
		t.Fatal("до утверждения стажёр плана не видит", r)
	}
	if _, e := GetPlanDraft(intern, "u-egorova"); e == nil || e.Code != "forbidden" {
		t.Fatal("черновик стажёру не показывается", e)
	}
	v, e := GetPlanDraft(buddy, "u-egorova")
	if e != nil || v.Status != store.PlanDraft || !v.CanEdit || v.CanApprove || len(v.Steps) != len(s.Data.Template.Steps) || v.AutoAt == "" {
		t.Fatal(v, e)
	}
	// Срок от старта 07.12 (пн): шаг с due_offset 1 → 08.12.
	if v.Steps[1].ID != "s-02" || v.Steps[1].DueDate != "2026-12-08" {
		t.Fatal(v.Steps[1])
	}
	removed := true
	if _, e := EditPlanDraft(lead, "u-egorova", nil, ""); e == nil {
		t.Fatal("руководитель не правит черновик — он утверждает")
	}
	if _, e := EditPlanDraft(buddy, "u-egorova", []PlanEdit{{StepID: "s-03", DueDate: "2026-12-12"}}, ""); e == nil {
		t.Fatal("суббота — не рабочий день")
	}
	v, e = EditPlanDraft(buddy, "u-egorova", []PlanEdit{{StepID: "s-03", DueDate: "2026-12-11"}, {StepID: "s-14", Removed: &removed}}, "Убрала A/B — у Полины другой проект")
	if e != nil || v.EditedBy != "Екатерина Белова" || v.Comment == "" {
		t.Fatal(v, e)
	}
	v, e = SubmitPlanDraft(buddy, "u-egorova", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	ready := false
	for _, n := range s.NotificationsFor("u-kovalev") {
		ready = ready || n.Type == "plan_review" && strings.Contains(n.Text, "на согласование")
	}
	if !ready {
		t.Fatal("руководитель не узнал, что план готов")
	}
	if _, e := ApprovePlan(buddy, "u-egorova"); e == nil {
		t.Fatal("бадди не утверждает")
	}
	v, e = ApprovePlan(lead, "u-egorova")
	if e != nil || v.Status != store.PlanApproved || v.ApprovedBy != "Андрей Ковалёв" || v.CanApprove {
		t.Fatal(v, e)
	}
	plan := data(t, Run(intern, "get_plan", nil))
	steps := plan["steps"].([]stepView)
	if len(steps) != len(s.Data.Template.Steps)-1 || plan["approval"] != nil {
		t.Fatal("убранный шаг не попадает в план", len(steps))
	}
	for _, st := range steps {
		if st.ID == "s-03" && st.DueDate != "2026-12-11" {
			t.Fatal("правка срока потерялась", st)
		}
	}
	if _, e := ApprovePlan(lead, "u-egorova"); e == nil {
		t.Fatal("повторное утверждение")
	}
}

// BR-81: без решения 2 рабочих дня — активируется по шаблону с пометкой «не утверждён»; утверждение снимает пометку.
func TestPlanAutoActivation(t *testing.T) {
	s := newStore(t)
	created, _ := time.Parse(time.RFC3339, "2026-11-11T09:00:00+03:00")
	s.Watch(created.Add(24 * time.Hour))
	if p, _ := s.Plan("u-egorova"); p.Status != store.PlanDraft {
		t.Fatal("рано", p.Status)
	}
	s.Watch(created.AddDate(0, 0, 3))
	if p, _ := s.Plan("u-egorova"); p.Status != store.PlanAutoActivated {
		t.Fatal(p.Status)
	}
	intern := ctxFor(t, s, "intern.egorova")
	if d := data(t, Run(intern, "get_plan", nil)); d["approval"] == nil || d["plan_status"] != store.PlanAutoActivated {
		t.Fatal(d)
	}
	// Обзор руководителя показывает тот же статус, что и план (а не исходный из трекера).
	for _, i := range TeamDashboard(ctxFor(t, s, "lead.kovalev")).Data.(teamOverview).Interns {
		if i.ID == "u-egorova" && i.PlanStatus != store.PlanAutoActivated {
			t.Fatal("обзор команды:", i.PlanStatus)
		}
	}
	reminded := false
	for _, n := range s.NotificationsFor("u-kovalev") {
		reminded = reminded || strings.Contains(n.Text, "«не утверждён»")
	}
	if !reminded {
		t.Fatal("руководителю нет напоминания")
	}
	if _, e := EditPlanDraft(ctxFor(t, s, "buddy.belova"), "u-egorova", nil, "x"); e == nil {
		t.Fatal("активированный план — уже не черновик")
	}
	if v, e := ApprovePlan(ctxFor(t, s, "lead.kovalev"), "u-egorova"); e != nil || v.Status != store.PlanApproved {
		t.Fatal(v, e)
	}
	if d := data(t, Run(intern, "get_plan", nil)); d["approval"] != nil {
		t.Fatal("пометка не снялась", d)
	}
}
