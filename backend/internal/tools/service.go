package tools

import (
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Имитация корпоративного сервиса заявок в стиле Jira (absence-service.json, access-service.json):
// заявка с номером, согласующим, статусом и историей; согласует руководитель с кодом причины отказа.
// Сюда же — путь «сделай сам» (BR-42): стажёр заполняет форму сервиса без Напарника.

const (
	SourceAgent = "agent" // «сделаю за тебя»: черновик Напарника → подтверждение стажёра
	SourceSelf  = "self"  // «сделай сам»: стажёр заполнил форму сервиса
)

// created — заявка принята сервисом: первая запись истории и сохранение.
// По умолчанию заявку готовил Напарник; CreateSelf переписывает источник.
func created(c Ctx, r *store.Request) {
	r.Source = SourceAgent
	r.History = []store.Event{
		{At: r.CreatedAt, By: c.User.ID, Text: "Черновик подготовил Напарник, " + c.Store.Name(c.User.ID) + " подтвердил(а)"},
		{At: r.CreatedAt, By: r.ApproverID, Text: "Заявка на согласовании: " + c.Store.Name(r.ApproverID)},
	}
	c.Store.AddRequest(r)
}

// CreateSelf — стажёр сам оформляет заявку в форме сервиса (BR-42, «сделай сам»).
// Нажатие «Отправить» в форме — и есть подтверждение человека, черновик не нужен.
// Проверки те же, что у агента: права, даты, системы своего отдела (BR-40, BR-44).
func CreateSelf(c Ctx, kind string, args map[string]any) Result {
	return atomicResult(c, func(c Ctx) Result { return createSelf(c, kind, args) })
}

func createSelf(c Ctx, kind string, args map[string]any) Result {
	tool := map[string]string{"absence": "create_absence_request", "access": "request_access"}[kind]
	if tool == "" {
		return fail("validation", "Неизвестный тип заявки.")
	}
	res := Run(c, tool, args)
	if !res.OK {
		return res
	}
	r := res.Data.(*store.Request)
	upd, _ := c.Store.UpdateRequest(r.ID, func(x *store.Request) {
		x.Source = SourceSelf
		x.History[0].Text = c.Store.Name(c.User.ID) + " оформил(а) заявку сам(а) в сервисе"
	})
	res.Data = upd
	return res
}

// reasonCodes — коды причин отказа для сервиса заявки.
func reasonCodes(c Ctx, kind string) []mockdata.ReasonCode {
	if kind == "access" {
		return c.Store.Data.Access.ReasonCodes
	}
	return c.Store.Data.Absence.ReasonCodes
}

// Decide — согласующий одобряет или отклоняет заявку (status_flow в мок-данных сервисов).
// Решает только согласующий этой заявки; отказ — с кодом причины, «Другое» — с комментарием.
func Decide(c Ctx, id, decision, reason, comment string) (store.Request, *Error) {
	return atomic(c, func(c Ctx) (store.Request, *Error) { return decide(c, id, decision, reason, comment) })
}

func decide(c Ctx, id, decision, reason, comment string) (store.Request, *Error) {
	var req *store.Request
	for _, r := range c.Store.Requests("") {
		if r.ID == id {
			req = &r
		}
	}
	switch {
	case req == nil:
		return store.Request{}, &Error{Code: "not_found", Message: "Заявка не найдена."}
	case req.ApproverID != c.User.ID:
		return store.Request{}, &Error{Code: "forbidden", Message: "Решение принимает согласующий заявки."}
	case req.Status != "pending" && ((decision == "approve" && (req.Status == "approved" || req.Status == "granted") && req.Decision == comment) || (decision == "reject" && req.Status == "rejected" && req.ReasonCode == reason && req.Decision == comment)):
		return *req, nil
	case req.Status != "pending":
		return store.Request{}, &Error{Code: "validation", Message: "По заявке уже есть решение."}
	}
	status, text := "", ""
	switch decision {
	case "approve":
		status, text = "approved", "Согласовано"
		if req.Kind == "access" {
			status, text = "granted", "Доступ выдан"
		}
	case "reject":
		var rc *mockdata.ReasonCode
		for _, x := range reasonCodes(c, req.Kind) {
			if x.Code == reason {
				rc = &x
			}
		}
		if rc == nil {
			return store.Request{}, &Error{Code: "validation", Message: "Выбери причину отказа."}
		}
		if rc.Code == "OTHER" && comment == "" {
			return store.Request{}, &Error{Code: "validation", Message: "Для «Другое» нужен комментарий."}
		}
		status, text = "rejected", "Отклонено: "+rc.Text
	default:
		return store.Request{}, &Error{Code: "validation", Message: "Решение: approve или reject."}
	}
	now := time.Now().Format(time.RFC3339)
	if comment != "" {
		text += ". Комментарий: " + comment
	}
	out, _ := c.Store.UpdateRequest(id, func(r *store.Request) {
		r.Status, r.DecidedAt, r.Decision = status, now, comment
		if decision == "reject" {
			r.ReasonCode = reason
		}
		r.History = append(r.History, store.Event{At: now, By: c.User.ID, Text: text + " — " + c.Store.Name(c.User.ID)})
		if status == "granted" && r.StepID != "" {
			r.History = append(r.History, store.Event{At: now, By: r.EmployeeID, Text: "Шаг плана можно отметить выполненным — стажёр подтверждает"})
		}
	})
	return out, nil
}

// Catalog — что можно оформить в сервисе: типы отсутствий, системы, порядок «сделай сам» и коды причин.
// Стажёру — все системы своего отдела, которые он вправе запросить, с отметкой «нужен сейчас» или «по плану
// с N-й недели» (заявку раньше срока руководитель может отклонить с кодом NOT_NEEDED_YET).
// Чужие системы и системы с ПДн не показываются (BR-40). Остальным ролям — только справочник кодов.
func Catalog(c Ctx) map[string]any {
	abs := c.Store.Data.Absence
	out := map[string]any{
		"absence": map[string]any{"types": abs.RequestTypes, "self_service": abs.SelfService, "reason_codes": abs.ReasonCodes,
			"approver": c.Store.Name(c.User.LeadID)},
		"access": map[string]any{"systems": []map[string]string{}, "reason_codes": c.Store.Data.Access.ReasonCodes},
	}
	if store.HasRole(c.User, store.RoleIntern) {
		out["access"].(map[string]any)["systems"] = requestableSystems(c)
	}
	return out
}

// SystemView — система в форме заявки на доступ.
type SystemView struct {
	System   string `json:"system"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	WeekFrom int    `json:"week_from"`
	Now      bool   `json:"now"` // нужен на текущей неделе по плану
}

func requestableSystems(c Ctx) []SystemView {
	week, status := c.Store.Week(c.User), accessStatus(c)
	var out []SystemView
	for _, s := range c.Store.Data.Access.Systems {
		if checkAccess(c, s.ID) != nil || s.WeekFrom == nil {
			continue
		}
		st := status[s.ID]
		if st == "" {
			st = "not_requested"
		}
		out = append(out, SystemView{s.ID, s.Title, st, *s.WeekFrom, *s.WeekFrom <= week})
	}
	return out
}
