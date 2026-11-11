package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func (s *Server) practices(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	writeResult(w, tools.PracticeList(tools.Ctx{Store: s.Store, User: u}))
}

func (s *Server) practiceSync(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	writeResult(w, s.Agent.SyncPractices(r.Context(), u))
}

func (s *Server) practiceGenerate(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	if !store.HasRole(u, store.RoleBuddy) {
		writeResult(w, tools.Result{Error: &tools.Error{Code: "forbidden", Message: "Подготовкой задания управляет бадди."}})
		return
	}
	writeResult(w, s.Agent.GeneratePractice(r.Context(), u, r.PathValue("intern"), r.PathValue("practice")))
}

func (s *Server) practiceAction(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	var body struct {
		Text    string `json:"text"`
		Answer  string `json:"answer"`
		Version int    `json:"task_version"`
		Action  string `json:"action"`
		Stage   string `json:"expected_stage"`
	}
	if r.PathValue("action") != "help-draft" {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20000))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
			writeResult(w, tools.Result{Error: &tools.Error{Code: "validation", Message: "Проверь поля и размер запроса практики."}})
			return
		}
	}
	c := tools.Ctx{Store: s.Store, User: u}
	intern, id := r.PathValue("intern"), r.PathValue("practice")
	var result tools.Result
	switch r.PathValue("action") {
	case "approve":
		result = s.Agent.ApprovePractice(u, intern, id, body.Text, body.Version)
	case "advance":
		result = tools.AdvancePractice(c, intern, id, body.Action, body.Stage)
	case "attempts":
		result = s.Agent.SubmitPractice(r.Context(), u, intern, id, r.Header.Get("Idempotency-Key"), body.Answer, body.Version)
	case "help-draft":
		result = tools.PracticeHelpDraft(c, intern, id)
	default:
		result = tools.Result{Error: &tools.Error{Code: "not_found", Message: "Такого действия практики нет."}}
	}
	writeResult(w, result)
}
