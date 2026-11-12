package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/practice"
	"github.com/worker4556/naparnik/backend/internal/store"
)

const practiceAttempts = 3

type PracticeView struct {
	ID              string             `json:"id"`
	InternID        string             `json:"intern_id"`
	InternName      string             `json:"intern_name"`
	Step            accessPlanStep     `json:"step"`
	Offer           string             `json:"offer"`
	OfferedToday    bool               `json:"offered_today"`
	Stage           string             `json:"stage"`
	TaskStatus      string             `json:"task_status"`
	TaskVersion     int                `json:"task_version"`
	ApprovalDue     string             `json:"approval_due,omitempty"`
	ApprovalExpired bool               `json:"approval_expired"`
	Available       bool               `json:"available"`
	General         []practice.Term    `json:"general_terms"`
	Team            []practice.Term    `json:"team_terms"`
	Rules           []practice.Rule    `json:"team_rules"`
	Sources         []practice.Source  `json:"sources"`
	Task            *practice.Task     `json:"task,omitempty"`
	Checklist       []string           `json:"checklist,omitempty"`
	Attempts        []practice.Attempt `json:"attempts,omitempty"`
	BestAttempt     *practice.Attempt  `json:"best_attempt,omitempty"`
	AttemptsCount   int                `json:"attempts_count"`
	ErrorTopics     []string           `json:"error_topics"`
	HelpAvailable   bool               `json:"help_available"`
}

func practiceTarget(c Ctx, internID, id string) (practice.Template, *Error) {
	intern, found := c.Store.User(internID)
	if !found || !store.HasRole(intern, store.RoleIntern) || !((c.User.ID == internID && store.HasRole(c.User, store.RoleIntern)) ||
		(intern.BuddyID == c.User.ID && store.HasRole(c.User, store.RoleBuddy))) {
		return practice.Template{}, &Error{Code: "forbidden", Message: "Практика доступна стажёру и его бадди."}
	}
	approved := false
	for _, plan := range c.Store.Data.Tracker.Plans {
		if plan.InternID == internID && plan.Status == "approved" {
			approved = true
		}
	}
	for _, t := range c.Store.Data.Practice.Templates {
		if t.ID != id || !approved {
			continue
		}
		for _, step := range c.Store.PlanSteps(internID) {
			tpl, ok := c.Store.TemplateStep(step.StepID)
			if ok && step.StepID == t.StepID && tpl.Owner == store.RoleIntern && (tpl.Type == "task" || tpl.Type == "study") {
				return t, nil
			}
		}
	}
	return practice.Template{}, &Error{Code: "not_found", Message: "У этого шага нет доступной практики."}
}

func practiceIDs(t practice.Template) []string {
	ids := append([]string{}, t.Task.Sources...)
	for _, term := range t.Team {
		ids = append(ids, term.Sources...)
	}
	for _, rule := range t.Rules {
		ids = append(ids, rule.Sources...)
	}
	return ids
}

func practiceSources(c Ctx, t practice.Template) ([]practice.Source, bool) {
	pages := map[string]mockdata.Page{}
	for _, p := range readablePages(c) {
		pages[p.ID] = p
	}
	seen, complete := map[string]bool{}, true
	out := []practice.Source{}
	for _, id := range practiceIDs(t) {
		p, ok := pages[id]
		if !ok {
			complete = false
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, practice.Source{ID: p.ID, Title: p.Title, Author: c.Store.Name(p.AuthorID), Updated: p.UpdatedAt, Body: p.Body})
	}
	return out, complete && len(t.Task.Criteria) > 0
}

func practiceState(c Ctx, intern, id string) practice.State {
	st, ok := c.Store.Practice(intern, id)
	if !ok {
		st = practice.State{InternID: intern, TemplateID: id, Stage: "offered", TaskStatus: "Pending", Attempts: []practice.Attempt{}, Events: []practice.Event{}}
	}
	return st
}

func practiceEvent(c Ctx, st *practice.State, kind string) {
	st.Events = append(st.Events, practice.Event{Type: kind, At: time.Now().Format(time.RFC3339), By: c.User.ID, Stage: st.Stage})
}

func approvalExpired(c Ctx, st practice.State) bool {
	return st.TaskStatus != "Approved" && st.ApprovalDue != "" && c.Store.Today.Format(time.DateOnly) >= st.ApprovalDue
}

