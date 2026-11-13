package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// settingsView — настройки человека (SCR-I8). Рабочие часы — из оргструктуры, здесь только показываются.
type settingsView struct {
	Proactivity              string   `json:"proactivity"`
	QuestionTextVisibleBuddy bool     `json:"question_text_visible_to_buddy"`
	Reminders                []string `json:"reminders"`
	QuietUntil               string   `json:"quiet_until,omitempty"`
	WorkingHours             string   `json:"working_hours"`
}

func (s *Server) settingsOf(u *mockdata.User) settingsView {
	set := s.Store.SettingsFor(u)
	v := settingsView{Proactivity: set.Proactivity, QuestionTextVisibleBuddy: set.QuestionTextVisibleBuddy,
		Reminders: set.Reminders, WorkingHours: u.WorkingHours}
	if v.Reminders == nil {
		v.Reminders = []string{}
	}
	if q := s.Store.QuietUntil(u.ID, time.Now()); !q.IsZero() {
		v.QuietUntil = q.Format(time.RFC3339)
	}
	return v
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	if err := s.Store.Refresh(r.Context()); err != nil {
		writeJSON(w, 503, errBody("unavailable", "Хранилище недоступно."))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "settings": s.settingsOf(u)})
}

// editSettings — PATCH /api/settings: меняются только переданные поля.
// quiet_hours: 0 — выключить «не беспокоить», 1…24 — включить на столько часов.
func (s *Server) editSettings(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Proactivity              *string   `json:"proactivity"`
		QuestionTextVisibleBuddy *bool     `json:"question_text_visible_to_buddy"`
		Reminders                *[]string `json:"reminders"`
		QuietHours               *int      `json:"quiet_hours"`
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&body); err != nil {
		writeJSON(w, 422, errBody("validation", "Некорректный запрос."))
		return
	}
	intern := store.HasRole(u, store.RoleIntern)
	switch {
	case body.Proactivity != nil && !slices.Contains(store.Proactivities, *body.Proactivity):
		writeJSON(w, 422, errBody("validation", "Режим: daily_plan, deadlines_and_meetings или off."))
		return
	case (body.Proactivity != nil || body.QuestionTextVisibleBuddy != nil) && !intern:
		writeJSON(w, 403, errBody("forbidden", "Режим подсказок и видимость вопросов — настройки стажёра."))
		return
	case body.QuietHours != nil && (*body.QuietHours < 0 || *body.QuietHours > 24):
		writeJSON(w, 422, errBody("validation", "«Не беспокоить» — от 0 до 24 часов."))
		return
	}
	if body.Reminders != nil {
		for _, x := range *body.Reminders {
			if !slices.Contains(store.Reminders, x) {
				writeJSON(w, 422, errBody("validation", "Напоминания: 1d (за день) и 1h (за час)."))
				return
			}
		}
	}
	err := s.Store.Transaction(func(st *store.Store) error {
		st.UpdateSettings(u.ID, func(e *store.SettingsEdit) {
			if body.Proactivity != nil {
				e.Proactivity = *body.Proactivity
			}
			if body.QuestionTextVisibleBuddy != nil {
				e.QuestionTextVisibleBuddy = body.QuestionTextVisibleBuddy
			}
			if body.Reminders != nil {
				e.Reminders = append([]string{}, *body.Reminders...)
			}
			if body.QuietHours != nil {
				e.QuietUntil = ""
				if *body.QuietHours > 0 {
					e.QuietUntil = time.Now().Add(time.Duration(*body.QuietHours) * time.Hour).Format(time.RFC3339)
				}
			}
		})
		return nil
	})
	if err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не удалось сохранить настройки. Повтори."))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "settings": s.settingsOf(u)})
}
