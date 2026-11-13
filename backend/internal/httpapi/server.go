// Package httpapi — HTTP API для фронта (пути из docs/03-requirements/07-FRONTEND.md, раздел 9).
//
// Авторизация в прототипе — заголовок X-User с логином тестовой учётки (A5: в Авито авторизация уже есть,
// переключатель учёток в проде выключается feature flag).
package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/flags"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/telegram"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

type Server struct {
	Agent       *agent.Agent
	Store       *store.Store
	StaticDir   string             // каталог собранного фронта (STATIC_DIR); пусто — на «/» страница со ссылкой на интерфейс
	FrontendURL string             // где интерфейс (FRONTEND_URL), по умолчанию http://localhost:3000
	TG          *telegram.Service  // уведомления в Telegram; nil — только веб
	KV          cache.KV           // Redis или память: коды привязки, лимит запросов к модели
	ChatLimit   int                // запросов к модели в минуту на пользователя; 0 — без лимита
	Events      *adapters.Exporter // выгрузка журнала событий; nil — не выгружается
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Refresh(r.Context()); err != nil {
			writeJSON(w, 503, errBody("unavailable", "Хранилище недоступно"))
			return
		}
		hits, misses := tools.CacheStats()
		kind := "off"
		if s.KV != nil {
			kind = s.KV.Kind()
		}
		out := map[string]any{"status": "ok", "cache": map[string]any{"kind": kind, "kb_hits": hits, "kb_misses": misses},
			"flags": flags.Default.Snapshot()}
		if s.Events != nil {
			out["events"] = s.Events.Stats()
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/profile", s.auth(s.myProfile)) // профили сотрудников (карточка стажёра)
	mux.HandleFunc("PATCH /api/profile", s.auth(s.editProfile))
	mux.HandleFunc("PUT /api/profile/photo", s.auth(s.putPhoto))
	mux.HandleFunc("DELETE /api/profile/photo", s.auth(s.deletePhoto))
	mux.HandleFunc("GET /api/profiles", s.auth(s.profiles))
	mux.HandleFunc("GET /api/profiles/{id}", s.auth(s.profile))
	mux.HandleFunc("GET /api/profiles/{id}/photo", s.auth(s.photo))
	mux.HandleFunc("GET /api/practices", s.auth(s.practices))
	mux.HandleFunc("POST /api/practices/sync", s.auth(s.practiceSync))
	mux.HandleFunc("POST /api/practices/{intern}/{practice}/generate", s.auth(s.practiceGenerate))
	mux.HandleFunc("POST /api/practices/{intern}/{practice}/{action}", s.auth(s.practiceAction))
	mux.HandleFunc("GET /api/dashboard/team", s.auth(s.teamDashboard))
	mux.HandleFunc("GET /api/team", s.auth(s.teamMap))      // SC-02: карта команды
	mux.HandleFunc("GET /api/settings", s.auth(s.settings)) // SCR-I8: настройки
	mux.HandleFunc("PATCH /api/settings", s.auth(s.editSettings))
	mux.HandleFunc("POST /api/chat/messages", s.auth(s.chat))
	mux.HandleFunc("POST /api/chat/reset", s.auth(s.chatReset))
	mux.HandleFunc("GET /api/chat/history", s.auth(s.chatHistory))
	mux.HandleFunc("GET /api/drafts", s.auth(s.drafts))
	mux.HandleFunc("GET /api/drafts/{id}", s.auth(s.draft))
	mux.HandleFunc("PATCH /api/drafts/{id}", s.auth(s.draftEdit))
	mux.HandleFunc("DELETE /api/drafts/{id}", s.auth(s.draftCancel))
	mux.HandleFunc("POST /api/drafts/{id}/confirm", s.auth(s.confirm))
	mux.HandleFunc("POST /api/requests/{id}/confirm", s.auth(s.confirm))       // заявки (отгул, доступ)
	mux.HandleFunc("POST /api/questions/{id}/confirm-send", s.auth(s.confirm)) // BR-11: вопрос человеку
	mux.HandleFunc("GET /api/plans/{internId}", s.auth(s.plan))
	mux.HandleFunc("GET /api/plans/{internId}/draft", s.auth(s.planDraft)) // SC-20, BR-81
	mux.HandleFunc("PATCH /api/plans/{internId}/draft", s.auth(s.planDraftEdit))
	mux.HandleFunc("POST /api/plans/{internId}/approve", s.auth(s.planApprove))
	mux.HandleFunc("PATCH /api/plan-steps/{id}", s.auth(s.markStep))
	mux.HandleFunc("GET /api/requests", s.auth(s.requests))
	mux.HandleFunc("POST /api/requests", s.auth(s.createRequest))            // BR-42: «сделай сам» — форма сервиса
	mux.HandleFunc("POST /api/requests/{id}/decision", s.auth(s.decide))     // согласование руководителем
	mux.HandleFunc("GET /api/services", s.auth(s.services))                  // справочник сервиса заявок
	mux.HandleFunc("GET /api/services/absence/precheck", s.auth(s.precheck)) // предупреждения формы отгула
	mux.HandleFunc("GET /api/services/access/rules", s.auth(s.accessRules))  // SC-33: объяснение доступа, включая будущие недели
	mux.HandleFunc("GET /api/questions", s.auth(s.questions))
	mux.HandleFunc("GET /api/today", s.auth(s.today))
	mux.HandleFunc("GET /api/users", s.users)
	mux.HandleFunc("GET /api/notifications", s.auth(s.notifications)) // центр уведомлений (BR-50…53)
	mux.HandleFunc("POST /api/notifications/read", s.auth(s.notificationsRead))
	mux.HandleFunc("POST /api/telegram/link", s.auth(s.telegramLink)) // BR-50: одноразовый код
	mux.HandleFunc("DELETE /api/telegram/link", s.auth(s.telegramUnlink))
	mux.HandleFunc("POST /api/feedback", s.auth(s.feedback)) // «помогло / не помогло / устарело» (M-04, H11)
	mux.HandleFunc("GET /api/metrics", s.auth(s.metrics))    // метрики из журнала событий (BR-73)
	mux.HandleFunc("GET /api/kb/pages", s.auth(s.kbPages))
	mux.HandleFunc("GET /api/kb/drafts", s.auth(s.kbDrafts))
	mux.HandleFunc("POST /api/kb/pages/{id}/approve", s.auth(s.kbApprove))
	mux.HandleFunc("POST /api/questions/{id}/answer", s.auth(s.answer))
	mux.HandleFunc("GET /api/questions/{id}/meeting-slots", s.auth(s.meetingSlots)) // US-14: 3 слота в рабочие часы обоих
	mux.HandleFunc("POST /api/questions/{id}/meeting", s.auth(s.scheduleMeeting))
	mux.HandleFunc("POST /api/questions/{id}/leave-open", s.auth(s.leaveOpen)) // BR-37
	mux.HandleFunc("GET /api/meetings", s.auth(s.meetings))
	mux.HandleFunc("POST /api/questions/{id}/not-mine", s.auth(s.notMine))  // BR-17
	mux.HandleFunc("POST /api/questions/{id}/redirect", s.auth(s.redirect)) // BR-16: бадди
	mux.HandleFunc("POST /api/questions/{id}/assign", s.auth(s.assign))     // BR-37: владелец базы
	mux.HandleFunc("GET /api/kb/maintenance", s.auth(s.kbMaintenance))      // SC-22
	mux.HandleFunc("POST /api/kb/duplicates/{id}", s.auth(s.kbDuplicate))
	mux.HandleFunc("POST /api/kb/drafts/{id}/remind", s.auth(s.kbRemind))
	mux.HandleFunc("POST /api/meetings/{id}/finish", s.auth(s.finishMeeting)) // в проде — сигнал ВКС
	if s.StaticDir != "" {
		files := http.FileServer(http.Dir(s.StaticDir))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// no-cache: браузер каждый раз сверяется с сервером — после обновления кода не остаётся старая версия
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
		})
	} else {
		// Backend — это API. Интерфейс отдаёт отдельный frontend; кто открыл адрес backend в браузере, видит ссылку на него.
		front := s.FrontendURL
		if front == "" {
			front = "http://localhost:3000"
		}
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Напарник · API</title>`+
				`<p>Это API Напарника. Интерфейс: <a href="%[1]s">%[1]s</a>. Проверка: <a href="/api/health">/api/health</a>.</p>`, html.EscapeString(front))
		})
	}
	return mux
}

type handler func(http.ResponseWriter, *http.Request, *mockdata.User)

func (s *Server) teamDashboard(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	writeResult(w, tools.TeamDashboard(tools.Ctx{Store: s.Store, User: u}))
}

func (s *Server) auth(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.Store.UserByLogin(r.Header.Get("X-User"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized", "Укажи тестовую учётку в заголовке X-User"))
			return
		}
		if err := s.Store.Refresh(r.Context()); err != nil {
			writeJSON(w, 503, errBody("unavailable", "Хранилище недоступно"))
			return
		}
		h(w, r, u)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(code, msg string) map[string]any {
	return map[string]any{"ok": false, "error": map[string]string{"code": code, "message": msg}}
}

var statusByCode = map[string]int{"forbidden": 403, "not_found": 404, "validation": 422, "unavailable": 503, "conflict": 409}

func writeResult(w http.ResponseWriter, res tools.Result) {
	status := http.StatusOK
	if res.Error != nil {
		status = statusByCode[res.Error.Code]
		if status == 0 {
			status = http.StatusBadRequest
		}
	}
	writeJSON(w, status, res)
}

// me — текущая учётка и стажёры, чей план ей виден (бадди и руководителю).
func (s *Server) me(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	interns := []map[string]string{}
	for _, other := range s.Store.Data.Team.Users {
		if other.ID != u.ID && store.HasRole(&other, store.RoleIntern) && s.Store.CanSeeIntern(u, other.ID) {
			interns = append(interns, map[string]string{"id": other.ID, "name": other.Name})
		}
	}
	writeJSON(w, 200, map[string]any{"id": u.ID, "login": u.Login, "name": u.Name, "position": u.Position, "roles": u.Roles,
		"week": s.Store.Week(u), "today": s.Store.Today.Format("2006-01-02"), "department": s.Store.Data.Team.Department.Name,
		"interns": interns})
}

// users — тестовые учётки для переключателя в демо (A5: в проде выключается).
func (s *Server) users(w http.ResponseWriter, _ *http.Request) {
	out := []map[string]any{}
	for _, u := range s.Store.Data.Team.Users {
		out = append(out, map[string]any{"id": u.ID, "login": u.Login, "name": u.Name, "position": u.Position, "roles": u.Roles})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

func (s *Server) kbPages(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	folders, pages := tools.KBPages(tools.Ctx{Store: s.Store, User: u})
	writeJSON(w, 200, map[string]any{"folders": nonNil(folders), "pages": nonNil(pages), "department": s.Store.Data.Team.Department.Name})
}

// answer — P2, US-10: адресат отвечает текстом; стажёр получает ответ сразу, автору — черновик страницы.
func (s *Server) answer(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Text string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	q, page, e := tools.AnswerQuestion(tools.Ctx{Store: s.Store, User: u}, r.PathValue("id"), body.Text)
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "question": q, "draft": page,
		"message": "Ответ отправлен. Проверь, я занёс твой ответ в базу — опубликую после твоего «Ок»."})
}

func (s *Server) kbDrafts(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	writeJSON(w, 200, map[string]any{"drafts": nonNil(tools.DraftsFor(tools.Ctx{Store: s.Store, User: u}))})
}

// kbApprove — BR-30: публикация только после «Ок» автора; правки — по желанию.
func (s *Server) kbApprove(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	page, e := tools.ApprovePage(tools.Ctx{Store: s.Store, User: u}, r.PathValue("id"), body.Title, body.Body)
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "page": page})
}

func (s *Server) today(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	writeResult(w, tools.Run(tools.Ctx{Store: s.Store, User: u}, "get_today", nil))
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		writeJSON(w, 422, errBody("validation", "Пустое сообщение"))
		return
	}
	// Лимит запросов к модели: защита от циклов и случайного расхода токенов (общий счётчик в Redis).
	if s.KV != nil && s.ChatLimit > 0 {
		if n, err := s.KV.Hit(r.Context(), "rl:chat:"+u.ID, time.Minute); err == nil && n > int64(s.ChatLimit) {
			writeJSON(w, http.StatusTooManyRequests, errBody("rate_limited", "Слишком много сообщений подряд — подожди минуту и повтори."))
			return
		}
	}
	reply, err := s.Agent.Ask(r.Context(), u.Login, body.Text)
	if err != nil {
		writeJSON(w, 500, errBody("internal", err.Error()))
		return
	}
	if len(reply.Sources) > 0 { // M-09: ответ из базы — событие для метрик
		ids := make([]string, 0, len(reply.Sources))
		for _, src := range reply.Sources {
			ids = append(ids, src.PageID)
		}
		_ = s.Store.Transaction(func(st *store.Store) error {
			st.Emit(store.EvKBAnswer, u.ID, ids[0], map[string]string{"pages": strings.Join(ids, ",")})
			return nil
		})
	}
	// История: вопрос не пропадает после закрытия чата, перезагрузки и перезапуска сервера.
	if err := s.Agent.Remember(u.Login, body.Text, reply); err != nil {
		log.Printf("chat history: %v", err)
	}
	writeJSON(w, 200, reply)
}

// chatReset — «Новый разговор»: окно чата пустеет, сохранённые вопросы остаются.
func (s *Server) chatReset(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	if err := s.Agent.NewConversation(u.Login); err != nil {
		writeJSON(w, 503, errBody("unavailable", "Не удалось начать новый разговор. Повтори."))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// teamMap — карта команды (SCR-I5): коллеги, зоны ответственности, их страницы и черновик сообщения.
func (s *Server) teamMap(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	writeJSON(w, 200, map[string]any{"people": tools.TeamMap(s.ctx(u)), "department": s.Store.Data.Team.Department.Name})
}

// chatHistory — сохранённые вопросы Напарнику: окно чата после перезагрузки и «Мои вопросы».
func (s *Server) chatHistory(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	if err := s.Store.Refresh(r.Context()); err != nil {
		writeJSON(w, 503, errBody("unavailable", "Хранилище недоступно."))
		return
	}
	writeJSON(w, 200, s.Agent.History(u.Login))
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Version *int `json:"version"`
	}
	if !decodeDraftBody(r, &body, true) || (body.Version != nil && *body.Version < 1) {
		writeJSON(w, 422, errBody("validation", "Неверная версия черновика."))
		return
	}
	if body.Version == nil {
		writeResult(w, s.Agent.Confirm(u.Login, r.PathValue("id")))
	} else {
		writeResult(w, s.Agent.Confirm(u.Login, r.PathValue("id"), *body.Version))
	}
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	writeResult(w, tools.Run(tools.Ctx{Store: s.Store, User: u}, "get_plan", map[string]any{"intern_id": r.PathValue("internId")}))
}

// markStep — клик человека в интерфейсе и есть подтверждение, черновик не нужен.
func (s *Server) markStep(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		InternID string `json:"intern_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	t, _ := tools.ByName("mark_step_done")
	c := tools.Ctx{Store: s.Store, User: u}
	if !tools.Allowed(t, u) {
		writeResult(w, tools.Result{Error: &tools.Error{Code: "forbidden", Message: "Отметить шаг может стажёр или его бадди."}})
		return
	}
	writeResult(w, tools.Run(c, "mark_step_done", map[string]any{"step_id": r.PathValue("id"), "intern_id": body.InternID}))
}

