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

func (s *Server) to(r *http.Request) string {
	var body struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	return body.To
}

func (s *Server) questionResult(w http.ResponseWriter, q store.Question, e *tools.Error) {
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "addressee_id": q.AddresseeID, "addressee": s.Store.Name(q.AddresseeID), "status": q.Status})
}

func (s *Server) notMine(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	q, e := tools.NotMine(s.ctx(u), r.PathValue("id"), s.to(r))
	s.questionResult(w, q, e)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	q, e := tools.Redirect(s.ctx(u), r.PathValue("id"), s.to(r))
	s.questionResult(w, q, e)
}

func (s *Server) assign(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	q, e := tools.AssignOpen(s.ctx(u), r.PathValue("id"), s.to(r))
	s.questionResult(w, q, e)
}

func (s *Server) kbMaintenance(w http.ResponseWriter, _ *http.Request, u *mockdata.User) {
	m, e := tools.KBMaintenance(s.ctx(u))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) kbDuplicate(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Decision string `json:"decision"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	d, e := tools.DecideDuplicate(s.ctx(u), r.PathValue("id"), body.Decision)
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "duplicate": d})
}

func (s *Server) kbRemind(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	if e := tools.RemindDraft(s.ctx(u), r.PathValue("id")); e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
