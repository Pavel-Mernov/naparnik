package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

const practicePath = "/api/practices/u-sokolov/pr-01/"
const goodPracticeAnswer = `WITH daily AS (
 SELECT event_date, sum(contacts) AS contacts, sum(views) AS views
 FROM sandbox.training_ads_mart
 WHERE event_date BETWEEN '2026-11-16' AND '2026-11-22' AND is_active = 1
 GROUP BY event_date
) SELECT event_date, 1.0 * contacts / nullIf(views, 0) AS conversion FROM daily;
Таблица sandbox.sokolov_ads_conv_20261124
График «Конверсия карточки в контакт по дням», папка «Стажёры / Черновики»`

func jsonBody(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func practiceItem(t *testing.T, h http.Handler, login string) map[string]any {
	t.Helper()
	code, r := call(t, h, "GET", "/api/practices", login, "")
	if code != 200 {
		t.Fatal(code, r)
	}
	items := r["data"].([]any)
	if len(items) != 1 {
		t.Fatal(r)
	}
	return items[0].(map[string]any)
}

func preparePracticeTest(t *testing.T, h http.Handler) int {
	t.Helper()
	if code, r := call(t, h, "POST", "/api/practices/sync", "intern.sokolov", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	v := practiceItem(t, h, "buddy.belova")
	text := v["task"].(map[string]any)["text"].(string) + " Задание проверено бадди."
	body := jsonBody(map[string]any{"text": text, "task_version": v["task_version"]})
	for range 2 {
		if code, r := call(t, h, "POST", practicePath+"approve", "buddy.belova", body); code != 200 {
			t.Fatal(code, r)
		}
	}
	v = practiceItem(t, h, "intern.sokolov")
	if !strings.Contains(v["task"].(map[string]any)["text"].(string), "проверено бадди") {
		t.Fatal(v)
	}
	return int(v["task_version"].(float64))
}

func advancePracticeTest(t *testing.T, h http.Handler, action string) map[string]any {
	t.Helper()
	v := practiceItem(t, h, "intern.sokolov")
	body := jsonBody(map[string]any{"action": action, "expected_stage": v["stage"]})
	code, r := call(t, h, "POST", practicePath+"advance", "intern.sokolov", body)
	if code != 200 {
		t.Fatal(code, r)
	}
	return r["data"].(map[string]any)
}

func trainingStage(t *testing.T, h http.Handler) {
	t.Helper()
	advancePracticeTest(t, h, "start")
	advancePracticeTest(t, h, "next")
	if v := advancePracticeTest(t, h, "next"); v["stage"] != "training" {
		t.Fatal(v)
	}
}

func attemptPractice(t *testing.T, h http.Handler, version int, answer, key string) (int, map[string]any) {
	t.Helper()
	return keyedCall(h, "POST", practicePath+"attempts", "intern.sokolov", jsonBody(map[string]any{"answer": answer, "task_version": version}), key)
}

// SC-30/31: одобрение → ошибочная попытка → исправление → только лучшая попытка у бадди.
func TestPracticeWorkflowAndPrivacy(t *testing.T) {
	s, h := newServer(t)
	s.Store.Data.Practice.Templates[0].Task.Text += " PRIVATE-DRAFT"
	if code, r := call(t, h, "POST", "/api/practices/sync", "intern.sokolov", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	v := practiceItem(t, h, "intern.sokolov")
	raw, _ := json.Marshal(v)
	if v["task"] != nil || strings.Contains(string(raw), "PRIVATE-DRAFT") {
		t.Fatal("черновик доступен стажёру", v)
	}
	if code, _ := attemptPractice(t, h, 1, goodPracticeAnswer, "before-approval"); code != 422 {
		t.Fatal("попытка без одобрения", code)
	}
	version := preparePracticeTest(t, h)
	trainingStage(t, h)
	bad := strings.ReplaceAll(strings.ReplaceAll(goodPracticeAnswer, "1.0 * ", ""), "event_date BETWEEN '2026-11-16' AND '2026-11-22' AND ", "") + "\nPRIVATE-FIRST"
	code, r := attemptPractice(t, h, version, bad, "attempt-1")
	if code != 200 {
		t.Fatal(code, r)
	}
	v = r["data"].(map[string]any)
	a := v["attempts"].([]any)[0].(map[string]any)["review"].(map[string]any)
	if a["passed"] != false || v["attempts_count"] != float64(1) {
		t.Fatal(v)
	}
	for range 2 {
		if code, r = attemptPractice(t, h, version, goodPracticeAnswer, "attempt-2"); code != 200 {
			t.Fatal(code, r)
		}
	}
	v = practiceItem(t, h, "buddy.belova")
	raw, _ = json.Marshal(v)
	if v["attempts"] != nil || strings.Contains(string(raw), "PRIVATE-FIRST") || v["attempts_count"] != float64(2) || v["best_attempt"].(map[string]any)["number"] != float64(2) {
		t.Fatal("лишние данные бадди", v)
	}
	if len(v["error_topics"].([]any)) != 2 {
		t.Fatal(v["error_topics"])
	}
	if code, _ := attemptPractice(t, h, version, goodPracticeAnswer+" другое", "attempt-2"); code != 422 {
		t.Fatal(code)
	}
	if code, _ := attemptPractice(t, h, version, goodPracticeAnswer, ""); code != 422 {
		t.Fatal(code)
	}
	v = advancePracticeTest(t, h, "next")
	if v["stage"] != "real_task" || len(v["checklist"].([]any)) == 0 {
		t.Fatal(v)
	}
	for _, step := range s.Store.PlanSteps("u-sokolov") {
		if step.StepID == "s-13" && step.Status == "Done" {
			t.Fatal("учебная практика отметила реальную задачу")
		}
	}
}

// SC-32: пропуски и повтор запроса не перескакивают несколько этапов.
func TestPracticeSkipAndDeadline(t *testing.T) {
	s, h := newServer(t)
	s.Store.Today = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) // пятница
	if code, r := call(t, h, "POST", "/api/practices/sync", "intern.sokolov", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	v := practiceItem(t, h, "buddy.belova")
	if v["approval_due"] != "2026-11-16" {
		t.Fatal(v)
	}
	advancePracticeTest(t, h, "start")
	body := `{"action":"next","expected_stage":"general_terms"}`
	for range 2 {
		if code, r := call(t, h, "POST", practicePath+"advance", "intern.sokolov", body); code != 200 || r["data"].(map[string]any)["stage"] != "team_terms" {
			t.Fatal(code, r)
		}
	}
	advancePracticeTest(t, h, "next")
	s.Store.Today = s.Store.Today.AddDate(0, 0, 2) // воскресенье — ещё не рабочий день
	if v = practiceItem(t, h, "intern.sokolov"); v["task"] != nil || v["approval_expired"] != false {
		t.Fatal(v)
	}
	if code, _ := call(t, h, "POST", practicePath+"advance", "intern.sokolov", `{"action":"next","expected_stage":"training"}`); code != 422 {
		t.Fatal(code)
	}
	s.Store.Today = s.Store.Today.AddDate(0, 0, 1)
	if v = advancePracticeTest(t, h, "next"); v["stage"] != "real_task" {
		t.Fatal(v)
	}
	if v = practiceItem(t, h, "buddy.belova"); v["task_status"] != "Draft" || v["task"] == nil {
		t.Fatal("черновик потерян", v)
	}
	// Отказ от всей практики работает и до генерации.
	s, h = newServer(t)
	for range 2 {
		if v = advancePracticeTest(t, h, "skip_all"); v["stage"] != "real_task" {
			t.Fatal(v)
		}
	}
	st, _ := s.Store.Practice("u-sokolov", "pr-01")
	if len(st.Events) != 1 || st.Events[0].Type != "practice_skipped" {
		t.Fatal(st)
	}
}

// BR-106: лимит 3, помощь — только с отдельным подтверждением, без частных ответов в вопросе.
func TestPracticeAttemptsLimitAndHelp(t *testing.T) {
	s, h := newServer(t)
	version := preparePracticeTest(t, h)
	trainingStage(t, h)
	for _, key := range []string{"1", "2", "3"} {
		if code, r := attemptPractice(t, h, version, "PRIVATE-ATTEMPT "+key, key); code != 200 {
			t.Fatal(code, r)
		}
	}
	if code, _ := attemptPractice(t, h, version, goodPracticeAnswer, "4"); code != 422 {
		t.Fatal(code)
	}
	var id string
	for range 2 {
		code, r := call(t, h, "POST", practicePath+"help-draft", "intern.sokolov", "{}")
		if code != 200 {
			t.Fatal(code, r)
		}
		got := r["data"].(map[string]any)["id"].(string)
		if id != "" && id != got {
			t.Fatal(r)
		}
		id = got
	}
	if len(s.Store.Questions()) != 0 {
		t.Fatal("вопрос без подтверждения")
	}
	code, r := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", "{}")
	if code != 200 {
		t.Fatal(code, r)
	}
	q := s.Store.Questions()[0]
	if q.AddresseeID != "u-belova" || strings.Contains(q.Text, "PRIVATE-ATTEMPT") {
		t.Fatal(q)
	}
}

// BR-40/105: другие роли, чужие стажёры, версия, отозванные источники, сбой модели.
func TestPracticeAuthorizationAndFailures(t *testing.T) {
	s, h := newServer(t)
	s.Store.Data.Team.Users = append(s.Store.Data.Team.Users, mockdata.User{ID: "other-buddy", Login: "other-buddy", Roles: []string{"buddy"}})
	for _, tc := range []struct {
		login, path string
		code        int
	}{
		{"", "/api/practices", 401}, {"lead.kovalev", "/api/practices", 403}, {"expert.orlov", "/api/practices", 403},
	} {
		if code, _ := call(t, h, "GET", tc.path, tc.login, ""); code != tc.code {
			t.Fatal(tc, code)
		}
	}
	version := preparePracticeTest(t, h)
	trainingStage(t, h)
	for _, login := range []string{"intern.egorova", "lead.kovalev", "expert.orlov", "other-buddy"} {
		if code, _ := call(t, h, "POST", practicePath+"approve", login, `{"text":"bad","task_version":1}`); code != 403 {
			t.Fatal(login, code)
		}
	}
	if code, _ := call(t, h, "POST", practicePath+"approve", "intern.sokolov", `{"text":"bad","task_version":1}`); code != 403 {
		t.Fatal(code)
	}
	if code, _ := attemptPractice(t, h, version-1, goodPracticeAnswer, "stale"); code != 422 {
		t.Fatal(code)
	}
	s.Agent.Provider = llm.NewStub("stub_error", s.Store.Today)
	if code, _ := attemptPractice(t, h, version, goodPracticeAnswer, "retry"); code != 503 {
		t.Fatal(code)
	}
	if practiceItem(t, h, "intern.sokolov")["attempts_count"] != float64(0) {
		t.Fatal("потрачена попытка при сбое")
	}
	s.Agent.Provider = llm.NewStub("stub_ok", s.Store.Today)
	if code, r := attemptPractice(t, h, version, goodPracticeAnswer, "retry"); code != 200 {
		t.Fatal(code, r)
	}
	s.Store.UpdatePage("kb-35", func(p *mockdata.Page) { p.Space = "space-dwh-pii"; p.Title = "PRIVATE-TITLE"; p.Body = "PRIVATE-BODY" })
	v := practiceItem(t, h, "intern.sokolov")
	raw, _ := json.Marshal(v)
	if v["available"] != false || v["task"] != nil || strings.Contains(string(raw), "PRIVATE-") {
		t.Fatal(v)
	}
	if code, _ := attemptPractice(t, h, version, goodPracticeAnswer, "retry"); code != 403 {
		t.Fatal("кэш обошёл новые права", code)
	}
}

// Ошибка генерации не блокирует реальную задачу, повтор не возвращает стажёра в пройденные этапы.
func TestPracticeGenerationFailureAndRetry(t *testing.T) {
	s, h := newServer(t)
	s.Agent.Provider = llm.NewStub("stub_error", s.Store.Today)
	if code, _ := call(t, h, "POST", "/api/practices/sync", "intern.sokolov", "{}"); code != 503 {
		t.Fatal(code)
	}
	v := practiceItem(t, h, "intern.sokolov")
	if v["task"] != nil || v["task_status"] != "Pending" {
		t.Fatal(v)
	}
	advancePracticeTest(t, h, "skip_all")
	s.Agent.Provider = llm.NewStub("stub_ok", s.Store.Today)
	if code, r := call(t, h, "POST", practicePath+"generate", "buddy.belova", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	v = practiceItem(t, h, "intern.sokolov")
	if v["stage"] != "real_task" || v["task"] != nil {
		t.Fatal(v)
	}
	if code, _ := call(t, h, "POST", practicePath+"generate", "intern.sokolov", "{}"); code != 403 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "POST", practicePath+"advance", "intern.sokolov", `{"action":"delete"}`); code != 422 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "POST", practicePath+"advance", "intern.sokolov", `{"action":"next"}`); code != 422 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "POST", practicePath+"advance", "intern.sokolov", `{"action":"start"}{"action":"skip_all"}`); code != 422 {
		t.Fatal(code)
	}
}
