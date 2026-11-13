package tools

import (
	"sort"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

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
	// Названия просроченных шагов — чтобы ИИ мог сказать, на чём застрял стажёр. Тексты вопросов сюда не входят.
	OverdueTitles []string `json:"overdue_step_titles,omitempty"`
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
		v := overviewOf(c, intern, questions)
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

// overviewOf — прогресс одного стажёра: те же числа на дашборде руководителя и в ответе ИИ.
func overviewOf(c Ctx, intern mockdata.User, questions []store.Question) internOverview {
	v := internOverview{ID: intern.ID, Name: intern.Name, Buddy: c.Store.Name(intern.BuddyID), Week: c.Store.Week(&intern), PlanStatus: "missing"}
	for _, plan := range c.Store.Data.Tracker.Plans {
		if plan.InternID == intern.ID {
			v.PlanStatus = plan.Status
		}
	}
	// Жизненный цикл плана (BR-81): draft → approved | auto_activated — из состояния плана, а не из трекера.
	if p, ok := c.Store.Plan(intern.ID); ok {
		v.PlanStatus = p.Status
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
			v.OverdueTitles = append(v.OverdueTitles, step.Title)
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
	return v
}

// teamProgress — «как дела у моих стажёров, кто отстаёт» в чате: руководитель видит стажёров, у которых он
// руководитель, бадди — своих подопечных. Те же числа, что на дашборде, без текстов вопросов (02-ROLES).
// Первыми — кто отстаёт: больше просроченных шагов, затем меньший прогресс.
func teamProgress(c Ctx, _ map[string]any) Result {
	questions := c.Store.Questions()
	interns := []internOverview{}
	for _, u := range c.Store.Data.Team.Users {
		if !store.HasRole(&u, store.RoleIntern) || (u.LeadID != c.User.ID && u.BuddyID != c.User.ID) {
			continue
		}
		interns = append(interns, overviewOf(c, u, questions))
	}
	sort.SliceStable(interns, func(i, j int) bool {
		if interns[i].OverdueSteps != interns[j].OverdueSteps {
			return interns[i].OverdueSteps > interns[j].OverdueSteps
		}
		return interns[i].Progress < interns[j].Progress
	})
	behind := []string{}
	for _, v := range interns {
		if v.OverdueSteps > 0 {
			behind = append(behind, v.Name)
		}
	}
	return ok(map[string]any{"interns": interns, "behind": behind, "total": len(interns),
		"note": "Отстаёт — есть просроченные шаги плана. Тексты вопросов стажёров не показываются (02-ROLES)."})
}
