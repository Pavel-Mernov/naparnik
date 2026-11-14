package tools

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type PlanDraftStep struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Stage   string `json:"stage"`
	Owner   string `json:"owner"`
	DueDate string `json:"due_date"`
	Removed bool   `json:"removed,omitempty"`
	Done    bool   `json:"done"`
}
type PlanHistoryView struct {
	Version  int                `json:"version"`
	Revision int                `json:"revision"`
	Action   string             `json:"action"`
	By       string             `json:"by"`
	At       string             `json:"at"`
	Comment  string             `json:"comment,omitempty"`
	Steps    []PlanDraftStep    `json:"steps"`
	Changes  []store.PlanChange `json:"changes"`
}
type PlanHistoryList struct {
	InternID       string            `json:"intern_id"`
	ActiveRevision int               `json:"active_revision"`
	History        []PlanHistoryView `json:"history"`
}
type PlanDraftView struct {
	InternID       string             `json:"intern_id"`
	Intern         string             `json:"intern"`
	Status         string             `json:"status"`
	CreatedAt      string             `json:"created_at,omitempty"`
	EditedBy       string             `json:"edited_by,omitempty"`
	Comment        string             `json:"comment,omitempty"`
	ApprovedBy     string             `json:"approved_by,omitempty"`
	AutoAt         string             `json:"auto_activate_at,omitempty"`
	CanEdit        bool               `json:"can_edit"`
	CanApprove     bool               `json:"can_approve"`
	CanSubmit      bool               `json:"can_submit"`
	CanReturn      bool               `json:"can_return"`
	CanStart       bool               `json:"can_start"`
	CanCancel      bool               `json:"can_cancel"`
	Version        int                `json:"version"`
	Revision       int                `json:"revision"`
	ActiveRevision int                `json:"active_revision"`
	ReviewStatus   string             `json:"review_status"`
	ReturnComment  string             `json:"return_comment,omitempty"`
	Stages         []mockdata.Stage   `json:"stages"`
	Steps          []PlanDraftStep    `json:"steps"`
	Changes        []store.PlanChange `json:"changes"`
	History        []PlanHistoryView  `json:"history"`
}
type PlanEdit struct {
	StepID  string `json:"step_id"`
	DueDate string `json:"due_date,omitempty"`
	Removed *bool  `json:"removed,omitempty"`
}

