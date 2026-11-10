package tools

import (
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

func requestType(c Ctx, t string) (mockdata.RequestType, bool) {
	for _, rt := range c.Store.Data.Absence.RequestTypes {
		if rt.Type == t {
			return rt, true
		}
	}
	return mockdata.RequestType{}, false
}

func pageBody(c Ctx, id string) (mockdata.Page, bool) {
	for _, p := range readablePages(c) {
		if p.ID == id {
			return p, true
		}
	}
	return mockdata.Page{}, false
}

// BR-42: два пути — «сделай сам» (шаги и ссылка) или «сделаю за тебя».
func getAbsenceRules(c Ctx, args map[string]any) Result {
	rt, found := requestType(c, str(args, "type"))
	if !found {
		return fail("validation", "Неизвестный тип отсутствия.")
	}
	ss := c.Store.Data.Absence.SelfService
	data := map[string]any{"type": rt.Type, "title": rt.Title, "self_service": ss, "approver": c.Store.Name(c.User.LeadID)}
	if p, found := pageBody(c, ss.SourcePage); found {
		data["rules"] = p.Body
		data["source"] = map[string]any{"page_id": p.ID, "title": p.Title, "author": c.Store.Name(p.AuthorID),
			"updated_at": p.UpdatedAt, "label": TestInstructionLabel}
	}
	return ok(data)
}

type warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func precheckAbsence(c Ctx, args map[string]any) Result {
	day, valid := parseDate(str(args, "date"))
	if !valid {
		return fail("validation", "Дата в формате ГГГГ-ММ-ДД.")
	}
	rt, _ := requestType(c, str(args, "type"))
	var ws []warning
	if !day.After(c.Store.Today) {
		ws = append(ws, warning{"PAST_DATE", "Дата уже прошла"})
	} else if rt.MinNoticeWorkdays > 0 && workdaysBetween(c.Store.Today, day) < rt.MinNoticeWorkdays {
		ws = append(ws, warning{"SHORT_NOTICE", "По тестовой инструкции отгул подают за 2 рабочих дня"})
	}
	date := day.Format(time.DateOnly)
	for _, e := range c.Store.Data.Calendar.Events {
		if strings.HasPrefix(e.Start, date) && containsStr(e.Participants, c.User.ID) && e.Type != "deadline" {
			ws = append(ws, warning{"MEETING_CONFLICT", "В этот день у тебя встреча: " + e.Title})
		}
	}
	for _, s := range planViews(c, c.User.ID) {
		if s.DueDate == date && s.Status != "Done" && s.Owner == store.RoleIntern {
			ws = append(ws, warning{"STEP_DEADLINE", "В этот день срок шага плана: " + s.Title})
		}
	}
	return ok(map[string]any{"date": date, "warnings": ws})
}

func createAbsenceRequest(c Ctx, args map[string]any) Result {
	rt, found := requestType(c, str(args, "type"))
	if !found {
		return fail("validation", "Неизвестный тип отсутствия.")
	}
	day, valid := parseDate(str(args, "date"))
	if !valid || !day.After(c.Store.Today) {
		return fail("validation", "Укажи будущую дату")
	}
	if mockFail("absence") {
		return fail("unavailable", "Сервис отсутствий временно недоступен. Черновик сохранён — можно повторить.")
	}
	r := &store.Request{ID: c.Store.NextID("ABS"), Kind: "absence", Type: rt.Type, Date: day.Format(time.DateOnly),
		Comment: str(args, "comment"), EmployeeID: c.User.ID, ApproverID: c.User.LeadID, Status: "pending",
		CreatedAt: time.Now().Format(time.RFC3339)}
	c.Store.AddRequest(r)
	return Result{OK: true, Data: r, Receipt: map[string]string{"id": r.ID, "status": "pending", "approver": c.Store.Name(r.ApproverID)}}
}

func listMyRequests(c Ctx, _ map[string]any) Result {
	return ok(c.Store.Requests(c.User.ID))
}

func getRequiredAccesses(c Ctx, _ map[string]any) Result {
	week := c.Store.Week(c.User)
	status := map[string]string{}
	for _, st := range c.Store.Data.Access.CurrentState[c.User.ID] {
		status[st.System] = st.Status
	}
	for _, r := range c.Store.Requests(c.User.ID) {
		if r.Kind == "access" {
			status[r.System] = r.Status
		}
	}
	var out []map[string]string
	for _, s := range c.Store.Data.Access.Systems {
		// BR-40: системы чужих отделов стажёру не показываются вовсе
		if s.Department != c.Store.Data.Team.Department.ID || s.WeekFrom == nil || *s.WeekFrom > week {
			continue
		}
		st := status[s.ID]
		if st == "" {
			st = "not_requested"
		}
		out = append(out, map[string]string{"system": s.ID, "title": s.Title, "status": st})
	}
	return ok(map[string]any{"week": week, "systems": out})
}

func findSystem(c Ctx, id string) *mockdata.System {
	for i := range c.Store.Data.Access.Systems {
		if c.Store.Data.Access.Systems[i].ID == id {
			return &c.Store.Data.Access.Systems[i]
		}
	}
	return nil
}

func checkAccess(c Ctx, id string) *Error {
	sys := findSystem(c, id)
	if sys == nil {
		return &Error{Code: "not_found", Message: "Такой системы нет."}
	}
	if sys.Department != c.Store.Data.Team.Department.ID || (sys.RequestableByIntern != nil && !*sys.RequestableByIntern) {
		// BR-44: объяснить и предложить запрос тому, у кого есть право
		return &Error{Code: "forbidden", Message: "Эта система вне зоны отдела.", WhoCan: c.Store.Name(c.User.LeadID)}
	}
	return nil
}

func requestAccess(c Ctx, args map[string]any) Result {
	if e := checkAccess(c, str(args, "system")); e != nil {
		return Result{Error: e}
	}
	sys := findSystem(c, str(args, "system"))
	if mockFail("access") {
		return fail("unavailable", "Сервис доступов временно недоступен. Черновик сохранён — можно повторить.")
	}
	r := &store.Request{ID: c.Store.NextID("ACC"), Kind: "access", System: sys.ID, Comment: str(args, "reason"),
		EmployeeID: c.User.ID, ApproverID: c.User.LeadID, Status: "pending", CreatedAt: time.Now().Format(time.RFC3339)}
	c.Store.AddRequest(r)
	return Result{OK: true, Data: r, Receipt: map[string]string{"id": r.ID, "status": "pending", "approver": c.Store.Name(r.ApproverID)}}
}
