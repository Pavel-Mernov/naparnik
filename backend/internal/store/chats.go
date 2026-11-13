package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// ChatTurn — вопрос Напарнику и его ответ. Из них собираются история чата после перезагрузки
// и раздел «Спрашивал у Напарника» в «Моих вопросах».
type ChatTurn struct {
	ID           string          `json:"id"`
	Conversation int             `json:"conversation"`
	Question     string          `json:"question"`
	Reply        json.RawMessage `json:"reply"`
	At           string          `json:"at"`
}

// ChatLog — история одного пользователя. Conversation — номер текущего разговора: «Новый разговор»
// очищает окно чата, но вопросы остаются в «Моих вопросах».
type ChatLog struct {
	Conversation int        `json:"conversation"`
	Next         int        `json:"next"`
	Turns        []ChatTurn `json:"turns"`
}

// maxChatTurns — сколько последних вопросов хранить на человека.
const maxChatTurns = 200

func (s *Store) AppendChat(userID, question string, reply json.RawMessage) ChatTurn {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.chats[userID]
	l.Next++
	t := ChatTurn{ID: fmt.Sprintf("CHAT-%s-%d", userID, l.Next), Conversation: l.Conversation, Question: question,
		Reply: append(json.RawMessage(nil), reply...), At: time.Now().Format(time.RFC3339)}
	l.Turns = append(append([]ChatTurn(nil), l.Turns...), t)
	if len(l.Turns) > maxChatTurns {
		l.Turns = l.Turns[len(l.Turns)-maxChatTurns:]
	}
	s.chats[userID] = l
	return t
}

// NewConversation — «Новый разговор»: следующий вопрос начнёт пустое окно чата.
func (s *Store) NewConversation(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.chats[userID]
	l.Conversation++
	s.chats[userID] = l
}

func (s *Store) Chat(userID string) ChatLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.chats[userID]
	l.Turns = append([]ChatTurn{}, l.Turns...)
	return l
}