func planOwners(c Ctx, id string) (buddy, lead bool, e *Error) {
	u, ok := c.Store.User(id)
	if !ok || !store.HasRole(u, store.RoleIntern) {
		return false, false, &Error{Code: "not_found", Message: "Стажёр не найден."}
	}
	buddy, lead = u.BuddyID == c.User.ID, u.LeadID == c.User.ID
	if !buddy && !lead {
		return false, false, &Error{Code: "forbidden", Message: "Черновик плана видят бадди и руководитель стажёра."}
	}
	return
}
func draftSteps(c Ctx, steps []store.PlanStep, done map[string]bool) []PlanDraftStep {
	out := []PlanDraftStep{}
	for _, st := range steps {
		t, _ := c.Store.TemplateStep(st.StepID)
		out = append(out, PlanDraftStep{st.StepID, t.Title, t.Stage, t.Owner, st.DueDate, st.Removed, done[st.StepID]})
	}
	return out
}
func historyViews(c Ctx, p store.PlanState, published bool) []PlanHistoryView {
	out := []PlanHistoryView{}
	for _, h := range p.History {
		if published && h.Action != store.PlanApproved && h.Action != store.PlanAutoActivated {
			continue
		}
		out = append(out, PlanHistoryView{h.Version, h.Revision, h.Action, c.Store.Name(h.ActorID), h.At, h.Comment, draftSteps(c, h.Steps, nil), h.Changes})
	}
	return out
}
func GetPlanHistory(c Ctx, id string, publishedOnly ...bool) (PlanHistoryList, *Error) {
	if !c.Store.CanSeeIntern(c.User, id) {
		return PlanHistoryList{}, &Error{Code: "forbidden", Message: "Можно смотреть только историю своего плана или планов своих стажёров."}
	}
	p, ok := c.Store.Plan(id)
	if !ok {
		return PlanHistoryList{}, &Error{Code: "not_found", Message: "План не найден."}
	}
	return PlanHistoryList{id, p.ActiveRevision, historyViews(c, p, c.User.ID == id || len(publishedOnly) > 0 && publishedOnly[0])}, nil
}
func GetPlanDraft(c Ctx, id string) (PlanDraftView, *Error) {
	buddy, lead, e := planOwners(c, id)
	if e != nil {
		return PlanDraftView{}, e
	}
	p, ok := c.Store.Plan(id)
	if !ok {
		return PlanDraftView{}, &Error{Code: "not_found", Message: "Плана для этого стажёра нет."}
	}
	done := map[string]bool{}
	for _, st := range c.Store.PlanSteps(id) {
		done[st.StepID] = st.Status == "Done"
	}
	v := PlanDraftView{InternID: id, Intern: c.Store.Name(id), Status: p.Status, CreatedAt: p.CreatedAt, EditedBy: c.Store.Name(p.EditedBy), Comment: p.Comment, ApprovedBy: c.Store.Name(p.ApprovedBy), Version: p.Version, Revision: p.Revision, ActiveRevision: p.ActiveRevision, ReviewStatus: p.ReviewStatus, ReturnComment: p.ReturnComment, Stages: c.Store.Data.Template.Stages, Steps: draftSteps(c, p.Steps, done), Changes: store.PlanChanges(p.ActiveSteps, p.Steps), History: historyViews(c, p, false)}
	if p.EditedBy == "" {
		v.EditedBy = ""
	}
	if p.ApprovedBy == "" || p.ApprovedBy == "system" {
		v.ApprovedBy = ""
	}
	v.CanEdit = buddy && p.ReviewStatus != ""
	v.CanStart = buddy && p.ActiveRevision > 0 && p.ReviewStatus == ""
	v.CanSubmit = buddy && (p.ReviewStatus == store.PlanEditing || p.ReviewStatus == store.PlanReturned)
	v.CanApprove = lead && (p.ReviewStatus == store.PlanReview || p.Status == store.PlanAutoActivated && p.ReviewStatus == "")
	v.CanReturn = lead && (p.ReviewStatus == store.PlanReview || p.ReviewStatus == "" && p.ActiveRevision > 0)
	v.CanCancel = buddy && p.ActiveRevision > 0 && p.ReviewStatus != ""
	if p.Status == store.PlanDraft && p.ReviewStatus == store.PlanReview {
		if at, err := time.Parse(time.RFC3339, p.SubmittedAt); err == nil {
			u, _ := c.Store.User(id)
			lead, _ := c.Store.User(u.LeadID)
			hours := ""
			if lead != nil {
				hours = lead.WorkingHours
			}
			v.AutoAt = store.AddWorking(hours, at, store.PlanApprovalDays*store.WorkingDay(hours)).Format(time.RFC3339)
		}
	}
	return v, nil
}

