package agent

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

// QuestionEdit — BR-14: владелец меняет только текст и срочность, а не адресата и инструмент.
type QuestionEdit struct {
	Version int     `json:"version"`
	Text    *string `json:"text,omitempty"`
	Urgent  *bool   `json:"urgent,omitempty"`
}

func draftView(d store.ActionDraft) Draft {
	status := "pending"
	if d.Cancelled {
		status = "cancelled"
	} else if len(d.Result) > 0 {
		status = "confirmed"
	}
	return Draft{ID: d.ID, UserID: d.UserID, Tool: d.Tool, Args: d.Args, Version: d.Version, Status: status, Result: d.Result}
}

func draftMissing() *tools.Error {
	return &tools.Error{Code: "not_found", Message: "Черновик не найден."}
}
func draftConflict(message string) *tools.Error {
	return &tools.Error{Code: "conflict", Message: message}
}

func (a *Agent) GetDraft(login, id string) (Draft, *tools.Error) {
	u, found := a.Store.UserByLogin(login)
	d, ok := a.Store.ActionDraft(id)
	if !found || !ok || d.UserID != u.ID {
		return Draft{}, draftMissing()
	}
	return draftView(d), nil
}

// mutateDraft сериализует правку, отмену и подтверждение, в том числе между экземплярами backend.
func (a *Agent) mutateDraft(login, id string, fn func(*store.Store, *mockdata.User, *store.ActionDraft) *tools.Error) (Draft, *tools.Error) {
	u, found := a.Store.UserByLogin(login)
	if !found {
		return Draft{}, draftMissing()
	}
	var out Draft
	var rejected *tools.Error
	err := a.Store.Transaction(func(s *store.Store) error {
		d, ok := s.ActionDraft(id)
		if !ok || d.UserID != u.ID {
			rejected = draftMissing()
		} else {
			rejected = fn(s, u, &d)
		}
		if rejected != nil {
			return errors.New("draft rejected")
		}
		s.PutDraft(d)
		out = draftView(d)
		return nil
	})
	if rejected != nil {
		return Draft{}, rejected
	}
	if err != nil {
		return Draft{}, &tools.Error{Code: "unavailable", Message: "Не удалось сохранить действие. Повтори запрос."}
	}
	return out, nil
}

func checkQuestionDraft(s *store.Store, u *mockdata.User, d store.ActionDraft) *tools.Error {
	text, _ := d.Args["text"].(string)
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 8000 {
		return &tools.Error{Code: "validation", Message: "Текст вопроса — от 1 до 8000 символов."}
	}
	if containsPII(text) {
		return &tools.Error{Code: "validation", Message: piiAnswer}
	}
	return tools.Check(tools.Ctx{Store: s, User: u}, d.Tool, d.Args)
}

func (a *Agent) EditQuestionDraft(login, id string, edit QuestionEdit) (Draft, *tools.Error) {
	return a.mutateDraft(login, id, func(s *store.Store, u *mockdata.User, d *store.ActionDraft) *tools.Error {
		if d.Cancelled || len(d.Result) > 0 {
			return draftConflict("Черновик уже отменён или отправлен.")
		}
		if d.Tool != "send_question" {
			return &tools.Error{Code: "validation", Message: "Здесь можно редактировать только вопрос человеку."}
		}
		if edit.Version != d.Version {
			return draftConflict("Черновик изменился. Обнови его и проверь текст перед отправкой.")
		}
		if edit.Version < 1 || (edit.Text == nil && edit.Urgent == nil) {
			return &tools.Error{Code: "validation", Message: "Укажи версию и изменения текста или срочности."}
		}
		changed := false
		if edit.Text != nil {
			text := strings.TrimSpace(*edit.Text)
			if d.Args["text"] != text {
				d.Args["text"], d.Args["title"] = text, tools.QuestionTitle("", text)
				changed = true
			}
		}
		urgent, _ := d.Args["urgent"].(bool)
		if edit.Urgent != nil && urgent != *edit.Urgent {
			d.Args["urgent"], changed = *edit.Urgent, true
		}
		if e := checkQuestionDraft(s, u, *d); e != nil {
			return e
		}
		if changed {
			d.Version++
			s.Emit(store.EvDraftEdited, u.ID, d.ID, map[string]string{"tool": d.Tool, "version": strconv.Itoa(d.Version)})
		}
		return nil
	})
}

// CancelDraft отменяет любое ещё не выполненное действие. Повтор отмены безопасен.
func (a *Agent) CancelDraft(login, id string, version int) (Draft, *tools.Error) {
	return a.mutateDraft(login, id, func(s *store.Store, u *mockdata.User, d *store.ActionDraft) *tools.Error {
		if d.Cancelled {
			return nil
		}
		if len(d.Result) > 0 {
			return draftConflict("Действие уже выполнено — отменить черновик нельзя.")
		}
		if version != d.Version {
			return draftConflict("Черновик изменился. Обнови его перед отменой.")
		}
		d.Cancelled = true
		d.Version++
		s.Emit(store.EvDraftCancelled, u.ID, d.ID, map[string]string{"tool": d.Tool})
		return nil
	})
}

// Confirm повторяет квитанцию после успеха. Старый запрос не может отправить изменённый вопрос.
// Без версии поддерживаются только исходные черновики (CLI и прежний интерфейс).
func (a *Agent) Confirm(login, id string, expected ...int) tools.Result {
	version := 0
	if len(expected) > 0 {
		version = expected[0]
	}
	var result tools.Result
	_, e := a.mutateDraft(login, id, func(s *store.Store, u *mockdata.User, d *store.ActionDraft) *tools.Error {
		if d.Cancelled {
			return draftConflict("Черновик отменён — ничего не отправлено.")
		}
		if (version == 0 && d.Version != 1) || (version != 0 && version != d.Version) {
			return draftConflict("Черновик изменился. Обнови его и подтверди актуальный текст.")
		}
		if len(d.Result) > 0 {
			if json.Unmarshal(d.Result, &result) != nil {
				return &tools.Error{Code: "unavailable", Message: "Не удалось прочитать квитанцию."}
			}
			return nil
		}
		if d.Tool == "send_question" {
			if e := checkQuestionDraft(s, u, *d); e != nil {
				return e
			}
		}
		result = tools.Run(tools.Ctx{Store: s, User: u}, d.Tool, d.Args)
		if result.Error != nil {
			return result.Error
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return &tools.Error{Code: "unavailable", Message: "Не удалось сохранить квитанцию."}
		}
		d.Result = raw
		return nil
	})
	if e != nil {
		return tools.Result{Error: e}
	}
	return result
}
