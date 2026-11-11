package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func (s *Server) ctx(u *mockdata.User) tools.Ctx { return tools.Ctx{Store: s.Store, User: u} }

func (s *Server) meetingSlots(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	slots, e := tools.MeetingSlots(s.ctx(u), r.PathValue("id"))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "slots": slots})
}

func (s *Server) scheduleMeeting(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Slot string `json:"slot"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	m, e := tools.ScheduleMeeting(s.ctx(u), r.PathValue("id"), body.Slot)
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "meeting": m})
}

func (s *Server) finishMeeting(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Recorded bool `json:"recorded"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	m, e := tools.FinishMeeting(s.ctx(u), r.PathValue("id"), body.Recorded)
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "meeting": m})
}

func (s *Server) leaveOpen(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	q, e := tools.LeaveOpen(s.ctx(u), r.PathValue("id"))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "status": q.Status})
}

// meetings — мои встречи по вопросам (участник видит; запись и транскрипт — только участникам, BR-36).
func (s *Server) meetings(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	out := []store.Meeting{}
	for _, m := range s.Store.Meetings() {
		for _, p := range m.Participants {
			if p == u.ID {
				out = append(out, m)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"meetings": out})
}
