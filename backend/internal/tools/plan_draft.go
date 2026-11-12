package tools

import (
	"fmt"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Черновик плана адаптации (SC-20, US-09, BR-81): бадди правит сроки и убирает лишние шаги,
// руководитель утверждает одной кнопкой. Стажёр видит план только после активации.

type PlanDraftStep struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Stage   string `json:"stage"`
	Owner   string `json:"owner"`
	DueDate string `json:"due_date"`
	Removed bool   `json:"removed,omitempty"`
}

type PlanDraftView struct {
	InternID   string           `json:"intern_id"`
	Intern     string           `json:"intern"`
	Status     string           `json:"status"`
	CreatedAt  string           `json:"created_at,omitempty"`
	EditedBy   string           `json:"edited_by,omitempty"`
	Comment    string           `json:"comment,omitempty"`
	ApprovedBy string           `json:"approved_by,omitempty"`
	AutoAt     string           `json:"auto_activate_at,omitempty"` // когда активируется без решения
	CanEdit    bool             `json:"can_edit"`
	CanApprove bool             `json:"can_approve"`
	Stages     []mockdata.Stage `json:"stages"` // названия этапов для группировки шагов на экране
	Steps      []PlanDraftStep  `json:"steps"`
}

type PlanEdit struct {
	StepID  string `json:"step_id"`
	DueDate string `json:"due_date,omitempty"`
	Removed *bool  `json:"removed,omitempty"`
}

func planOwners(c Ctx, internID string) (buddy, lead bool, e *Error) {
	intern, found := c.Store.User(internID)
	if !found || !store.HasRole(intern, store.RoleIntern) {
		return false, false, &Error{Code: "not_found", Message: "Стажёр не найден."}
	}
	buddy, lead = intern.BuddyID == c.User.ID, intern.LeadID == c.User.ID
	if !buddy && !lead {
		return false, false, &Error{Code: "forbidden", Message: "Черновик плана видят бадди и руководитель стажёра."}
	}
	return buddy, lead, nil
}

// GetPlanDraft — черновик (или состояние) плана для бадди и руководителя.
func GetPlanDraft(c Ctx, internID string) (PlanDraftView, *Error) {
	isBuddy, isLead, e := planOwners(c, internID)
	if e != nil {
		return PlanDraftView{}, e
	}
	p, found := c.Store.Plan(internID)
	if !found {
		return PlanDraftView{}, &Error{Code: "not_found", Message: "Плана для этого стажёра нет."}
	}
	v := PlanDraftView{InternID: internID, Intern: c.Store.Name(internID), Status: p.Status, CreatedAt: p.CreatedAt, EditedBy: c.Store.Name(p.EditedBy),
		Comment: p.Comment, ApprovedBy: c.Store.Name(p.ApprovedBy), Stages: c.Store.Data.Template.Stages, Steps: []PlanDraftStep{},
		CanEdit: isBuddy && p.Status == store.PlanDraft, CanApprove: isLead && p.Status != store.PlanApproved}
	if p.ApprovedBy == "" || p.ApprovedBy == "system" {
		v.ApprovedBy = ""
	}
	if created, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil && p.Status == store.PlanDraft {
		hours := ""
		if intern, ok := c.Store.User(internID); ok {
			if lead, ok := c.Store.User(intern.LeadID); ok {
				hours = lead.WorkingHours
			}
		}
		v.AutoAt = store.AddWorking(hours, created, store.PlanApprovalDays*store.WorkingDay(hours)).Format(time.RFC3339)
	}
	steps := p.Steps
	if p.Status != store.PlanDraft {
		for _, st := range c.Store.PlanSteps(internID) {
			steps = append(steps, store.PlanStep{StepID: st.StepID, DueDate: st.DueDate})
		}
	}
	for _, st := range steps {
		t, _ := c.Store.TemplateStep(st.StepID)
		v.Steps = append(v.Steps, PlanDraftStep{st.StepID, t.Title, t.Stage, t.Owner, st.DueDate, st.Removed})
	}
	return v, nil
}

