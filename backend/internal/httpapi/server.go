// Package httpapi — HTTP API для фронта (пути из docs/03-requirements/07-FRONTEND.md, раздел 9).
//
// Авторизация в прототипе — заголовок X-User с логином тестовой учётки (A5: в Авито авторизация уже есть,
// переключатель учёток в проде выключается feature flag).
package httpapi

import (
	"encoding/json"
	"net/http"

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
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("POST /api/chat/messages", s.auth(s.chat))
	mux.HandleFunc("POST /api/drafts/{id}/confirm", s.auth(s.confirm))
	mux.HandleFunc("POST /api/requests/{id}/confirm", s.auth(s.confirm))       // заявки (отгул, доступ)
	mux.HandleFunc("POST /api/questions/{id}/confirm-send", s.auth(s.confirm)) // BR-11: вопрос человеку
	mux.HandleFunc("GET /api/plans/{internId}", s.auth(s.plan))
	mux.HandleFunc("PATCH /api/plan-steps/{id}", s.auth(s.markStep))
	mux.HandleFunc("GET /api/requests", s.auth(s.requests))
	mux.HandleFunc("GET /api/questions", s.auth(s.questions))
	mux.HandleFunc("GET /api/today", s.auth(s.today))
	mux.HandleFunc("GET /api/users", s.users)
	if s.StaticDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(s.StaticDir)))
	}
	return mux
}

type handler func(http.ResponseWriter, *http.Request, *mockdata.User)

func (s *Server) auth(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.Store.UserByLogin(r.Header.Get("X-User"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized", "Укажи тестовую учётку в заголовке X-User"))
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
	writeResult(w, t.Run(c, map[string]any{"step_id": r.PathValue("id"), "intern_id": body.InternID}))
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

type questionView struct {
	ID          string `json:"id"`
	AskedBy     string `json:"asked_by"`
	AddresseeID string `json:"addressee_id"`
	Addressee   string `json:"addressee"`
	Text        string `json:"text,omitempty"`
	TextHidden  bool   `json:"text_hidden,omitempty"`
	Urgent      bool   `json:"urgent"`
	Status      string `json:"status"`
}

// questions — видимость по 02-ROLES:
// стажёр — свои; адресат — входящие; бадди — маршрут вопросов своих стажёров, текст по настройке стажёра (BR-86);
// руководитель — только количество, без текстов (H8).
func (s *Server) questions(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	var out []questionView
	count := 0
	for _, q := range s.Store.Questions() {
		asker, _ := s.Store.User(q.AskedBy)
		v := questionView{q.ID, q.AskedBy, q.AddresseeID, s.Store.Name(q.AddresseeID), q.Text, false, q.Urgent, q.Status}
		switch {
		case q.AskedBy == u.ID || q.AddresseeID == u.ID:
			out = append(out, v)
		case asker != nil && asker.BuddyID == u.ID:
			if asker.Settings != nil && !asker.Settings.QuestionTextVisibleBuddy {
				v.Text, v.TextHidden = "", true
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