func practiceView(c Ctx, t practice.Template, st practice.State) PracticeView {
	step, _ := c.Store.TemplateStep(t.StepID)
	sources, available := practiceSources(c, t)
	v := PracticeView{ID: t.ID, InternID: st.InternID, InternName: c.Store.Name(st.InternID), Step: accessPlanStep{t.StepID, step.Title}, Offer: t.Offer,
		Stage: st.Stage, TaskStatus: st.TaskStatus, TaskVersion: st.Version, ApprovalDue: st.ApprovalDue, ApprovalExpired: approvalExpired(c, st),
		Available: available, Sources: []practice.Source{}, General: t.General, Team: []practice.Term{}, Rules: []practice.Rule{}, ErrorTopics: []string{}, AttemptsCount: len(st.Attempts)}
	for _, s := range c.Store.PlanSteps(st.InternID) {
		if s.StepID == t.StepID {
			v.OfferedToday = s.Status != "Done" && (s.Status == "Today" || s.Status == "Overdue" || s.DueDate <= c.Store.Today.Format(time.DateOnly))
		}
	}
	if !available {
		return v
	} // BR-40: ни названий недоступных источников, ни связанных с ними текстов.
	v.Team, v.Rules, v.Sources = t.Team, t.Rules, sources
	if st.TaskStatus == "Approved" || c.User.ID != st.InternID && st.TaskStatus == "Draft" {
		task := t.Task
		task.Text = st.TaskText
		v.Task = &task
	}
	if st.Stage == "real_task" {
		v.Checklist = t.Checklist
	}
	seen := map[string]bool{}
	for i := range st.Attempts {
		a := st.Attempts[i]
		if v.BestAttempt == nil || a.Review.Score >= v.BestAttempt.Review.Score {
			v.BestAttempt = &a
		}
		for _, check := range a.Review.Checks {
			if !check.Passed && check.Criterion > 0 && check.Criterion <= len(t.Task.Criteria) {
				topic := t.Task.Criteria[check.Criterion-1]
				if check.Criterion <= len(t.Task.CheckTopics) {
					topic = t.Task.CheckTopics[check.Criterion-1]
				}
				if !seen[topic] {
					seen[topic] = true
					v.ErrorTopics = append(v.ErrorTopics, topic)
				}
			}
		}
	}
	if c.User.ID == st.InternID {
		v.Attempts = st.Attempts
		v.BestAttempt = nil
	}
	v.HelpAvailable = len(st.Attempts) >= practiceAttempts && !practicePassed(st)
	return v
}

func practicePassed(st practice.State) bool {
	for _, a := range st.Attempts {
		if a.Review.Passed {
			return true
		}
	}
	return false
}

func PracticeList(c Ctx) Result {
	if !store.HasRole(c.User, store.RoleIntern) && !store.HasRole(c.User, store.RoleBuddy) {
		return fail("forbidden", "Практика доступна стажёру и бадди.")
	}
	out := []PracticeView{}
	for _, intern := range c.Store.Data.Team.Users {
		for _, t := range c.Store.Data.Practice.Templates {
			if _, e := practiceTarget(c, intern.ID, t.ID); e == nil {
				out = append(out, practiceView(c, t, practiceState(c, intern.ID, t.ID)))
			}
		}
	}
	return ok(out)
}

func getPractice(c Ctx, args map[string]any) Result {
	list := PracticeList(c)
	if !list.OK {
		return list
	}
	for _, v := range list.Data.([]PracticeView) {
		if str(args, "practice_id") == "" || str(args, "practice_id") == v.ID {
			return ok(v)
		}
	}
	return fail("not_found", "У текущего плана нет такой практики.")
}

func advancePracticeTool(c Ctx, args map[string]any) Result {
	r := getPractice(c, args)
	if !r.OK {
		return r
	}
	v := r.Data.(PracticeView)
	return AdvancePractice(c, c.User.ID, v.ID, str(args, "action"), str(args, "expected_stage"))
}

// Подготовка — внутренняя запись черновика, отправки людям нет. LLM вызывается после транзакции.
func PreparePractice(c Ctx, intern, id string) (practice.ModelInput, bool, *Error) {
	type preparation struct {
		Input  practice.ModelInput
		Needed bool
	}
	p, e := atomic(c, func(c Ctx) (preparation, *Error) {
		t, e := practiceTarget(c, intern, id)
		if e != nil {
			return preparation{}, e
		}
		c.Store.EnsurePracticeSources(practiceIDs(t))
		sources, available := practiceSources(c, t)
		if !available {
			return preparation{}, &Error{Code: "unavailable", Message: "Материалы практики пока недоступны."}
		}
		st, exists := c.Store.Practice(intern, id)
		if !exists {
			st = practiceState(c, intern, id)
			practiceEvent(c, &st, "practice_offered")
			c.Store.PutPractice(st)
		}
		return preparation{practice.ModelInput{Task: t.Task, Sources: sources}, st.TaskStatus == "Pending"}, nil
	})
	return p.Input, p.Needed, e
}