// EditPlanDraft — бадди правит черновик: сроки, убрать или вернуть шаг, комментарий руководителю.
func EditPlanDraft(c Ctx, internID string, edits []PlanEdit, comment string) (PlanDraftView, *Error) {
	return atomic(c, func(c Ctx) (PlanDraftView, *Error) {
		isBuddy, _, e := planOwners(c, internID)
		if e != nil {
			return PlanDraftView{}, e
		}
		p, _ := c.Store.Plan(internID)
		switch {
		case !isBuddy:
			return PlanDraftView{}, &Error{Code: "forbidden", Message: "Черновик плана правит бадди; руководитель утверждает."}
		case p.Status != store.PlanDraft:
			return PlanDraftView{}, &Error{Code: "validation", Message: "План уже активирован — правки в черновик больше не вносятся."}
		}
		index := map[string]int{}
		for i, st := range p.Steps {
			index[st.StepID] = i
		}
		for _, ed := range edits {
			i, has := index[ed.StepID]
			if !has {
				return PlanDraftView{}, &Error{Code: "not_found", Message: "В черновике нет шага " + ed.StepID + "."}
			}
			if ed.DueDate != "" {
				d, ok := parseDate(ed.DueDate)
				if !ok || d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
					return PlanDraftView{}, &Error{Code: "validation", Message: "Срок — рабочий день в формате ГГГГ-ММ-ДД."}
				}
				p.Steps[i].DueDate = ed.DueDate
			}
			if ed.Removed != nil {
				p.Steps[i].Removed = *ed.Removed
			}
		}
		c.Store.UpdatePlan(internID, func(st *store.PlanState) {
			st.Steps, st.EditedBy, st.EditedAt = p.Steps, c.User.ID, store.Now().Format(time.RFC3339)
			if comment != "" {
				st.Comment = comment
			}
		})
		intern, _ := c.Store.User(internID)
		c.Store.Emit(store.EvPlanEdited, c.User.ID, internID, map[string]string{"edits": fmt.Sprint(len(edits))})
		c.Store.Notify(store.Notification{ID: "plan-ready:" + internID, UserID: intern.LeadID, Type: "plan_review", View: "team", TG: "pending",
			Text: fmt.Sprintf("🗂 %s поправил(а) план адаптации %s — он ждёт твоего утверждения.\nЧто сделать: открой план и нажми «Утвердить». Без решения за 2 рабочих дня план активируется по шаблону с пометкой «не утверждён».",
				c.User.Name, intern.Name)})
		return GetPlanDraft(c, internID)
	})
}

// ApprovePlan — руководитель утверждает: шаги появляются у стажёра; у плана, активированного без решения,
// снимается пометка «не утверждён».
func ApprovePlan(c Ctx, internID string) (PlanDraftView, *Error) {
	return atomic(c, func(c Ctx) (PlanDraftView, *Error) {
		_, isLead, e := planOwners(c, internID)
		if e != nil {
			return PlanDraftView{}, e
		}
		if !isLead {
			return PlanDraftView{}, &Error{Code: "forbidden", Message: "План утверждает руководитель стажёра."}
		}
		if !c.Store.ActivatePlan(internID, store.PlanApproved, c.User.ID, store.Now()) {
			return PlanDraftView{}, &Error{Code: "validation", Message: "План уже утверждён."}
		}
		intern, _ := c.Store.User(internID)
		c.Store.Emit(store.EvPlanDecided, c.User.ID, internID, map[string]string{"status": store.PlanApproved})
		c.Store.Notify(store.Notification{ID: "plan-ok:" + internID, UserID: internID, Type: "plan_review", View: "plan", TG: "pending",
			Text: fmt.Sprintf("🗺 Твой план адаптации готов — %s утвердил(а) его.\nЧто сделать: загляни в «Мой план», первые шаги уже там.", c.User.Name)})
		if intern.BuddyID != "" {
			c.Store.Notify(store.Notification{ID: "plan-ok-b:" + internID, UserID: intern.BuddyID, Type: "plan_review", View: "team", TG: "pending",
				Text: fmt.Sprintf("✅ %s утвердил(а) план %s.", c.User.Name, intern.Name)})
		}
		return GetPlanDraft(c, internID)
	})
}
