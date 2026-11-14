package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

type planBody struct {
	Version *int             `json:"version"`
	Steps   []tools.PlanEdit `json:"steps,omitempty"`
	Comment string           `json:"comment,omitempty"`
}

func readPlanBody(w http.ResponseWriter, r *http.Request) (planBody, bool) {
	var b planBody
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&b); err != nil {
		writeJSON(w, 422, errBody("validation", "Некорректный запрос плана."))
		return b, false
	}
	if err := d.Decode(new(any)); err != io.EOF || b.Version == nil || *b.Version < 1 {
		writeJSON(w, 422, errBody("validation", "Передай актуальную версию плана."))
		return b, false
	}
	return b, true
}
func planResponse(w http.ResponseWriter, v tools.PlanDraftView, e *tools.Error) {
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) planDraft(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	v, e := tools.GetPlanDraft(s.ctx(u), r.PathValue("internId"))
	planResponse(w, v, e)
}
func (s *Server) planDraftEdit(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	b, ok := readPlanBody(w, r)
	if !ok {
		return
	}
	v, e := tools.EditPlanDraft(s.ctx(u), r.PathValue("internId"), b.Steps, b.Comment, *b.Version)
	planResponse(w, v, e)
}
func (s *Server) planApprove(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	b, ok := readPlanBody(w, r)
	if !ok {
		return
	}
	v, e := tools.ApprovePlan(s.ctx(u), r.PathValue("internId"), *b.Version)
	planResponse(w, v, e)
}
func (s *Server) planAction(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	b, ok := readPlanBody(w, r)
	if !ok {
		return
	}
	var v tools.PlanDraftView
	var e *tools.Error
	c, id := s.ctx(u), r.PathValue("internId")
	switch r.PathValue("action") {
	case "start":
		v, e = tools.StartPlanRevision(c, id, *b.Version)
	case "submit":
		v, e = tools.SubmitPlanDraft(c, id, *b.Version)
	case "return":
		v, e = tools.ReturnPlan(c, id, *b.Version, b.Comment)
	case "cancel":
		v, e = tools.CancelPlanRevision(c, id, *b.Version)
	default:
		writeJSON(w, 404, errBody("not_found", "Действие не найдено."))
		return
	}
	planResponse(w, v, e)
}
func (s *Server) planHistory(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	h, e := tools.GetPlanHistory(s.ctx(u), r.PathValue("internId"))
	if e != nil {
		writeResult(w, tools.Result{Error: e})
		return
	}
	writeJSON(w, 200, h)
}
