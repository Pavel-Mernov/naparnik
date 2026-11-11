package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// notifications — центр уведомлений и состояние Telegram у текущей учётки.
func (s *Server) notifications(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	list := s.Store.NotificationsFor(u.ID)
	unread := 0
	for _, n := range list {
		if !n.Read {
			unread++
		}
	}
	_, linked := s.Store.TelegramChat(u.ID)
	tg := map[string]any{"connected": linked, "mode": "off"}
	if s.TG != nil {
		tg["mode"], tg["bot"] = s.TG.Mode(), s.TG.Bot()
	}
	writeJSON(w, 200, map[string]any{"notifications": list, "unread": unread, "telegram": tg})
}

func (s *Server) notificationsRead(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	if err := s.Store.Transaction(func(st *store.Store) error { st.MarkRead(u.ID); return nil }); err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не получилось сохранить."))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// telegramLink — BR-50: одноразовый код на 10 минут; стажёр отправляет его боту.
func (s *Server) telegramLink(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	if s.TG == nil {
		writeJSON(w, 503, errBody("unavailable", "Telegram не настроен на сервере."))
		return
	}
	code, link, err := s.TG.LinkCode(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не получилось выдать код. Попробуй ещё раз."))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "code": code, "link": link, "bot": s.TG.Bot(), "mode": s.TG.Mode(), "expires_in_min": 10})
}

func (s *Server) telegramUnlink(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	if err := s.Store.Transaction(func(st *store.Store) error { st.UnlinkTelegram(u.ID); return nil }); err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не получилось сохранить."))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// feedback — оценка ответа из базы: помогло / не помогло / устарело (M-04, M-09). «Устарело» уходит автору (H11).
func (s *Server) feedback(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		PageID string `json:"page_id"`
		Value  string `json:"value"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Value != "helped" && body.Value != "not_helped" && body.Value != "stale" {
		writeJSON(w, 422, errBody("validation", "Оценка: helped, not_helped или stale."))
		return
	}
	author := ""
	for _, p := range s.Store.Pages() {
		if p.ID == body.PageID {
			author = p.AuthorID
		}
	}
	err := s.Store.Transaction(func(st *store.Store) error {
		st.Emit(store.EvAnswerFeedback, u.ID, body.PageID, map[string]string{"value": body.Value, "author": author})
		return nil
	})
	if err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не получилось сохранить оценку."))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// metrics — метрики пользы из журнала событий (09-METRICS): руководителю отдела.
func (s *Server) metrics(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	if !store.HasRole(u, store.RoleLead) {
		writeJSON(w, 403, errBody("forbidden", "Метрики отдела видит руководитель."))
		return
	}
	writeJSON(w, 200, Metrics(s.Store.Events()))
}

// Metrics считает показатели по журналу: сколько вопросов закрыла база, как быстро отвечают люди,
// растёт ли база, полезны ли ответы. Числа — по событиям прототипа, без данных до запуска.
func Metrics(events []store.LogEvent) map[string]any {
	count := map[string]int{}
	feedback := map[string]int{}
	sent := map[string]time.Time{}
	var waits []float64
	for _, e := range events {
		count[e.Type]++
		at, _ := time.Parse(time.RFC3339, e.At)
		switch e.Type {
		case store.EvQuestionSent:
			sent[e.Object] = at
		case store.EvQuestionAnswered:
			if t0, ok := sent[e.Object]; ok {
				waits = append(waits, at.Sub(t0).Minutes())
			}
		case store.EvAnswerFeedback:
			feedback[e.Meta["value"]]++
		}
	}
	median := 0.0
	if len(waits) > 0 {
		sort.Float64s(waits)
		median = waits[len(waits)/2]
	}
	kb, toPeople := count[store.EvKBAnswer], count[store.EvQuestionSent]
	share := 0.0
	if kb+toPeople > 0 {
		share = float64(kb) / float64(kb+toPeople)
	}
	rated := feedback["helped"] + feedback["not_helped"] + feedback["stale"]
	csat := 0.0
	if rated > 0 {
		csat = float64(feedback["helped"]) / float64(rated)
	}
	return map[string]any{
		"events":                len(events),
		"kb_answers":            kb,                              // M-09: ответы из базы
		"questions_to_people":   toPeople,                        // вопросы людям
		"kb_share":              share,                           // M-09: доля закрытых базой
		"answered_by_people":    count[store.EvQuestionAnswered], // ответы людей
		"median_answer_minutes": median,                          // M-01/M-08: медиана ожидания ответа
		"pages_published":       count[store.EvPagePublished],    // M-10: рост базы
		"csat":                  csat,                            // M-04: доля «помогло»
		"feedback":              feedback,                        // помогло / не помогло / устарело
		"requests_created":      count[store.EvRequestCreated],   // действия через сервисы
		"requests_decided":      count[store.EvRequestDecided],
		"steps_done":            count[store.EvStepDone], // M-02: прогресс по плану
	}
}