func FinishPracticeGeneration(c Ctx, intern, id, text string) Result {
	return atomicResult(c, func(c Ctx) Result {
		t, e := practiceTarget(c, intern, id)
		if e != nil {
			return Result{Error: e}
		}
		if _, ok := practiceSources(c, t); !ok {
			return fail("forbidden", "Материалы практики больше недоступны.")
		}
		st := practiceState(c, intern, id)
		if st.TaskStatus == "Pending" {
			st.TaskStatus, st.TaskText, st.Version = "Draft", text, st.Version+1
			st.CreatedDay = c.Store.Today.Format(time.DateOnly)
			due := c.Store.Today.AddDate(0, 0, 1)
			for due.Weekday() == time.Saturday || due.Weekday() == time.Sunday {
				due = due.AddDate(0, 0, 1)
			}
			st.ApprovalDue = due.Format(time.DateOnly)
			practiceEvent(c, &st, "practice_task_prepared")
			c.Store.PutPractice(st)
		}
		return ok(practiceView(c, t, st))
	})
}

func ApprovePractice(c Ctx, intern, id, text string, version int) Result {
	return atomicResult(c, func(c Ctx) Result {
		t, e := practiceTarget(c, intern, id)
		if e != nil {
			return Result{Error: e}
		}
		if c.User.ID == intern || !store.HasRole(c.User, store.RoleBuddy) {
			return fail("forbidden", "Задание одобряет бадди стажёра.")
		}
		if _, ok := practiceSources(c, t); !ok {
			return fail("forbidden", "Материалы практики недоступны.")
		}
		st := practiceState(c, intern, id)
		text = strings.TrimSpace(text)
		if st.TaskStatus == "Approved" && st.TaskText == text && st.Version == version+1 {
			return ok(practiceView(c, t, st))
		}
		if st.TaskStatus != "Draft" || st.Version != version {
			return fail("validation", "Черновик уже изменён; обнови практику.")
		}
		dataset := strings.Fields(t.Task.Dataset)
		if text == "" || len(text) > 6000 || len(dataset) == 0 || !strings.Contains(text, dataset[0]) {
			return fail("validation", "Оставь в задании учебный набор данных из шаблона.")
		}
		st.TaskText, st.TaskStatus, st.ApprovedBy, st.Version = text, "Approved", c.User.ID, st.Version+1
		practiceEvent(c, &st, "practice_task_approved")
		c.Store.PutPractice(st)
		return ok(practiceView(c, t, st))
	})
}

func AdvancePractice(c Ctx, intern, id, action, expectedStage string) Result {
	return atomicResult(c, func(c Ctx) Result {
		t, e := practiceTarget(c, intern, id)
		if e != nil {
			return Result{Error: e}
		}
		if c.User.ID != intern {
			return fail("forbidden", "Этап практики выбирает стажёр.")
		}
		st := practiceState(c, intern, id)
		if action != "start" && action != "next" && action != "skip_stage" && action != "skip_all" {
			return fail("validation", "Неизвестное действие практики.")
		}
		if (action == "next" || action == "skip_stage") && expectedStage == "" {
			return fail("validation", "Передай текущий этап практики.")
		}
		if expectedStage != "" && st.Stage != expectedStage {
			return ok(practiceView(c, t, st))
		}
		if st.Stage == "real_task" || action == "start" && st.Stage != "offered" {
			return ok(practiceView(c, t, st))
		}
		if action == "skip_all" {
			st.Stage = "real_task"
		} else {
			switch st.Stage {
			case "offered":
				st.Stage = "general_terms"
			case "general_terms":
				st.Stage = "team_terms"
			case "team_terms":
				st.Stage = "training"
				if approvalExpired(c, st) {
					st.Stage = "real_task"
				}
			case "training":
				if action != "skip_stage" && !practicePassed(st) && !approvalExpired(c, st) {
					return fail("validation", "Можно дождаться одобрения, пройти задание или пропустить его.")
				}
				st.Stage = "real_task"
			}
		}
		kind := "practice_stage_changed"
		if action == "start" {
			kind = "practice_started"
		}
		if strings.HasPrefix(action, "skip") {
			kind = "practice_skipped"
		}
		practiceEvent(c, &st, kind)
		c.Store.PutPractice(st)
		return ok(practiceView(c, t, st))
	})
}

