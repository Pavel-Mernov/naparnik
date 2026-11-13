package store

import (
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// SettingsEdit — что человек поменял в настройках (SCR-I8). Остальное — из оргструктуры и мок-данных.
type SettingsEdit struct {
	Proactivity              string   `json:"proactivity,omitempty"`
	QuestionTextVisibleBuddy *bool    `json:"question_text_visible_to_buddy,omitempty"`
	Reminders                []string `json:"reminders"` // null — не менял; [] — выключил напоминания
	// QuietUntil — «не беспокоить» до этого момента (RFC3339): несрочные уведомления в Telegram ждут (BR-52).
	QuietUntil string `json:"quiet_until,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// Режимы проактивности (SCR-I8): утренний план, только дедлайны и встречи, выключено.
var Proactivities = []string{"daily_plan", "deadlines_and_meetings", "off"}

// Reminders — за сколько напоминать о событии: за день, за час (BR-84).
var Reminders = []string{"1d", "1h"}

// SettingsFor — действующие настройки: по умолчанию текст вопросов виден бадди (BR-86) и напоминание за час,
// поверх — мок-данные, поверх — правки человека.
func (s *Store) SettingsFor(u *mockdata.User) mockdata.Settings {
	out := mockdata.Settings{QuestionTextVisibleBuddy: true, Reminders: []string{"1h"}}
	if u.Settings != nil {
		out = *u.Settings
		out.Reminders = append([]string(nil), u.Settings.Reminders...)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.settings[u.ID]; ok {
		if e.Proactivity != "" {
			out.Proactivity = e.Proactivity
		}
		if e.QuestionTextVisibleBuddy != nil {
			out.QuestionTextVisibleBuddy = *e.QuestionTextVisibleBuddy
		}
		if e.Reminders != nil {
			out.Reminders = append([]string{}, e.Reminders...)
		}
	}
	return out
}

// QuietUntil — до какого момента включено «не беспокоить»; нулевое время — выключено или истекло.
func (s *Store) QuietUntil(userID string, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := time.Parse(time.RFC3339, s.settings[userID].QuietUntil)
	if err != nil || !t.After(now) {
		return time.Time{}
	}
	return t
}

func (s *Store) UpdateSettings(userID string, fn func(*SettingsEdit)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.settings[userID]
	if e.Reminders != nil {
		e.Reminders = append([]string{}, e.Reminders...)
	}
	fn(&e)
	e.UpdatedAt = time.Now().Format(time.RFC3339)
	s.settings[userID] = e
}
