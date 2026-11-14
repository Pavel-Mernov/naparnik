package tools

import (
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Смена адресата вопроса: «не ко мне» (SC-08, BR-17), переадресация бадди (SC-09, BR-16),
// назначение открытого вопроса владельцем базы (SC-22, BR-37). Маршрут сохраняется в вопросе —
// бадди видит, где ИИ ошибся с адресатом (SCR-B2).

func findQuestion(c Ctx, id string) (store.Question, bool) {
	for _, q := range c.Store.Questions() {
		if q.ID == id {
			return q, true
		}
	}
	return store.Question{}, false
}

// reroute — общий шаг: новый адресат, запись в маршрут, событие для уведомлений и метрик.
func reroute(c Ctx, q store.Question, to, kind string) (store.Question, *Error) {
	target, ok := c.Store.User(to)
	if !ok {
		return q, &Error{Code: "not_found", Message: "Такого человека нет в команде."}
	}
	if to == q.AddresseeID {
		return q, &Error{Code: "validation", Message: "Вопрос и так у этого человека."}
	}
	if store.HasRole(target, store.RoleIntern) || to == q.AskedBy {
		return q, &Error{Code: "validation", Message: "Вопрос нужно отправить сотруднику, а не стажёру."}
	}
	from := q.AddresseeID
	now := time.Now().Format(time.RFC3339)
	out, _ := c.Store.UpdateQuestion(q.ID, func(x *store.Question) {
		x.Route = append(x.Route, store.RouteStep{At: now, From: from, To: to, By: c.User.ID, Kind: kind})
		x.AddresseeID, x.Status, x.Hint = to, "Sent", ""
		if kind == "not_mine" {
			x.Reason = "Предыдущий адресат подсказал, что лучше ответит этот коллега"
		}
	})
	if to == c.Store.UserBuddy(q.AskedBy) { // BR-10a: если дальше бадди — подсказка про топ-руководителя
		hint := seniorHint(Ctx{Store: c.Store, User: askerOf(c, q.AskedBy)}, q.Title+" "+q.Text)
		out, _ = c.Store.UpdateQuestion(q.ID, func(x *store.Question) { x.Hint = hint })
	}
	c.Store.Emit(store.EvQuestionRerouted, c.User.ID, q.ID, map[string]string{"from": from, "to": to, "asker": q.AskedBy, "kind": kind,
		"urgent": map[bool]string{true: "true", false: "false"}[q.Urgent]})
	return out, nil
}

func askerOf(c Ctx, id string) *mockdata.User {
	u, _ := c.Store.User(id)
	return u
}

// NotMine — адресат: «не ко мне» (BR-17). Указал коллегу — вопрос ему; не указал — ИИ предлагает следующего
// кандидата без тех, кто уже отказался; кандидатов нет — бадди стажёра.
func NotMine(c Ctx, questionID, to string) (store.Question, *Error) {
	return atomic(c, func(c Ctx) (store.Question, *Error) {
		q, ok := findQuestion(c, questionID)
		switch {
		case !ok:
			return q, &Error{Code: "not_found", Message: "Вопрос не найден."}
		case q.AddresseeID != c.User.ID:
			return q, &Error{Code: "forbidden", Message: "«Не ко мне» может нажать только адресат."}
		case q.Status != "Sent":
			return q, &Error{Code: "validation", Message: "По вопросу уже есть ответ или встреча."}
		case q.Hint != "" && c.User.ID == c.Store.UserBuddy(q.AskedBy):
			// BR-10a: вопрос по теме топ-руководителя бадди решает сам — ответить или переадресовать
			return q, &Error{Code: "validation", Message: "Этот вопрос пришёл тебе как бадди — ответь или переадресуй его."}
		}
		if to == "" {
			asker := Ctx{Store: c.Store, User: askerOf(c, q.AskedBy)}
			exclude := map[string]bool{c.User.ID: true}
			for _, r := range q.Route {
				exclude[r.From] = true
			}
			id, conf, _ := findExpertExcluding(asker, q.Title+" "+q.Text, exclude)
			if id == "" || conf < ExpertThreshold || isSenior(asker, id) {
				id = c.Store.UserBuddy(q.AskedBy)
			}
			to = id
		}
		return reroute(c, q, to, "not_mine")
	})
}

// Redirect — бадди переадресует вопрос своего стажёра (BR-16); владелец базы назначает открытый вопрос (BR-37, SC-22).
func Redirect(c Ctx, questionID, to string) (store.Question, *Error) {
	return atomic(c, func(c Ctx) (store.Question, *Error) {
		q, ok := findQuestion(c, questionID)
		if !ok {
			return q, &Error{Code: "not_found", Message: "Вопрос не найден."}
		}
		isBuddy := c.User.ID == c.Store.UserBuddy(q.AskedBy)
		isOwner := c.User.ID == c.Store.KBOwner()
		switch {
		case q.Status == "Open" && isOwner:
			return reroute(c, q, to, "assign")
		case !isBuddy:
			return q, &Error{Code: "forbidden", Message: "Переадресовать может бадди стажёра."}
		case q.Status != "Sent" && q.Status != "NoRecording":
			return q, &Error{Code: "validation", Message: "По вопросу уже есть ответ или встреча."}
		}
		return reroute(c, q, to, "redirect")
	})
}
