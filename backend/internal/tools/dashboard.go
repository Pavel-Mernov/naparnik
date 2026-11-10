package tools

import "github.com/worker4556/naparnik/backend/internal/store"

type internOverview struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Buddy         string `json:"buddy"`
	Week          int    `json:"week"`
	PlanStatus    string `json:"plan_status"`
	Stage         string `json:"stage"`
	Progress      int    `json:"progress_pct"`
	OverdueSteps  int    `json:"overdue_steps"`
	OpenQuestions int    `json:"open_questions"`
}

// В обзор не входят тексты вопросов, ответы, комментарии и причины отсутствия.
type requestOverview struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Type         string `json:"type,omitempty"`
	Date         string `json:"date,omitempty"`
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	Status       string `json:"status"`
}

type teamOverview struct {
	Department string            `json:"department"`
	Interns    []internOverview  `json:"interns"`
	Requests   []requestOverview `json:"requests"`
	Totals     struct {
		Interns         int `json:"interns"`
		OverdueSteps    int `json:"overdue_steps"`
		OpenQuestions   int `json:"open_questions"`
		PendingRequests int `json:"pending_requests"`
	} `json:"totals"`
}

// TeamDashboard — SC-21, 02-ROLES: руководитель видит своих стажёров и числа,
// а не личные вопросы. Доступ к плану стажёра не превращает его задачи в «мои».
func TeamDashboard(c Ctx) Result {
	if !store.HasRole(c.User, store.RoleLead) {
		return fail("forbidden", "Обзор команды доступен руководителю.")
	}
	out := teamOverview{Department: c.Store.Data.Team.Department.Name, Interns: []internOverview{}, Requests: []requestOverview{}}
	questions := c.Store.Questions()
	owned := map[string]string{}
	for _, intern := range c.Store.Data.Team.Users {
		if !store.HasRole(&intern, store.RoleIntern) || intern.LeadID != c.User.ID {
			continue
		}
		owned[intern.ID] = intern.Name
		v := internOverview{ID: intern.ID, Name: intern.Name, Buddy: c.Store.Name(intern.BuddyID), Week: c.Store.Week(&intern), PlanStatus: "missing"}
		for _, plan := range c.Store.Data.Tracker.Plans {
			if plan.InternID == intern.ID {
				v.PlanStatus = plan.Status
			}
		}
		mine, done := 0, 0
		for _, step := range planViews(c, intern.ID) {
			if step.Owner != store.RoleIntern {
				continue
			}
			mine++
			if step.Status == "Done" {
				done++
			} else if v.Stage == "" {
				v.Stage = step.Stage
			}
			if step.Status == "Overdue" {
				v.OverdueSteps++
			}
		}
		v.Progress = done * 100 / max(mine, 1)
		for _, stage := range c.Store.Data.Template.Stages {
			if stage.ID == v.Stage {
				v.Stage = stage.Title
				break
			}
		}
		for _, q := range questions {
			if q.AskedBy == intern.ID && q.Status == "Sent" {
				v.OpenQuestions++
			}
		}
		out.Totals.OverdueSteps += v.OverdueSteps
		out.Totals.OpenQuestions += v.OpenQuestions
		out.Interns = append(out.Interns, v)
	}
	out.Totals.Interns = len(out.Interns)
	for _, req := range c.Store.Requests("") {
		name, ok := owned[req.EmployeeID]
		if !ok || req.ApproverID != c.User.ID {
			continue
		}
		out.Requests = append(out.Requests, requestOverview{req.ID, req.Kind, req.Title, req.Type, req.Date, req.EmployeeID, name, req.Status})
		if req.Status == "pending" {
			out.Totals.PendingRequests++
		}
	}
	return ok(out)
}
