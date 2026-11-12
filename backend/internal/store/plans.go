package store

import (
	"fmt"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// Жизненный цикл плана адаптации (BR-81, SC-20, US-09): ИИ готовит черновик из шаблона → бадди правит →
// руководитель утверждает одной кнопкой. Нет решения 2 рабочих дня → план активируется по шаблону
// с пометкой «не утверждён», руководителю — напоминание.

const (
	PlanDraft         = "draft"
	PlanApproved      = "approved"
	PlanAutoActivated = "auto_activated" // активирован без утверждения
	PlanApprovalDays  = 2
)

type PlanStep struct {
	StepID  string `json:"step_id"`
	DueDate string `json:"due_date"`
	Removed bool   `json:"removed,omitempty"` // бадди убрал шаг из плана этого стажёра
}

type PlanState struct {
	InternID   string     `json:"intern_id"`
	Status     string     `json:"status"`
	CreatedAt  string     `json:"created_at,omitempty"`
	EditedBy   string     `json:"edited_by,omitempty"`
	EditedAt   string     `json:"edited_at,omitempty"`
	Comment    string     `json:"comment,omitempty"` // комментарий бадди руководителю
	ApprovedBy string     `json:"approved_by,omitempty"`
	ApprovedAt string     `json:"approved_at,omitempty"`
	Steps      []PlanStep `json:"steps,omitempty"` // черновик; после активации шаги живут в трекере
}

// AddWorkdays — дата через n рабочих дней (без выходных).
func AddWorkdays(d time.Time, n int) time.Time {
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n--
		}
	}
	return d
}

// initialPlan — состояние плана из трекера; для черновика шаги собираются из шаблона по сроку от старта.
func (s *Store) initialPlan(p mockdata.Plan) *PlanState {
	st := &PlanState{InternID: p.InternID, Status: p.Status, CreatedAt: p.CreatedAt, ApprovedBy: p.ApprovedBy, ApprovedAt: p.ApprovedAt}
	if st.Status == "" {
		st.Status = PlanApproved
	}
	if st.Status != PlanDraft {
		return st
	}
	u, _ := s.User(p.InternID)
	start, err := time.Parse(time.DateOnly, u.StartDate)
	if err != nil {
		start = s.Today
	}
	for _, t := range s.Data.Template.Steps {
		st.Steps = append(st.Steps, PlanStep{StepID: t.ID, DueDate: AddWorkdays(start, t.DueOffset).Format(time.DateOnly)})
	}
	return st
}

func (s *Store) Plan(internID string) (PlanState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[internID]
	if !ok {
		return PlanState{}, false
	}
	cp := *p
	cp.Steps = append([]PlanStep(nil), p.Steps...)
	return cp, true
}

func (s *Store) UpdatePlan(internID string, fn func(*PlanState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[internID]
	if ok {
		fn(p)
	}
	return ok
}

// ActivatePlan — шаги черновика становятся задачами трекера (кроме убранных бадди).
func (s *Store) ActivatePlan(internID, status, by string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[internID]
	if !ok || p.Status == PlanApproved {
		return false
	}
	if p.Status == PlanDraft {
		epic, n := "ADAPT", 0
		for _, tp := range s.Data.Tracker.Plans {
			if tp.InternID == internID {
				fmt.Sscanf(tp.EpicKey, "ADAPT-%d", &n)
			}
		}
		m := map[string]*mockdata.TrackerStep{}
		for i, st := range p.Steps {
			if st.Removed {
				continue
			}
			m[st.StepID] = &mockdata.TrackerStep{StepID: st.StepID, IssueKey: fmt.Sprintf("%s-%d", epic, n+i+1), DueDate: st.DueDate, Status: "Planned"}
		}
		s.steps[internID] = m
	}
	p.Status = status
	if status == PlanApproved {
		p.ApprovedBy, p.ApprovedAt = by, now.Format(time.RFC3339)
	}
	return true
}

// watchPlans — BR-81: черновик без решения 2 рабочих дня активируется с пометкой «не утверждён».
func (s *Store) watchPlans(now time.Time) {
	for _, intern := range s.Data.Team.Users {
		p, ok := s.Plan(intern.ID)
		if !ok || p.Status != PlanDraft {
			continue
		}
		created, err := time.Parse(time.RFC3339, p.CreatedAt)
		if err != nil {
			continue
		}
		lead, _ := s.User(intern.LeadID)
		hours := ""
		if lead != nil {
			hours = lead.WorkingHours
		}
		if now.Before(AddWorking(hours, created, PlanApprovalDays*WorkingDay(hours))) {
			continue
		}
		s.ActivatePlan(intern.ID, PlanAutoActivated, "system", now)
		s.Emit(EvPlanDecided, "system", intern.ID, map[string]string{"status": PlanAutoActivated})
		s.Notify(Notification{ID: "plan-auto:" + intern.ID, UserID: intern.LeadID, Type: "plan_review", View: "team", TG: "pending",
			Text: fmt.Sprintf("🗂 План адаптации %s два рабочих дня ждал утверждения — я активировал его по шаблону с пометкой «не утверждён».\nЧто сделать: проверь план и нажми «Утвердить» — пометка снимется.", intern.Name)})
		if intern.BuddyID != "" {
			s.Notify(Notification{ID: "plan-auto-b:" + intern.ID, UserID: intern.BuddyID, Type: "plan_review", View: "team", TG: "pending",
				Text: fmt.Sprintf("🗂 План %s активирован по шаблону, пока без утверждения руководителя. Шаги уже видны стажёру.", intern.Name)})
		}
	}
}
