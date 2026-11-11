// Package httpapi — HTTP API для фронта (пути из docs/03-requirements/07-FRONTEND.md, раздел 9).
//
// Авторизация в прототипе — заголовок X-User с логином тестовой учётки (A5: в Авито авторизация уже есть,
// переключатель учёток в проде выключается feature flag).
package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

type Server struct {
	Agent     *agent.Agent
	Store     *store.Store
	StaticDir string // каталог фронта; пусто — фронт не отдаётся
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Refresh(r.Context()); err != nil {
			writeJSON(w, 503, errBody("unavailable", "Хранилище недоступно"))
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/practices", s.auth(s.practices))
	mux.HandleFunc("POST /api/practices/sync", s.auth(s.practiceSync))
	mux.HandleFunc("POST /api/practices/{intern}/{practice}/generate", s.auth(s.practiceGenerate))
	mux.HandleFunc("POST /api/practices/{intern}/{practice}/{action}", s.auth(s.practiceAction))
	mux.HandleFunc("GET /api/dashboard/team", s.auth(s.teamDashboard))
	mux.HandleFunc("POST /api/chat/messages", s.auth(s.chat))
	mux.HandleFunc("POST /api/chat/reset", s.auth(s.chatReset))
	mux.HandleFunc("GET /api/drafts", s.auth(s.drafts))
	mux.HandleFunc("POST /api/drafts/{id}/confirm", s.auth(s.confirm))
	mux.HandleFunc("POST /api/requests/{id}/confirm", s.auth(s.confirm))       // заявки (отгул, доступ)
	mux.HandleFunc("POST /api/questions/{id}/confirm-send", s.auth(s.confirm)) // BR-11: вопрос человеку
	mux.HandleFunc("GET /api/plans/{internId}", s.auth(s.plan))
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
	mux.HandleFunc("GET /api/kb/pages", s.auth(s.kbPages))
	mux.HandleFunc("GET /api/kb/drafts", s.auth(s.kbDrafts))
	mux.HandleFunc("POST /api/kb/pages/{id}/approve", s.auth(s.kbApprove))
	mux.HandleFunc("POST /api/questions/{id}/answer", s.auth(s.answer))
	if s.StaticDir != "" {
		files := http.FileServer(http.Dir(s.StaticDir))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// no-cache: браузер каждый раз сверяется с сервером — после обновления кода не остаётся старая версия
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
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

var statusByCode = map[string]int{"forbidden": 403, "not_found": 404, "validation": 422, "unavailable": 503}

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
	reply, err := s.Agent.Ask(r.Context(), u.Login, body.Text)
	if err != nil {
		writeJSON(w, 500, errBody("internal", err.Error()))
		return
	}
	writeJSON(w, 200, reply)
}

func (s *Server) chatReset(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	s.Agent.Reset(u.Login)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	writeResult(w, s.Agent.Confirm(u.Login, r.PathValue("id")))
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
	ID          string `json:"id"`
	AskedBy     string `json:"asked_by"`
	AddresseeID string `json:"addressee_id"`
	Addressee   string `json:"addressee"`
	Title       string `json:"title,omitempty"`
	Text        string `json:"text,omitempty"`
	TextHidden  bool   `json:"text_hidden,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Urgent      bool   `json:"urgent"`
	Status      string `json:"status"`
	Answer      string `json:"answer,omitempty"`
	PageID      string `json:"page_id,omitempty"`
}

// questions — видимость по 02-ROLES:
// стажёр — свои; адресат — входящие; бадди — маршрут вопросов своих стажёров, текст по настройке стажёра (BR-86);
// руководитель — только количество, без текстов (H8).
func (s *Server) questions(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	var out []questionView
	count := 0
	for _, q := range s.Store.Questions() {
		asker, _ := s.Store.User(q.AskedBy)
		v := questionView{q.ID, q.AskedBy, q.AddresseeID, s.Store.Name(q.AddresseeID), q.Title, q.Text, false, q.Reason, q.Urgent, q.Status, q.Answer, q.PageID}
		switch {
		case q.AskedBy == u.ID || q.AddresseeID == u.ID:
			out = append(out, v)
		case asker != nil && asker.BuddyID == u.ID:
			if asker.Settings != nil && !asker.Settings.QuestionTextVisibleBuddy {
				v.Title, v.Text, v.Answer, v.Reason, v.TextHidden = "", "", "", "", true // BR-86: скрыт и текст, и ответ
			}
			out = append(out, v)
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
		out = append(out, agent.Draft{ID: d.ID, Tool: d.Tool, Args: d.Args})
	}
	writeJSON(w, 200, map[string]any{"drafts": out})
}
