package agent

import (
	"encoding/json"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// HistoryTurn — вопрос и ответ Напарника для окна чата и «Моих вопросов».
// Черновики в ответе — в текущем состоянии: подтверждённый показывается квитанцией, отменённый — отменённым.
type HistoryTurn struct {
	ID           string `json:"id"`
	Conversation int    `json:"conversation"`
	Question     string `json:"question"`
	At           string `json:"at"`
	Reply        Reply  `json:"reply"`
}

type History struct {
	Conversation int           `json:"conversation"`
	Turns        []HistoryTurn `json:"turns"`
}

// Remember сохраняет ход разговора, который человек видел в чате. Вызывается из веб-чата,
// а не из Ask: прогон evals и консольный бадди не засоряют историю учёток.
func (a *Agent) Remember(login, question string, r Reply) error {
	u, found := a.Store.UserByLogin(login)
	if !found {
		return nil
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	// ПДн и секреты в историю не попадают (BR-61, BR-72): Напарник обещает «никуда не сохраняю».
	return a.Store.Transaction(func(s *store.Store) error {
		s.AppendChat(u.ID, MaskPII(question), raw)
		return nil
	})
}

// History — сохранённые вопросы человека, от старых к новым.
func (a *Agent) History(login string) History {
	u, found := a.Store.UserByLogin(login)
	if !found {
		return History{Turns: []HistoryTurn{}}
	}
	log := a.Store.Chat(u.ID)
	out := History{Conversation: log.Conversation, Turns: []HistoryTurn{}}
	for _, t := range log.Turns {
		var r Reply
		if json.Unmarshal(t.Reply, &r) != nil {
			continue
		}
		for i, d := range r.Drafts {
			if cur, ok := a.Store.ActionDraft(d.ID); ok && cur.UserID == u.ID {
				r.Drafts[i] = draftView(cur)
			}
		}
		if r.Drafts == nil {
			r.Drafts = []Draft{}
		}
		if r.Sources == nil {
			r.Sources = []Source{}
		}
		out.Turns = append(out.Turns, HistoryTurn{t.ID, t.Conversation, t.Question, t.At, r})
	}
	return out
}

// NewConversation — «Новый разговор»: модель забывает контекст, окно чата пустеет, вопросы остаются в «Моих вопросах».
func (a *Agent) NewConversation(login string) error {
	a.Reset(login)
	u, found := a.Store.UserByLogin(login)
	if !found {
		return nil
	}
	return a.Store.Transaction(func(s *store.Store) error {
		s.NewConversation(u.ID)
		return nil
	})
}
