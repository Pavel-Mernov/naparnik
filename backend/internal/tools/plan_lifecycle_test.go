package tools

import (
	"reflect"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// SC-20, SCR-L2: активный план остаётся рабочим, пока новая редакция проходит весь цикл согласования.
func TestActivePlanRevisionLifecycle(t *testing.T) {
	s := newStore(t)
	buddy, lead, intern := ctxFor(t, s, "buddy.belova"), ctxFor(t, s, "lead.kovalev"), ctxFor(t, s, "intern.sokolov")
	before := s.PlanSteps("u-sokolov")
	target := ""
	for _, st := range before {
		if st.Status != "Done" {
			target = st.StepID
			break
		}
	}
	v, e := StartPlanRevision(buddy, "u-sokolov", 1)
	if e != nil || v.Revision != 2 || !v.CanEdit {
		t.Fatal(v, e)
	}
	if _, e := EditPlanDraft(buddy, "u-sokolov", nil, "stale", 1); e == nil || e.Code != "conflict" {
		t.Fatal(e)
	}
	v, e = EditPlanDraft(buddy, "u-sokolov", []PlanEdit{{StepID: target, DueDate: "2026-12-21"}}, "Срок учитывает новый проект", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, s.PlanSteps("u-sokolov")) {
		t.Fatal("несогласованные правки изменили задачи")
	}
	if _, e := ApprovePlan(lead, "u-sokolov", v.Version); e == nil {
		t.Fatal("несохранённая на согласование редакция утверждена")
	}
	v, e = SubmitPlanDraft(buddy, "u-sokolov", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = ReturnPlan(lead, "u-sokolov", v.Version, "Перенеси ещё на день")
	if e != nil || v.ReturnComment == "" || v.ReviewStatus != store.PlanReturned {
		t.Fatal(v, e)
	}
	v, e = EditPlanDraft(buddy, "u-sokolov", []PlanEdit{{StepID: target, DueDate: "2026-12-22"}}, "Учтено замечание руководителя", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = SubmitPlanDraft(buddy, "u-sokolov", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	// Во время согласования работа продолжается — ещё один шаг становится выполненным.
	doneID := ""
	for _, st := range before {
		if st.StepID != target && st.Status != "Done" {
			doneID = st.StepID
			break
		}
	}
	s.SetStepStatus("u-sokolov", doneID, "Done")
	v, e = ApprovePlan(lead, "u-sokolov", v.Version)
	if e != nil || v.ActiveRevision != 2 || v.ReviewStatus != "" {
		t.Fatal(v, e)
	}
	seen := map[string]bool{}
	for _, st := range v.Steps {
		if seen[st.ID] {
			t.Fatal("дубликат шага")
		}
		seen[st.ID] = true
	}
	kept := 0
	for _, st := range v.Steps {
		if !st.Removed {
			kept++
		}
	}
	if kept != len(before) {
		t.Fatal(kept, len(before))
	}
	after := s.PlanSteps("u-sokolov")
	for i, st := range after {
		if st.StepID == target && st.DueDate != "2026-12-22" {
			t.Fatal(st)
		}
		if st.StepID == target && st.Status != "Planned" {
			t.Fatal("статус не учитывает новый срок", st)
		}
		if before[i].Status == "Done" && st != before[i] {
			t.Fatal("потерян выполненный шаг или квитанция", st)
		}
		if st.StepID == doneID && st.Status != "Done" {
			t.Fatal("потерян прогресс во время согласования")
		}
	}
	h, e := GetPlanHistory(intern, "u-sokolov")
	if e != nil || len(h.History) != 2 || h.History[1].Comment != "Учтено замечание руководителя" {
		t.Fatal(h, e)
	}
	if _, leaked := data(t, Run(intern, "get_plan", nil))["plan_history"]; leaked {
		t.Fatal("история для человека попала в инструмент модели")
	}
	if _, e := GetPlanHistory(ctxFor(t, s, "expert.orlov"), "u-sokolov"); e == nil {
		t.Fatal("чужая история")
	}
	v, e = StartPlanRevision(buddy, "u-sokolov", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = CancelPlanRevision(buddy, "u-sokolov", v.Version)
	if e != nil || !v.CanStart || !reflect.DeepEqual(after, s.PlanSteps("u-sokolov")) {
		t.Fatal(v, e)
	}
	v, e = ReturnPlan(lead, "u-sokolov", v.Version, "Нужна новая редакция для следующего проекта")
	if e != nil || v.Revision != 4 || v.ReviewStatus != store.PlanReturned {
		t.Fatal(v, e)
	}
}

// Выполненный во время согласования шаг нельзя потерять вместе с новой редакцией.
func TestPlanCompletionRaceAndValidation(t *testing.T) {
	s := newStore(t)
	buddy, lead := ctxFor(t, s, "buddy.belova"), ctxFor(t, s, "lead.kovalev")
	v, e := StartPlanRevision(buddy, "u-sokolov", 1)
	if e != nil {
		t.Fatal(e)
	}
	var target string
	var due string
	for _, st := range s.PlanSteps("u-sokolov") {
		if st.Status != "Done" {
			target, due = st.StepID, st.DueDate
			break
		}
	}
	removed := true
	v, e = EditPlanDraft(buddy, "u-sokolov", []PlanEdit{{StepID: target, Removed: &removed}}, "Шаг больше не нужен", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = SubmitPlanDraft(buddy, "u-sokolov", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	s.SetStepStatus("u-sokolov", target, "Done")
	if _, e := ApprovePlan(lead, "u-sokolov", v.Version); e == nil || e.Code != "conflict" {
		t.Fatal(e)
	}
	v, e = ReturnPlan(lead, "u-sokolov", v.Version, "Сохрани выполненный шаг")
	if e != nil {
		t.Fatal(e)
	}
	kept := false
	v, e = EditPlanDraft(buddy, "u-sokolov", []PlanEdit{{StepID: target, Removed: &kept, DueDate: due}}, "Выполненный шаг сохранён", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	for _, ed := range [][]PlanEdit{{{StepID: "unknown"}}, {{StepID: target}, {StepID: target}}} {
		if _, e := EditPlanDraft(buddy, "u-sokolov", ed, "", v.Version); e == nil {
			t.Fatal("некорректная правка принята")
		}
	}
	if _, e := ReturnPlan(lead, "u-sokolov", v.Version, ""); e == nil {
		t.Fatal("нет замечания")
	}
	if _, e := StartPlanRevision(lead, "u-sokolov", v.Version); e == nil {
		t.Fatal("руководитель сам редактирует")
	}
	if _, e := GetPlanDraft(buddy, "missing"); e == nil {
		t.Fatal("нет стажёра")
	}
}

// Решение «вернуть» останавливает автоактивацию; после повторной отправки срок начинается заново.
func TestReturnedPlanDoesNotAutoActivate(t *testing.T) {
	old := store.Now
	defer func() { store.Now = old }()
	now, _ := time.Parse(time.RFC3339, "2026-11-12T11:00:00+03:00")
	store.Now = func() time.Time { return now }
	s := newStore(t)
	buddy, lead := ctxFor(t, s, "buddy.belova"), ctxFor(t, s, "lead.kovalev")
	v, e := ReturnPlan(lead, "u-egorova", 1, "Нужна правка сроков")
	if e != nil {
		t.Fatal(e)
	}
	s.Watch(now.AddDate(0, 0, 20))
	p, _ := s.Plan("u-egorova")
	if p.Status != store.PlanDraft {
		t.Fatal(p)
	}
	v, e = EditPlanDraft(buddy, "u-egorova", nil, "Сроки проверены", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = SubmitPlanDraft(buddy, "u-egorova", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	due, _ := time.Parse(time.RFC3339, v.AutoAt)
	s.Watch(due.Add(-time.Second))
	p, _ = s.Plan("u-egorova")
	if p.Status != store.PlanDraft {
		t.Fatal("ранняя автоактивация")
	}
	s.Watch(due)
	p, _ = s.Plan("u-egorova")
	if p.Status != store.PlanAutoActivated {
		t.Fatal(p)
	}
	// Отмена новой редакции не меняет номер действующей при позднем утверждении автоактивированного плана.
	v, e = StartPlanRevision(buddy, "u-egorova", p.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = CancelPlanRevision(buddy, "u-egorova", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = ApprovePlan(lead, "u-egorova", v.Version)
	if e != nil || v.ActiveRevision != 1 {
		t.Fatal(v, e)
	}
	events := s.Events()
	last := events[len(events)-1]
	if last.Type != store.EvPlanDecided || last.Meta["revision"] != "1" {
		t.Fatal(last)
	}
}
