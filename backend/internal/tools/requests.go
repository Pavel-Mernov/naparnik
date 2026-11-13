package tools

import (
	"github.com/worker4556/naparnik/backend/internal/adapters"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// SelfServiceHint — где в прототипе путь «сделай сам»: форма имитации сервиса (BR-42).
const SelfServiceHint = "раздел «Заявки» → кнопка «Оформить самому»"

func requestType(c Ctx, t string) (mockdata.RequestType, bool) {
	for _, rt := range absenceCatalog(c).RequestTypes {
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
	ss := absenceCatalog(c).SelfService
	data := map[string]any{"type": rt.Type, "title": rt.Title, "self_service": ss, "approver": c.Store.Name(c.User.LeadID),
		"self_service_in_app": SelfServiceHint}
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
	for _, e := range calendarEvents(c) {
		if strings.HasPrefix(e.Start, date) && containsStr(e.Participants, c.User.ID) && e.Type != "deadline" {
			ws = append(ws, warning{"MEETING_CONFLICT", "В этот день у тебя встреча: " + e.Title})
		}
	}
	for _, s := range planViews(c, c.User.ID) {
		if s.DueDate == date && s.Status != "Done" && s.Owner == store.RoleIntern {
			ws = append(ws, warning{"STEP_DEADLINE", "В этот день срок шага плана: " + s.Title})
		}
	}
	if dup := activeRequest(c, sameAbsence(rt.Type, date)); dup != nil {
		ws = append(ws, warning{"DUPLICATE", "На эту дату уже есть заявка " + dup.ID + " (" + statusRU(dup.Status) + ")"})
	}
	return ok(map[string]any{"date": date, "warnings": ws})
}

// activeRequest — заявка пользователя, которая ещё в силе (не отклонена), подходящая под условие.
func activeRequest(c Ctx, match func(store.Request) bool) *store.Request {
	for _, r := range c.Store.Requests(c.User.ID) {
		if r.Status != "rejected" && match(r) {
			return &r
		}
	}
	return nil
}

func duplicateError(r *store.Request) Result {
	return fail("conflict", "Такая заявка уже есть: "+r.ID+" ("+statusRU(r.Status)+"). Повторно отправлять не нужно.")
}

func statusRU(s string) string {
	switch s {
	case "approved":
		return "одобрена"
	case "granted":
		return "доступ выдан"
	}
	return "на согласовании"
}

func sameAbsence(t, date string) func(store.Request) bool {
	return func(r store.Request) bool { return r.Kind == "absence" && r.Type == t && r.Date == date }
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
	if dup := activeRequest(c, sameAbsence(rt.Type, day.Format(time.DateOnly))); dup != nil {
		return duplicateError(dup)
	}
	ctx, cancel := svcCtx()
	defer cancel()
	rc, err := c.svc().Absence.Submit(ctx, adapters.AbsenceSubmit{Key: "abs:" + c.User.ID + ":" + rt.Type + ":" + day.Format(time.DateOnly),
		EmployeeID: c.User.ID, ApproverID: c.User.LeadID, Type: rt.Type, Date: day.Format(time.DateOnly), Comment: str(args, "comment")})
	if err != nil {
		return serviceError(err, "Сервис отсутствий")
	}
	r := &store.Request{ID: rc.ID, Kind: "absence", Type: rt.Type, Date: day.Format(time.DateOnly),
		Comment: str(args, "comment"), EmployeeID: c.User.ID, ApproverID: c.User.LeadID, Status: rc.Status,
		CreatedAt: time.Now().Format(time.RFC3339)}
	created(c, r)
	return Result{OK: true, Data: r, Receipt: map[string]string{"id": r.ID, "status": r.Status, "approver": c.Store.Name(r.ApproverID)}}
}

func listMyRequests(c Ctx, _ map[string]any) Result {
	return ok(c.Store.Requests(c.User.ID))
}

// accessStatus — статус каждой системы для стажёра: выданные из сервиса доступов и его заявки поверх.
func accessStatus(c Ctx) map[string]string {
	status := map[string]string{}
	for _, st := range grantedAccess(c) {
		status[st.System] = st.Status
	}
	for _, r := range c.Store.Requests(c.User.ID) {
		if r.Kind == "access" {
			status[r.System] = r.Status
		}
	}
	return status
}

func getRequiredAccesses(c Ctx, _ map[string]any) Result {
	week := c.Store.Week(c.User)
	var out []map[string]string
	for _, s := range requestableSystems(c) {
		if s.Now {
			out = append(out, map[string]string{"system": s.System, "title": s.Title, "status": s.Status})
		}
	}
	return ok(map[string]any{"week": week, "systems": out, "self_service_in_app": SelfServiceHint})
}

func findSystem(c Ctx, id string) *mockdata.System {
	systems := accessCatalog(c).Systems
	for i := range systems {
		if systems[i].ID == id {
			return &systems[i]
		}
	}
	return nil
}

func checkAccess(c Ctx, id string) *Error {
	sys := findSystem(c, id)
	if sys == nil {
		return &Error{Code: "not_found", Message: "Такой системы нет."}
	}
	if sys.Department != c.Store.Data.Team.Department.ID || sys.WeekFrom == nil || *sys.WeekFrom < 1 || (sys.RequestableByIntern != nil && !*sys.RequestableByIntern) {
		// BR-40, BR-45: без названия закрытой системы и без предложения заявки на неё.
		return &Error{Code: "forbidden", Message: "Этот доступ стажёру недоступен. Обсуди задачу с бадди или руководителем.", WhoCan: c.Store.Name(c.User.LeadID)}
	}
	return nil
}

func requestAccess(c Ctx, args map[string]any) Result {
	if e := checkAccess(c, str(args, "system")); e != nil {
		return Result{Error: e}
	}
	sys := findSystem(c, str(args, "system"))
	if dup := activeRequest(c, func(r store.Request) bool { return r.Kind == "access" && r.System == sys.ID }); dup != nil {
		return duplicateError(dup)
	}
	ctx, cancel := svcCtx()
	defer cancel()
	rc, err := c.svc().Access.Submit(ctx, adapters.AccessSubmit{Key: "acc:" + c.User.ID + ":" + sys.ID, EmployeeID: c.User.ID,
		ApproverID: c.User.LeadID, System: sys.ID, Reason: str(args, "reason")})
	if err != nil {
		return serviceError(err, "Сервис доступов")
	}
	r := &store.Request{ID: rc.ID, Kind: "access", System: sys.ID, Title: sys.Title, Comment: str(args, "reason"),
		EmployeeID: c.User.ID, ApproverID: c.User.LeadID, Status: rc.Status, CreatedAt: time.Now().Format(time.RFC3339)}
	if step := relatedAccessStep(c, sys.ID); step != nil {
		r.StepID = step.ID // SC-27: после выдачи доступа шаг предлагается отметить
	}
	created(c, r)
	return Result{OK: true, Data: r, Receipt: map[string]string{"id": r.ID, "status": r.Status, "approver": c.Store.Name(r.ApproverID)}}
}
