package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func recordedMeeting(t *testing.T, srv *Server, h http.Handler) store.Meeting {
	t.Helper()
	u, _ := srv.Store.UserByLogin("intern.sokolov")
	r := tools.Run(tools.Ctx{Store: srv.Store, User: u}, "send_question", map[string]any{
		"addressee_id": "u-smirnova", "text": "Как выбрать длительность A/B-теста для низкого трафика?",
	})
	if !r.OK {
		t.Fatal(r.Error)
	}
	q := r.Data.(*store.Question)
	code, slots := call(t, h, "GET", "/api/questions/"+q.ID+"/meeting-slots", "expert.smirnova", "")
	if code != 200 {
		t.Fatal(code, slots)
	}
	code, scheduled := call(t, h, "POST", "/api/questions/"+q.ID+"/meeting", "expert.smirnova", jsonBody(map[string]any{"slot": slots["slots"].([]any)[0]}))
	if code != 200 {
		t.Fatal(code, scheduled)
	}
	id := scheduled["meeting"].(map[string]any)["id"].(string)
	if code, r := call(t, h, "POST", "/api/meetings/"+id+"/finish", "expert.smirnova", `{"recorded":true}`); code != 200 {
		t.Fatal(code, r)
	}
	return srv.Store.Meetings()[0]
}

func checkRecordingPrivacy(t *testing.T, srv *Server, h http.Handler, m store.Meeting) {
	t.Helper()
	for _, login := range []string{"intern.sokolov", "expert.smirnova", "intern.egorova", "buddy.belova", "lead.kovalev"} {
		_, meetings := call(t, h, "GET", "/api/meetings", login, "")
		participant := login == "intern.sokolov" || login == "expert.smirnova"
		if got := strings.Contains(jsonBody(meetings), m.RecordingURL); got != participant {
			t.Fatalf("%s: запись доступна только участникам: %v", login, meetings)
		}
		for _, path := range []string{"/api/kb/pages", "/api/kb/drafts", "/api/questions"} {
			_, data := call(t, h, "GET", path, login, "")
			if strings.Contains(jsonBody(data), m.RecordingURL) {
				t.Fatalf("%s %s раскрывает запись: %v", login, path, data)
			}
		}
	}
	// Общая статья остаётся полезной: выжимка и таймкоды доступны следующему стажёру.
	_, kb := call(t, h, "GET", "/api/kb/pages", "intern.egorova", "")
	if !strings.Contains(jsonBody(kb), "05:48–11:20") {
		t.Fatal("из статьи потеряны таймкоды")
	}
	u, _ := srv.Store.UserByLogin("intern.egorova")
	res := tools.Run(tools.Ctx{Store: srv.Store, User: u}, "search_kb", map[string]any{"query": "Длительность A/B-теста при низком трафике"})
	if !res.OK || !strings.Contains(jsonBody(res), m.PageID) || strings.Contains(jsonBody(res), m.RecordingURL) {
		t.Fatalf("поиск раскрывает запись или теряет выжимку: %v", res)
	}
}

// SC-10, BR-36: запись — в личной карточке встречи; статья, ответ и поиск — без URL.
func TestMeetingRecordingPrivacyAPI(t *testing.T) {
	srv, h := newServer(t)
	m := recordedMeeting(t, srv, h)
	if m.RecordingURL == "" {
		t.Fatal("нет записи у участников")
	}
	if code, r := call(t, h, "POST", "/api/kb/pages/"+m.PageID+"/approve", "expert.smirnova", "{}"); code != 200 {
		t.Fatal(code, r)
	}
	checkRecordingPrivacy(t, srv, h, m)
}

// Обновление существующей PostgreSQL-БД: старые статьи и ответы очищаются без потери записи.
func TestPostgresLegacyMeetingRecordingPrivacy(t *testing.T) {
	dsn := postgresSchema(t)
	srv, h := persistentServer(t, dsn)
	m := recordedMeeting(t, srv, h)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw []byte
	if err := db.QueryRow(`SELECT payload FROM naparnik_state WHERE id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var old map[string]any
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	body := "Выжимка ответа.\n\nОтвет на встрече: 05:48–11:20. Запись (только участникам): " + m.RecordingURL
	for _, p := range old["pages"].([]any) {
		if p := p.(map[string]any); p["id"] == m.PageID {
			p["body"], p["status"] = body, "Published"
		}
	}
	for _, q := range old["questions"].([]any) {
		if q := q.(map[string]any); q["id"] == m.QuestionID {
			q["answer"] = body
		}
	}
	raw, err = json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE naparnik_state SET payload=$1 WHERE id=1`, raw); err != nil {
		t.Fatal(err)
	}
	srv2, h2 := persistentServer(t, dsn)
	checkRecordingPrivacy(t, srv2, h2, m)
	// Любая следующая запись сохраняет очищенную версию; после перезапуска защита остаётся.
	if code, r := call(t, h2, "POST", "/api/feedback", "intern.egorova", jsonBody(map[string]any{"page_id": m.PageID, "value": "helped"})); code != 200 {
		t.Fatal(code, r)
	}
	srv3, h3 := persistentServer(t, dsn)
	checkRecordingPrivacy(t, srv3, h3, m)
}
