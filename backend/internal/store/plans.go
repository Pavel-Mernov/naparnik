package store

import (
	"fmt"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// BR-81, SC-20, SCR-L2: редакция согласуется отдельно от действующего плана.
const (
	PlanDraft         = "draft"
	PlanApproved      = "approved"
	PlanAutoActivated = "auto_activated"
	PlanEditing       = "editing"
	PlanReview        = "review"
	PlanReturned      = "returned"
	PlanApprovalDays  = 2
)

type PlanStep struct {
	StepID  string `json:"step_id"`
	DueDate string `json:"due_date"`
	Removed bool   `json:"removed,omitempty"`
}
type PlanChange struct {
	StepID     string `json:"step_id"`
	OldDueDate string `json:"old_due_date"`
	DueDate    string `json:"due_date"`
	WasRemoved bool   `json:"was_removed"`
	Removed    bool   `json:"removed"`
}
type PlanHistory struct {
	Version  int          `json:"version"`
	Revision int          `json:"revision"`
	Action   string       `json:"action"`
	ActorID  string       `json:"actor_id"`
	At       string       `json:"at"`
	Comment  string       `json:"comment,omitempty"`
	Steps    []PlanStep   `json:"steps"`
	Changes  []PlanChange `json:"changes"`
}
type PlanState struct {
	InternID       string        `json:"intern_id"`
	Status         string        `json:"status"`
	CreatedAt      string        `json:"created_at,omitempty"`
	EditedBy       string        `json:"edited_by,omitempty"`
	EditedAt       string        `json:"edited_at,omitempty"`
	Comment        string        `json:"comment,omitempty"`
	ApprovedBy     string        `json:"approved_by,omitempty"`
	ApprovedAt     string        `json:"approved_at,omitempty"`
	Steps          []PlanStep    `json:"steps,omitempty"`
	Version        int           `json:"version"`
	Revision       int           `json:"revision"`
	ActiveRevision int           `json:"active_revision"`
	ReviewStatus   string        `json:"review_status,omitempty"`
	SubmittedAt    string        `json:"submitted_at,omitempty"`
	ReturnComment  string        `json:"return_comment,omitempty"`
	ActiveSteps    []PlanStep    `json:"active_steps,omitempty"`
	History        []PlanHistory `json:"history"`
}

func AddWorkdays(d time.Time, n int) time.Time {
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n--
		}
	}
	return d
}
func (s *Store) initialPlan(p mockdata.Plan) *PlanState {
	st := &PlanState{InternID: p.InternID, Status: p.Status, CreatedAt: p.CreatedAt, ApprovedBy: p.ApprovedBy, ApprovedAt: p.ApprovedAt}
	if st.Status == "" {
		st.Status = PlanApproved
	}
	if st.Status == PlanDraft {
		u, _ := s.User(p.InternID)
		start, err := time.Parse(time.DateOnly, u.StartDate)
		if err != nil {
			start = s.Today
		}
		for _, t := range s.Data.Template.Steps {
			st.Steps = append(st.Steps, PlanStep{StepID: t.ID, DueDate: AddWorkdays(start, t.DueOffset).Format(time.DateOnly)})
		}
	}
	s.initPlanVersion(st)
	return st
}