// Все проверки выполняются на копии внутри общей транзакции PostgreSQL.
func planMutation(c Ctx, id string, expected []int, role string, fn func(Ctx, store.PlanState) *Error) (PlanDraftView, *Error) {
	return atomic(c, func(c Ctx) (PlanDraftView, *Error) {
		buddy, lead, e := planOwners(c, id)
		if e != nil {
			return PlanDraftView{}, e
		}
		if role == store.RoleBuddy && !buddy || role == store.RoleLead && !lead {
			return PlanDraftView{}, &Error{Code: "forbidden", Message: "Это действие недоступно твоей роли в плане стажёра."}
		}
		p, ok := c.Store.Plan(id)
		if !ok {
			return PlanDraftView{}, &Error{Code: "not_found", Message: "План не найден."}
		}
		if len(expected) > 0 && expected[0] != p.Version {
			return PlanDraftView{}, &Error{Code: "conflict", Message: "План изменился. Обнови его и проверь актуальную редакцию."}
		}
		if e := fn(c, p); e != nil {
			return PlanDraftView{}, e
		}
		return GetPlanDraft(c, id)
	})
}
func planComment(comment string) *Error {
	if utf8.RuneCountInString(comment) > 2000 {
		return &Error{Code: "validation", Message: "Комментарий — не больше 2000 символов."}
	}
	return nil
}
func validatePlan(c Ctx, id string, steps []store.PlanStep) *Error {
	live := map[string]mockdata.TrackerStep{}
	for _, st := range c.Store.PlanSteps(id) {
		live[st.StepID] = st
	}
	interns := 0
	for _, st := range steps {
		t, ok := c.Store.TemplateStep(st.StepID)
		if !ok {
			return &Error{Code: "validation", Message: "Шага нет в шаблоне плана."}
		}
		if prev, ok := live[st.StepID]; ok && prev.Status == "Done" && (st.Removed || st.DueDate != prev.DueDate) {
			return &Error{Code: "conflict", Message: "Выполненный шаг нельзя убрать или перенести. Обнови план: стажёр мог завершить его во время согласования."}
		}
		if st.Removed {
			continue
		}
		d, ok := parseDate(st.DueDate)
		if !ok || d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			return &Error{Code: "validation", Message: "Срок — рабочий день в формате ГГГГ-ММ-ДД."}
		}
		if t.Owner == store.RoleIntern {
			interns++
		}
	}
	if interns == 0 {
		return &Error{Code: "validation", Message: "Оставь хотя бы один шаг стажёра."}
	}
	return nil
}
func planNotice(c Ctx, id, action, text string, recipients ...string) {
	p, _ := c.Store.Plan(id)
	event := store.EvPlanReview
	revision := p.Revision
	if action == "approved" {
		event = store.EvPlanDecided
		revision = p.ActiveRevision
	}
	c.Store.Emit(event, c.User.ID, id, map[string]string{"action": action, "revision": fmt.Sprint(revision), "version": fmt.Sprint(p.Version), "status": p.Status})
	for _, who := range recipients {
		if who != "" {
			view := "team"
			if who == id {
				view = "plan"
			}
			c.Store.Notify(store.Notification{ID: fmt.Sprintf("plan:%s:%d:%s", id, p.Version, who), UserID: who, Type: "plan_review", View: view, TG: "pending", Text: text})
		}
	}
}
func EditPlanDraft(c Ctx, id string, edits []PlanEdit, comment string, version ...int) (PlanDraftView, *Error) {
	comment = strings.TrimSpace(comment)
	return planMutation(c, id, version, store.RoleBuddy, func(c Ctx, p store.PlanState) *Error {
		if p.ReviewStatus == "" {
			return &Error{Code: "validation", Message: "Сначала начни новую редакцию действующего плана."}
		}
		if e := planComment(comment); e != nil {
			return e
		}
		index := map[string]int{}
		for i, st := range p.Steps {
			index[st.StepID] = i
		}
		seen := map[string]bool{}
		before := append([]store.PlanStep(nil), p.Steps...)
		for _, ed := range edits {
			i, ok := index[ed.StepID]
			if !ok {
				return &Error{Code: "not_found", Message: "В плане нет шага " + ed.StepID + "."}
			}
			if seen[ed.StepID] {
				return &Error{Code: "validation", Message: "Не передавай один шаг дважды."}
			}
			seen[ed.StepID] = true
			if ed.DueDate != "" {
				p.Steps[i].DueDate = ed.DueDate
			}
			if ed.Removed != nil {
				p.Steps[i].Removed = *ed.Removed
			}
		}
		if e := validatePlan(c, id, p.Steps); e != nil {
			return e
		}
		changes := store.PlanChanges(before, p.Steps)
		if len(changes) == 0 && p.Comment == comment {
			return nil
		}
		c.Store.UpdatePlan(id, func(st *store.PlanState) {
			st.Steps = p.Steps
			st.Comment = comment
			st.ReviewStatus = store.PlanEditing
			st.SubmittedAt = ""
			st.EditedBy = c.User.ID
			st.EditedAt = store.Now().Format(time.RFC3339)
			st.Record("edited", c.User.ID, comment, store.Now(), changes)
		})
		c.Store.Emit(store.EvPlanEdited, c.User.ID, id, map[string]string{"edits": fmt.Sprint(len(changes))})
		return nil
	})
}
func StartPlanRevision(c Ctx, id string, version int) (PlanDraftView, *Error) {
	return planMutation(c, id, []int{version}, store.RoleBuddy, func(c Ctx, p store.PlanState) *Error {
		if p.ActiveRevision == 0 || p.ReviewStatus != "" {
			return &Error{Code: "conflict", Message: "У плана уже есть редакция на доработке или согласовании."}
		}
		c.Store.UpdatePlan(id, func(st *store.PlanState) {
			st.Revision++
			st.Steps = append([]store.PlanStep(nil), st.ActiveSteps...)
			st.ReviewStatus = store.PlanEditing
			st.Comment = ""
			st.ReturnComment = ""
			st.EditedBy = c.User.ID
			st.Record("started", c.User.ID, "", store.Now(), nil)
		})
		planNotice(c, id, "started", "🗂 Бадди начал новую редакцию плана. Действующий план пока не меняется.")
		return nil
	})
}
func SubmitPlanDraft(c Ctx, id string, version int) (PlanDraftView, *Error) {
	return planMutation(c, id, []int{version}, store.RoleBuddy, func(c Ctx, p store.PlanState) *Error {
		if p.ReviewStatus != store.PlanEditing && p.ReviewStatus != store.PlanReturned {
			return &Error{Code: "conflict", Message: "План уже отправлен на согласование."}
		}
		if e := validatePlan(c, id, p.Steps); e != nil {
			return e
		}
		if p.ActiveRevision > 0 && strings.TrimSpace(p.Comment) == "" {
			return &Error{Code: "validation", Message: "Объясни руководителю, зачем меняется действующий план."}
		}
		c.Store.UpdatePlan(id, func(st *store.PlanState) {
			st.ReviewStatus = store.PlanReview
			st.SubmittedAt = store.Now().Format(time.RFC3339)
			st.Record("submitted", c.User.ID, st.Comment, store.Now(), nil)
		})
		u, _ := c.Store.User(id)
		planNotice(c, id, "submitted", fmt.Sprintf("🗂 %s отправил(а) редакцию плана %s на согласование. Открой план: утверди или верни с комментарием.", c.User.Name, u.Name), u.LeadID)
		return nil
	})
}
func ReturnPlan(c Ctx, id string, version int, comment string) (PlanDraftView, *Error) {
	comment = strings.TrimSpace(comment)
	return planMutation(c, id, []int{version}, store.RoleLead, func(c Ctx, p store.PlanState) *Error {
		if comment == "" {
			return &Error{Code: "validation", Message: "Напиши бадди, что нужно доработать."}
		}
		if e := planComment(comment); e != nil {
			return e
		}
		if p.ReviewStatus != store.PlanReview && !(p.ReviewStatus == "" && p.ActiveRevision > 0) {
			return &Error{Code: "conflict", Message: "План уже находится на доработке у бадди."}
		}
		c.Store.UpdatePlan(id, func(st *store.PlanState) {
			if st.ReviewStatus == "" {
				st.Revision++
				st.Steps = append([]store.PlanStep(nil), st.ActiveSteps...)
				st.Comment = ""
			}
			st.ReviewStatus = store.PlanReturned
			st.SubmittedAt = ""
			st.ReturnComment = comment
			st.Record("returned", c.User.ID, comment, store.Now(), nil)
		})
		u, _ := c.Store.User(id)
		planNotice(c, id, "returned", fmt.Sprintf("🗂 %s вернул(а) план %s на доработку. Замечания — в карточке плана. Действующий план сохраняется.", c.User.Name, u.Name), u.BuddyID)
		return nil
	})
}
func CancelPlanRevision(c Ctx, id string, version int) (PlanDraftView, *Error) {
	return planMutation(c, id, []int{version}, store.RoleBuddy, func(c Ctx, p store.PlanState) *Error {
		if p.ActiveRevision == 0 || p.ReviewStatus == "" {
			return &Error{Code: "validation", Message: "Отменить можно только новую редакцию действующего плана."}
		}
		c.Store.UpdatePlan(id, func(st *store.PlanState) {
			st.Record("cancelled", c.User.ID, st.Comment, store.Now(), nil)
			st.Steps = append([]store.PlanStep(nil), st.ActiveSteps...)
			st.ReviewStatus = ""
			st.SubmittedAt = ""
			st.ReturnComment = ""
			st.Comment = ""
		})
		u, _ := c.Store.User(id)
		planNotice(c, id, "cancelled", fmt.Sprintf("🗂 Новая редакция плана %s отменена. Действующий план не изменился.", u.Name), u.LeadID)
		return nil
	})
}
func ApprovePlan(c Ctx, id string, version ...int) (PlanDraftView, *Error) {
	return planMutation(c, id, version, store.RoleLead, func(c Ctx, p store.PlanState) *Error {
		if p.ReviewStatus != store.PlanReview && !(p.Status == store.PlanAutoActivated && p.ReviewStatus == "") {
			return &Error{Code: "validation", Message: "План не отправлен на согласование или уже утверждён."}
		}
		if e := validatePlan(c, id, p.Steps); e != nil {
			return e
		}
		if !c.Store.ActivatePlan(id, store.PlanApproved, c.User.ID, store.Now()) {
			return &Error{Code: "conflict", Message: "Состояние плана изменилось. Обнови его перед утверждением."}
		}
		u, _ := c.Store.User(id)
		planNotice(c, id, "approved", fmt.Sprintf("🗺 %s утвердил(а) редакцию плана %s. Изменения и причина доступны в истории плана; выполненные шаги сохранены.", c.User.Name, u.Name), id, u.BuddyID)
		return nil
	})
}
