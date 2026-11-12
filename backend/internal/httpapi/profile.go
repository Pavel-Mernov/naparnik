package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

// Профили сотрудников (карточка стажёра): spec/05, «Профили».

func (s *Server) profileResult(w http.ResponseWriter, v tools.ProfileView, e *tools.Error) {
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "profile": v})
}

func (s *Server) myProfile(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	v, e := tools.GetProfile(s.ctx(u), u.ID)
	s.profileResult(w, v, e)
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	v, e := tools.GetProfile(s.ctx(u), r.PathValue("id"))
	s.profileResult(w, v, e)
}

func (s *Server) profiles(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	role := r.URL.Query().Get("role")
	if role != "" && role != store.RoleIntern && role != store.RoleBuddy && role != store.RoleExpert && role != store.RoleLead {
		writeResult(w, tools.Result{Error: &tools.Error{Code: "validation", Message: "role: intern, buddy, expert или lead."}})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "profiles": tools.ListProfiles(s.ctx(u), role)})
}

// PATCH /api/profile {phone} — правится только телефон; остальное — из оргструктуры.
func (s *Server) editProfile(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		writeResult(w, tools.Result{Error: &tools.Error{Code: "validation", Message: "Некорректный запрос."}})
		return
	}
	var phone string
	for k, v := range body {
		if k != "phone" || json.Unmarshal(v, &phone) != nil {
			writeResult(w, tools.Result{Error: &tools.Error{Code: "validation", Message: "Править можно только телефон; ФИО, отдел и почта — из оргструктуры."}})
			return
		}
	}
	v, e := tools.UpdatePhone(s.ctx(u), phone)
	s.profileResult(w, v, e)
}

// PUT /api/profile/photo — тело запроса — само изображение (JPEG, PNG, WebP до 512 КБ).
func (s *Server) putPhoto(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	data, err := io.ReadAll(io.LimitReader(r.Body, tools.MaxPhotoBytes+1))
	if err != nil {
		writeResult(w, tools.Result{Error: &tools.Error{Code: "validation", Message: "Не удалось прочитать файл."}})
		return
	}
	v, e := tools.SetPhoto(s.ctx(u), data)
	s.profileResult(w, v, e)
}

func (s *Server) deletePhoto(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	v, e := tools.DeletePhoto(s.ctx(u))
	s.profileResult(w, v, e)
}

func (s *Server) photo(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	p, e := tools.PhotoOf(s.ctx(u), r.PathValue("id"))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600") // URL меняется при новом фото (?v=…)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(p.Data)
}