// requests — свои заявки; руководитель видит заявки своих стажёров (лента, SC-29).
func (s *Server) requests(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	var out []store.Request
	for _, req := range s.Store.Requests("") {
		if req.EmployeeID == u.ID || req.ApproverID == u.ID {
			out = append(out, req)
		}
	}
	writeJSON(w, 200, map[string]any{"requests": nonNil(out)})
}

// createRequest — стажёр сам заполнил форму сервиса: kind = absence (type, date, comment) или access (system, reason).
func (s *Server) createRequest(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody("validation", "Пустая заявка"))
		return
	}
	kind, _ := body["kind"].(string)
	writeResult(w, tools.CreateSelfOnce(tools.Ctx{Store: s.Store, User: u}, r.Header.Get("Idempotency-Key"), kind, body))
}

// decide — согласующий одобряет или отклоняет заявку с кодом причины.
func (s *Server) decide(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		Comment  string `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	req, e := tools.Decide(tools.Ctx{Store: s.Store, User: u}, r.PathValue("id"), body.Decision, body.Reason, strings.TrimSpace(body.Comment))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "request": req})
}

func (s *Server) services(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	writeJSON(w, 200, tools.Catalog(tools.Ctx{Store: s.Store, User: u}))
}

func (s *Server) accessRules(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	writeResult(w, tools.Run(tools.Ctx{Store: s.Store, User: u}, "get_access_rules", map[string]any{"system": r.URL.Query().Get("system")}))
}

func (s *Server) precheck(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	q := r.URL.Query()
	writeResult(w, tools.Run(tools.Ctx{Store: s.Store, User: u}, "precheck_absence", map[string]any{"type": q.Get("type"), "date": q.Get("date")}))
}

type questionView struct {
	ID          string            `json:"id"`
	AskedBy     string            `json:"asked_by"`
	AddresseeID string            `json:"addressee_id"`
	Addressee   string            `json:"addressee"`
	Title       string            `json:"title,omitempty"`
	Text        string            `json:"text,omitempty"`
	TextHidden  bool              `json:"text_hidden,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Urgent      bool              `json:"urgent"`
	Status      string            `json:"status"`
	Answer      string            `json:"answer,omitempty"`
	PageID      string            `json:"page_id,omitempty"`
	Hint        string            `json:"hint,omitempty"`    // BR-10a: только адресату
	Route       []store.RouteStep `json:"route,omitempty"`   // смены адресата: видит стажёр, адресаты и бадди
	DueAt       string            `json:"due_at,omitempty"`  // срок ответа текущего адресата (BR-18)
	Overdue     bool              `json:"overdue,omitempty"` // срок истёк
}