func practiceAttemptScope(c Ctx, intern, id, key, answer string, version int) (string, string, *Error) {
	if c.User.ID != intern {
		return "", "", &Error{Code: "forbidden", Message: "Отправлять попытки может только стажёр."}
	}
	if key == "" || len(key) > 128 || strings.TrimSpace(answer) == "" || len(answer) > 12000 {
		return "", "", &Error{Code: "validation", Message: "Нужны ответ до 12000 символов и Idempotency-Key до 128 символов."}
	}
	raw, _ := json.Marshal([]any{intern, id, version, answer})
	return c.User.ID + ":practice:" + id + ":" + key, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func previousPracticeAttempt(c Ctx, scope, fingerprint string) (*PracticeView, *Error) {
	if op, ok := c.Store.Operation(scope); ok {
		if op.Fingerprint != fingerprint {
			return nil, &Error{Code: "validation", Message: "Этот ключ уже использован для другой попытки."}
		}
		var v PracticeView
		if json.Unmarshal(op.Result, &v) != nil {
			return nil, &Error{Code: "unavailable", Message: "Не удалось прочитать результат попытки."}
		}
		return &v, nil
	}
	return nil, nil
}

func CheckPracticeAttempt(c Ctx, intern, id, key, answer string, version int) (practice.ModelInput, *PracticeView, *Error) {
	t, e := practiceTarget(c, intern, id)
	if e != nil {
		return practice.ModelInput{}, nil, e
	}
	scope, fp, e := practiceAttemptScope(c, intern, id, key, answer, version)
	if e != nil {
		return practice.ModelInput{}, nil, e
	}
	if _, ok := practiceSources(c, t); !ok {
		return practice.ModelInput{}, nil, &Error{Code: "forbidden", Message: "Материалы практики недоступны."}
	}
	if prior, e := previousPracticeAttempt(c, scope, fp); prior != nil || e != nil {
		return practice.ModelInput{}, prior, e
	}
	st := practiceState(c, intern, id)
	if st.Stage != "training" || st.TaskStatus != "Approved" || st.Version != version || len(st.Attempts) >= practiceAttempts || practicePassed(st) {
		return practice.ModelInput{}, nil, &Error{Code: "validation", Message: "Задание изменилось, практика завершена или попытки закончились."}
	}
	sources, ok := practiceSources(c, t)
	if !ok {
		return practice.ModelInput{}, nil, &Error{Code: "forbidden", Message: "Материалы практики недоступны."}
	}
	task := t.Task
	task.Text = st.TaskText
	return practice.ModelInput{Task: task, Sources: sources, Answer: answer}, nil, nil
}

func CommitPracticeAttempt(c Ctx, intern, id, key, answer string, version int, review practice.Review) Result {
	return atomicResult(c, func(c Ctx) Result {
		_, prior, e := CheckPracticeAttempt(c, intern, id, key, answer, version)
		if e != nil {
			return Result{Error: e}
		}
		if prior != nil {
			return ok(*prior)
		}
		t, _ := practiceTarget(c, intern, id)
		st := practiceState(c, intern, id)
		st.Attempts = append(st.Attempts, practice.Attempt{Number: len(st.Attempts) + 1, Answer: answer, Review: review})
		practiceEvent(c, &st, "practice_attempt_submitted")
		c.Store.PutPractice(st)
		view := practiceView(c, t, st)
		raw, _ := json.Marshal(view)
		scope, fp, _ := practiceAttemptScope(c, intern, id, key, answer, version)
		c.Store.PutOperation(scope, store.Operation{Fingerprint: fp, Result: raw})
		return ok(view)
	})
}

func PracticeHelpDraft(c Ctx, intern, id string) Result {
	return atomicResult(c, func(c Ctx) Result {
		t, e := practiceTarget(c, intern, id)
		if e != nil {
			return Result{Error: e}
		}
		if c.User.ID != intern {
			return fail("forbidden", "Вопрос подтверждает сам стажёр.")
		}
		st := practiceState(c, intern, id)
		if len(st.Attempts) < practiceAttempts || practicePassed(st) {
			return fail("validation", "Помощь после трёх неудачных попыток пока не требуется.")
		}
		if previous, ok := c.Store.ActionDraft(st.HelpDraftID); ok && previous.Cancelled {
			st.HelpDraftID = "" // новый запрос помощи после явной отмены создаёт новый черновик
		}
		if st.HelpDraftID == "" {
			internUser, _ := c.Store.User(intern)
			view := practiceView(c, t, st)
			args := map[string]any{"title": "Помощь с учебным заданием", "text": "Нужна помощь с учебным заданием перед шагом «" + view.Step.Title + "». После трёх попыток остались темы: " + strings.Join(view.ErrorTopics, "; "), "addressee_id": internUser.BuddyID, "reason": "Три попытки учебной практики", "urgent": false}
			if e := Check(c, "send_question", args); e != nil {
				return Result{Error: e}
			}
			st.HelpDraftID = c.Store.NextID("DRAFT")
			c.Store.PutDraft(store.ActionDraft{ID: st.HelpDraftID, UserID: intern, Tool: "send_question", Args: args})
			c.Store.PutPractice(st)
		}
		d, _ := c.Store.ActionDraft(st.HelpDraftID)
		return ok(map[string]any{"id": d.ID, "tool": d.Tool, "args": d.Args, "version": d.Version})
	})
}