// Миграция прежних снимков; вызывается до публикации состояния читателям.
func (s *Store) initPlanVersion(p *PlanState) {
	if p.Version != 0 {
		return
	}
	p.Version, p.Revision = 1, 1
	action, at, actor := "created", p.CreatedAt, "system"
	if p.Status == PlanDraft {
		p.ReviewStatus, p.SubmittedAt = PlanReview, p.CreatedAt
	} else {
		p.ActiveRevision = 1
		p.Steps = s.activeConfiguration(p.InternID, p.Steps)
		p.ActiveSteps = append([]PlanStep(nil), p.Steps...)
		action, at, actor = p.Status, p.ApprovedAt, p.ApprovedBy
	}
	if at == "" {
		at = s.Today.Format(time.RFC3339)
	}
	if actor == "" {
		actor = "system"
	}
	p.History = []PlanHistory{{Version: 1, Revision: 1, Action: action, ActorID: actor, At: at, Steps: append([]PlanStep{}, p.Steps...), Changes: []PlanChange{}}}
}
func (s *Store) activeConfiguration(id string, previous []PlanStep) []PlanStep {
	old := map[string]PlanStep{}
	for _, st := range previous {
		old[st.StepID] = st
	}
	out := []PlanStep{}
	for _, t := range s.Data.Template.Steps {
		st := old[t.ID]
		st.StepID, st.Removed = t.ID, true
		if live := s.steps[id][t.ID]; live != nil {
			st.DueDate, st.Removed = live.DueDate, false
		}
		out = append(out, st)
	}
	return out
}
func (s *Store) Plan(id string) (PlanState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if !ok {
		return PlanState{}, false
	}
	cp := *p
	cp.Steps = append([]PlanStep(nil), p.Steps...)
	cp.ActiveSteps = append([]PlanStep(nil), p.ActiveSteps...)
	cp.History = append([]PlanHistory(nil), p.History...)
	for i := range cp.History {
		cp.History[i].Steps = append([]PlanStep{}, p.History[i].Steps...)
		cp.History[i].Changes = append([]PlanChange{}, p.History[i].Changes...)
	}
	return cp, true
}
func (s *Store) UpdatePlan(id string, fn func(*PlanState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if ok {
		fn(p)
	}
	return ok
}
func PlanChanges(before, after []PlanStep) []PlanChange {
	old := map[string]PlanStep{}
	for _, st := range before {
		old[st.StepID] = st
	}
	out := []PlanChange{}
	for _, st := range after {
		prev, ok := old[st.StepID]
		if !ok {
			prev = PlanStep{StepID: st.StepID, Removed: true}
		}
		if prev.DueDate != st.DueDate || prev.Removed != st.Removed {
			out = append(out, PlanChange{st.StepID, prev.DueDate, st.DueDate, prev.Removed, st.Removed})
		}
	}
	return out
}

// Record сохраняет неизменяемый снимок. Вызывается под блокировкой Store.
func (p *PlanState) Record(action, actor, comment string, now time.Time, changes []PlanChange) {
	p.Version++
	p.History = append(p.History, PlanHistory{Version: p.Version, Revision: p.Revision, Action: action, ActorID: actor, At: now.Format(time.RFC3339), Comment: comment, Steps: append([]PlanStep{}, p.Steps...), Changes: append([]PlanChange{}, changes...)})
}

// ActivatePlan сохраняет статусы, номера задач и квитанции выполненных шагов.
func (s *Store) ActivatePlan(id, status, by string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if !ok || (p.ReviewStatus != PlanReview && !(p.Status == PlanAutoActivated && p.ReviewStatus == "")) {
		return false
	}
	if status == PlanAutoActivated && p.Status != PlanDraft {
		return false
	}
	for _, st := range p.Steps {
		if old := s.steps[id][st.StepID]; old != nil && old.Status == "Done" && (st.Removed || st.DueDate != old.DueDate) {
			return false
		}
	}
	changes := PlanChanges(p.ActiveSteps, p.Steps)
	epic := "ADAPT-" + id
	for _, tp := range s.Data.Tracker.Plans {
		if tp.InternID == id {
			epic = tp.EpicKey
		}
	}
	m := map[string]*mockdata.TrackerStep{}
	for _, st := range p.Steps {
		if st.Removed {
			continue
		}
		if old := s.steps[id][st.StepID]; old != nil {
			cp := *old
			cp.DueDate = st.DueDate
			if cp.Status != "Done" && old.DueDate != st.DueDate {
				cp.Status = planStepStatus(st.DueDate, s.Today)
			}
			m[st.StepID] = &cp
		} else {
			m[st.StepID] = &mockdata.TrackerStep{StepID: st.StepID, IssueKey: epic + "-" + st.StepID, DueDate: st.DueDate, Status: planStepStatus(st.DueDate, s.Today)}
		}
	}
	revision := p.Revision
	if p.ReviewStatus == "" {
		revision = p.ActiveRevision
	}
	s.steps[id] = m
	p.Status, p.ActiveRevision, p.ReviewStatus = status, revision, ""
	p.ActiveSteps = append([]PlanStep(nil), p.Steps...)
	if status == PlanApproved {
		p.ApprovedBy, p.ApprovedAt = by, now.Format(time.RFC3339)
	}
	p.Record(status, by, p.Comment, now, changes)
	p.History[len(p.History)-1].Revision = revision
	return true
}

func planStepStatus(due string, today time.Time) string {
	day := today.Format(time.DateOnly)
	if due < day {
		return "Overdue"
	}
	if due == day {
		return "Today"
	}
	return "Planned"
}
func (s *Store) watchPlans(now time.Time) {
	for _, intern := range s.Data.Team.Users {
		p, ok := s.Plan(intern.ID)
		if !ok || p.Status != PlanDraft || p.ReviewStatus != PlanReview {
			continue
		}
		submitted, err := time.Parse(time.RFC3339, p.SubmittedAt)
		if err != nil {
			continue
		}
		lead, _ := s.User(intern.LeadID)
		hours := ""
		if lead != nil {
			hours = lead.WorkingHours
		}
		if now.Before(AddWorking(hours, submitted, PlanApprovalDays*WorkingDay(hours))) {
			continue
		}
		if !s.ActivatePlan(intern.ID, PlanAutoActivated, "system", now) {
			continue
		}
		s.Emit(EvPlanDecided, "system", intern.ID, map[string]string{"status": PlanAutoActivated})
		for _, who := range []string{intern.LeadID, intern.BuddyID} {
			if who != "" {
				s.Notify(Notification{ID: fmt.Sprintf("plan-auto:%s:%d:%s", intern.ID, p.Revision, who), UserID: who, Type: "plan_review", View: "team", TG: "pending", Text: fmt.Sprintf("🗂 План адаптации %s два рабочих дня ждал утверждения — активирован с пометкой «не утверждён». Открой план и проверь его.", intern.Name)})
			}
		}
	}
}