// questions — видимость по 02-ROLES:
// стажёр — свои; адресат — входящие; бадди — маршрут вопросов своих стажёров, текст по настройке стажёра (BR-86);
// руководитель — только количество, без текстов (H8).
func (s *Server) questions(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	var out []questionView
	count := 0
	for _, q := range s.Store.Questions() {
		asker, _ := s.Store.User(q.AskedBy)
		v := questionView{q.ID, q.AskedBy, q.AddresseeID, s.Store.Name(q.AddresseeID), q.Title, q.Text, false, q.Reason, q.Urgent, q.Status, q.Answer, q.PageID, "", q.Route, "", false}
		if _, due, ok := s.Store.QuestionSLA(q); ok && q.Status == "Sent" {
			v.DueAt, v.Overdue = due.Format(time.RFC3339), !store.Now().Before(due)
		}
		if q.AddresseeID == u.ID {
			v.Hint = q.Hint
		}
		switch {
		case q.AskedBy == u.ID || q.AddresseeID == u.ID:
			out = append(out, v)
		case asker != nil && asker.BuddyID == u.ID:
			if !s.Store.SettingsFor(asker).QuestionTextVisibleBuddy {
				v.Title, v.Text, v.Answer, v.Reason, v.TextHidden = "", "", "", "", true // BR-86: скрыт и текст, и ответ
			}
			out = append(out, v)
		case q.Status == "Open" && u.ID == s.Store.Data.Team.Department.KBOwnerID:
			out = append(out, v) // BR-37: открытые вопросы видит владелец базы, чтобы назначить, кто ответит
		case asker != nil && asker.LeadID == u.ID:
			count++
		}
	}
	writeJSON(w, 200, map[string]any{"questions": nonNil(out), "team_questions_count": count})
}

func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// drafts восстанавливает незавершённые действия после перезапуска сервера.
func (s *Server) drafts(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	out := []agent.Draft{}
	for _, d := range s.Store.PendingDrafts(u.ID) {
		out = append(out, agent.Draft{ID: d.ID, Tool: d.Tool, Args: d.Args, Version: d.Version, Status: "pending"})
	}
	writeJSON(w, 200, map[string]any{"drafts": out})
}
