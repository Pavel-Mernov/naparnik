package tools

import (
	"strings"

	"github.com/worker4556/naparnik/backend/internal/store"
)

type stepView struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Stage   string `json:"stage"`
	Owner   string `json:"owner"`
	DueDate string `json:"due_date"`
	Status  string `json:"status"`
	Source  string `json:"source"`
}

func targetIntern(c Ctx, internID string) (string, *Result) {
	if internID == "" {
		if store.HasRole(c.User, store.RoleIntern) {
			return c.User.ID, nil
		}
		r := fail("validation", "Укажи intern_id стажёра.")
		return "", &r
	}
	if !c.Store.CanSeeIntern(c.User, internID) {
		r := fail("forbidden", "Можно смотреть только свой план или план своих стажёров.")
		return "", &r
	}
	return internID, nil
}

func planViews(c Ctx, internID string) []stepView {
	var out []stepView
	for _, st := range c.Store.PlanSteps(internID) {
		t, _ := c.Store.TemplateStep(st.StepID)
		out = append(out, stepView{st.StepID, t.Title, t.Stage, t.Owner, st.DueDate, st.Status, t.Source})
	}
	return out
}

func getPlan(c Ctx, args map[string]any) Result {
	internID, errRes := targetIntern(c, str(args, "intern_id"))
	if errRes != nil {
		return *errRes
	}
	steps := planViews(c, internID)
	if len(steps) == 0 {
		return fail("not_found", "План адаптации ещё не утверждён.")
	}
	var mine, done int
	var next *stepView
	for i, s := range steps {
		if s.Owner != store.RoleIntern {
			continue
		}
		mine++
		if s.Status == "Done" {
			done++
		} else if next == nil || s.Status == "Overdue" && next.Status != "Overdue" {
			next = &steps[i]
		}
	}
	return ok(map[string]any{
		"intern": c.Store.Name(internID), "stages": c.Store.Data.Template.Stages, "steps": steps,
		"progress_pct": done * 100 / max(mine, 1), "next_step": next,
	})
}

func getToday(c Ctx, _ map[string]any) Result {
	var overdue, today []stepView
	for _, s := range planViews(c, c.User.ID) {
		if s.Owner != store.RoleIntern {
			continue
		}
		switch s.Status {
		case "Overdue":
			overdue = append(overdue, s)
		case "Today":
			today = append(today, s)
		}
	}
	steps := append(overdue, today...)
	if len(steps) > 3 {
		steps = steps[:3]
	}
	day := c.Store.Today.Format("2006-01-02")
	var events []map[string]string
	for _, e := range c.Store.Data.Calendar.Events {
		if strings.HasPrefix(e.Start, day) && containsStr(e.Participants, c.User.ID) {
			events = append(events, map[string]string{"title": e.Title, "start": e.Start[11:16]})
		}
	}
	for _, m := range c.Store.Meetings() { // встречи по вопросам — тоже в «Ближайшем» (US-14)
		if m.Status == "scheduled" && strings.HasPrefix(m.Start, day) && containsStr(m.Participants, c.User.ID) {
			events = append(events, map[string]string{"title": "Встреча по вопросу · " + c.Store.Name(m.OrganizerID), "start": m.Start[11:16]})
		}
	}
	return ok(map[string]any{"week": c.Store.Week(c.User), "steps": steps, "events": events})
}

func markStepDone(c Ctx, args map[string]any) Result {
	internID := str(args, "intern_id")
	if internID == "" {
		internID = c.User.ID
	}
	if internID != c.User.ID {
		intern, found := c.Store.User(internID)
		if !found || intern.BuddyID != c.User.ID {
			return fail("forbidden", "Отметить шаг может стажёр или его бадди.")
		}
	}
	stepID := str(args, "step_id")
	if !c.Store.SetStepStatus(internID, stepID, "Done") {
		return fail("not_found", "Такого шага нет в плане.")
	}
	c.Store.Emit(store.EvStepDone, c.User.ID, stepID, map[string]string{"intern": internID})
	return Result{OK: true, Data: map[string]string{"step_id": stepID, "status": "Done"},
		Receipt: map[string]string{"id": "STEP-" + stepID, "status": "Done"}}
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
