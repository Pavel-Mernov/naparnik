package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func decodeDraftBody(r *http.Request, out any, optional bool) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	err = d.Decode(out)
	if err == io.EOF && optional {
		return true
	}
	return err == nil && d.Decode(new(any)) == io.EOF
}

func (s *Server) draft(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	d, e := s.Agent.GetDraft(u.Login, r.PathValue("id"))
	s.draftResult(w, d, e)
}

func (s *Server) draftEdit(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var edit agent.QuestionEdit
	if !decodeDraftBody(r, &edit, false) || edit.Version < 1 {
		writeJSON(w, 422, errBody("validation", "Нужны версия черновика и изменения текста или срочности."))
		return
	}
	d, e := s.Agent.EditQuestionDraft(u.Login, r.PathValue("id"), edit)
	s.draftResult(w, d, e)
}

func (s *Server) draftCancel(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Version int `json:"version"`
	}
	if !decodeDraftBody(r, &body, false) || body.Version < 1 {
		writeJSON(w, 422, errBody("validation", "Укажи версию отменяемого черновика."))
		return
	}
	d, e := s.Agent.CancelDraft(u.Login, r.PathValue("id"), body.Version)
	s.draftResult(w, d, e)
}

func (s *Server) draftResult(w http.ResponseWriter, d agent.Draft, e *tools.Error) {
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "draft": d})
}
