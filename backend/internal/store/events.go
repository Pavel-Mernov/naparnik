package store

import (
	"fmt"
	"time"
)

// Журнал событий (BR-73) и уведомления (BR-50…53, docs/04-mock-data/notifications.json).
// Событие пишется в той же транзакции, что и действие, — нет «действие есть, события нет».
// Уведомления выводятся из событий по правилам ниже; канал Telegram — только факт и ссылка (BR-51).

type LogEvent struct {
	ID     string            `json:"id"`
	At     string            `json:"at"`
	Type   string            `json:"type"`
	Actor  string            `json:"actor,omitempty"`
	Object string            `json:"object,omitempty"`
	Meta   map[string]string `json:"meta,omitempty"`
}

// Notification — запись в центре уведомлений. TG: "" — не для Telegram, "pending" — ждёт отправки,
// "sent" / "skipped" (не подключён) / "failed" — итог. DeliverAt — не раньше этого времени (сводки, рабочие часы).
type Notification struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	View      string `json:"view"` // раздел веба, куда ведёт «Открыть»
	Urgent    bool   `json:"urgent"`
	At        string `json:"at"`
	Read      bool   `json:"read"`
	TG        string `json:"tg,omitempty"`
	DeliverAt string `json:"deliver_at,omitempty"`
}

// Event types — словарь журнала: по ним считаются метрики 09-METRICS.
const (
	EvQuestionSent       = "question_sent"
	EvQuestionAnswered   = "question_answered"
	EvPagePublished      = "page_published"
	EvRequestCreated     = "request_created"
	EvRequestDecided     = "request_decided"
	EvStepDone           = "step_done"
	EvKBAnswer           = "kb_answer"
	EvAnswerFeedback     = "answer_feedback"
	EvDraftConfirmed     = "draft_confirmed"
	EvMeetingScheduled   = "meeting_scheduled"
	EvMeetingNoRecording = "meeting_no_recording"
	EvQuestionOpen       = "question_open"
	EvQuestionRerouted   = "question_rerouted"
	EvPagesMerged        = "pages_merged"
	EvQuestionOverdue    = "question_overdue" // SLA ответа истёк (BR-18)
	EvInternIdle         = "intern_idle"      // стажёр простаивает (BR-85)
	EvPlanEdited         = "plan_edited"      // бадди поправил черновик плана (BR-81)
	EvPlanDecided        = "plan_decided"     // план утверждён или активирован без утверждения
)

// Now — часы для журнала; в тестах подменяются.
var Now = time.Now

// Emit записывает событие и уведомления, которые из него следуют.
func (s *Store) Emit(typ, actor, object string, meta map[string]string) LogEvent {
	e := LogEvent{ID: s.NextID("EV"), At: Now().Format(time.RFC3339), Type: typ, Actor: actor, Object: object, Meta: meta}
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
	for _, n := range s.notificationsFor(e) {
		s.Notify(n)
	}
	return e
}

// Notify добавляет уведомление; повтор с тем же ID не дублирует (напоминания, сводки).
func (s *Store) Notify(n Notification) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n.ID == "" {
		s.seq["N"]++
		n.ID = fmt.Sprintf("N-%d", s.seq["N"])
	}
	for _, x := range s.notifications {
		if x.ID == n.ID {
			return false
		}
	}
	if n.At == "" {
		n.At = Now().Format(time.RFC3339)
	}
	s.notifications = append(s.notifications, n)
	return true
}

func (s *Store) Events() []LogEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LogEvent(nil), s.events...)
}

// NotificationsFor — центр уведомлений пользователя, новые сверху.
func (s *Store) NotificationsFor(userID string) []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Notification{}
	for i := len(s.notifications) - 1; i >= 0; i-- {
		if s.notifications[i].UserID == userID {
			out = append(out, s.notifications[i])
		}
	}
	return out
}

func (s *Store) MarkRead(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.notifications {
		if s.notifications[i].UserID == userID {
			s.notifications[i].Read = true
		}
	}
}

// PendingTelegram — уведомления, которые пора отправить в Telegram.
func (s *Store) PendingTelegram(now time.Time) []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Notification
	for _, n := range s.notifications {
		if n.TG != "pending" {
			continue
		}
		if n.DeliverAt != "" {
			if t, err := time.Parse(time.RFC3339, n.DeliverAt); err == nil && now.Before(t) {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func (s *Store) SetTelegramStatus(ids []string, status, deliverAt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for i := range s.notifications {
		if set[s.notifications[i].ID] {
			s.notifications[i].TG = status
			if deliverAt != "" {
				s.notifications[i].DeliverAt = deliverAt
			}
		}
	}
}

// TelegramChat — привязанный чат пользователя (BR-50: подключение по желанию).
func (s *Store) TelegramChat(userID string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.tgLinks[userID]
	return id, ok
}

// LinkTelegram привязывает чат и возвращает в очередь пропущенное за последние сутки (не больше 10):
// человек подключил бота — и сразу видит, что уже произошло.
func (s *Store) LinkTelegram(userID string, chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tgLinks[userID] = chatID
	since := Now().Add(-24 * time.Hour)
	requeued := 0
	for i := len(s.notifications) - 1; i >= 0 && requeued < 10; i-- {
		n := &s.notifications[i]
		if n.UserID != userID || n.TG != "skipped" {
			continue
		}
		if at, err := time.Parse(time.RFC3339, n.At); err == nil && at.Before(since) {
			continue
		}
		n.TG = "pending"
		requeued++
	}
}

func (s *Store) UnlinkTelegram(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tgLinks, userID)
}

// UserByTelegram — кто привязал этот чат (для /stop).
func (s *Store) UserByTelegram(chatID int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for u, c := range s.tgLinks {
		if c == chatID {
			return u, true
		}
	}
	return "", false
}
