package tools

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
)

func askSmirnova(t *testing.T, s *store.Store, text string) store.Question {
	t.Helper()
	r := Run(ctxFor(t, s, "intern.sokolov"), "send_question", map[string]any{"addressee_id": "u-smirnova", "text": text})
	if !r.OK {
		t.Fatal(r.Error)
	}
	return *r.Data.(*store.Question)
}

// SC-10: встреча с записью → выжимка с таймкодами → черновик страницы → «Ок» → база.
func TestMeetingWithRecording(t *testing.T) {
	s := newStore(t)
	q := askSmirnova(t, s, "Как выбрать длительность A/B-теста для низкого трафика?")
	olga := ctxFor(t, s, "expert.smirnova")

	if _, e := MeetingSlots(ctxFor(t, s, "expert.orlov"), q.ID); e == nil || e.Code != "forbidden" {
		t.Fatal("слоты — только адресату")
	}
	slots, e := MeetingSlots(olga, q.ID)
	// BR-41: пересечение рабочих часов и занятости — как в calendar.json (expected_slots MTG-301)
	want := []string{"2026-11-12T11:00:00+03:00", "2026-11-12T14:00:00+03:00", "2026-11-13T10:00:00+03:00"}
	if e != nil || strings.Join(slots, ",") != strings.Join(want, ",") {
		t.Fatalf("слоты: %v %v", slots, e)
	}
	if _, e := ScheduleMeeting(olga, q.ID, "2026-11-12T03:00:00+03:00"); e == nil || e.Code != "validation" {
		t.Fatal("ночной слот")
	}
	m, e := ScheduleMeeting(olga, q.ID, want[1])
	if e != nil || m.ID != "MTG-303" || m.Status != "scheduled" || m.End != "2026-11-12T14:30:00+03:00" {
		t.Fatalf("встреча: %+v %v", m, e)
	}
	// встреча — в «Ближайшем» стажёра в свой день
	s.Today = s.Today.AddDate(0, 0, 1)
	evs := Run(ctxFor(t, s, "intern.sokolov"), "get_today", nil).Data.(map[string]any)["events"].([]map[string]string)
	found := false
	for _, ev := range evs {
		found = found || (ev["start"] == "14:00" && strings.Contains(ev["title"], "Встреча по вопросу · Ольга Смирнова"))
	}
	if !found {
		t.Fatalf("в «Ближайшем»: %v", evs)
	}
	s.Today = s.Today.AddDate(0, 0, -1)
	if _, e := ScheduleMeeting(olga, q.ID, want[0]); e == nil {
		t.Fatal("вторая встреча по тому же вопросу")
	}
	// слот занят назначенной встречей — для нового вопроса его уже нет
	q2 := askSmirnova(t, s, "Как исключить ботов из выборки эксперимента?")
	if s2, _ := MeetingSlots(olga, q2.ID); strings.Contains(strings.Join(s2, ","), want[1]) {
		t.Fatal("занятый слот предложен снова")
	}
	if _, e := FinishMeeting(ctxFor(t, s, "intern.sokolov"), m.ID, true); e == nil || e.Code != "forbidden" {
		t.Fatal("итог отмечает организатор")
	}
	done, e := FinishMeeting(olga, m.ID, true)
	if e != nil || done.Status != "recorded" || done.FixtureID != "MTG-301" || done.PageID == "" {
		t.Fatalf("итог: %+v %v", done, e)
	}
	if _, e := FinishMeeting(olga, m.ID, true); e == nil {
		t.Fatal("повторный итог")
	}
	// BR-36: в черновике — ответ с таймкодами и ссылкой, без посторонних тем
	var page = struct{ Title, Body, Type, Status string }{}
	for _, p := range s.Pages() {
		if p.ID == done.PageID {
			page.Title, page.Body, page.Type, page.Status = p.Title, p.Body, p.Type, p.Status
		}
	}
	if page.Type != "meeting" || page.Status != "Draft" || page.Title != "Длительность A/B-теста при низком трафике" ||
		!strings.Contains(page.Body, "05:48–11:20") || !strings.Contains(page.Body, "vks.mock.local/rec/MTG-301") || strings.Contains(page.Body, "отпуск") {
		t.Fatalf("черновик: %+v", page)
	}
	for _, x := range s.Questions() {
		if x.ID == q.ID && (x.Status != "Answered" || x.PageID != done.PageID) {
			t.Fatalf("стажёр видит ответ: %+v", x)
		}
	}
	if pub, e := ApprovePage(olga, done.PageID, "", ""); e != nil || pub.Status != "Published" {
		t.Fatal("публикация")
	}
	// уведомления: стажёру — встреча и ответ; Ольге — проверить черновик
	types := map[string]bool{}
	for _, n := range append(s.NotificationsFor("u-sokolov"), s.NotificationsFor("u-smirnova")...) {
		types[n.Type] = true
	}
	for _, typ := range []string{"meeting_scheduled", "answer_ready", "draft_review", "answer_published"} {
		if !types[typ] {
			t.Errorf("нет уведомления %s", typ)
		}
	}
}

// SC-11: записи нет → одно предложение ответить текстом → отказ → «Открыт» у владельца базы (BR-37).
func TestMeetingWithoutRecording(t *testing.T) {
	s := newStore(t)
	q := askSmirnova(t, s, "Можно ли смотреть результаты эксперимента отдельно по регионам?")
	olga := ctxFor(t, s, "expert.smirnova")
	slots, _ := MeetingSlots(olga, q.ID)
	m, _ := ScheduleMeeting(olga, q.ID, slots[0])
	if _, e := LeaveOpen(olga, q.ID); e == nil {
		t.Fatal("открытым — только после встречи без записи")
	}
	if done, e := FinishMeeting(olga, m.ID, false); e != nil || done.Status != "no_recording" {
		t.Fatal(e)
	}
	if n := s.NotificationsFor("u-smirnova")[0]; n.Type != "meeting_no_recording" || !strings.Contains(n.Text, "Ответишь текстом") {
		t.Fatalf("предложение ответить текстом: %+v", n)
	}
	open, e := LeaveOpen(olga, q.ID)
	if e != nil || open.Status != "Open" {
		t.Fatal(e)
	}
	if n := s.NotificationsFor("u-kovalev")[0]; n.Type != "open_question" {
		t.Fatalf("владельцу базы: %+v", n)
	}
	if n := s.NotificationsFor("u-sokolov")[0]; n.Type != "question_open" {
		t.Fatalf("стажёру: %+v", n)
	}
	// вариант: после встречи без записи эксперт отвечает текстом
	q2 := askSmirnova(t, s, "Как выбрать длительность A/B-теста для низкого трафика?")
	s2, _ := MeetingSlots(olga, q2.ID)
	m2, _ := ScheduleMeeting(olga, q2.ID, s2[0])
	_, _ = FinishMeeting(olga, m2.ID, false)
	if _, _, e := AnswerQuestion(olga, q2.ID, "Минимум две недели, по MDE."); e != nil {
		t.Fatalf("ответ текстом после встречи без записи: %v", e)
	}
	if _, e := FinishMeeting(olga, "MTG-404", true); e == nil || e.Code != "not_found" {
		t.Fatal("нет встречи")
	}
	if _, e := MeetingSlots(olga, "Q-404"); e == nil || e.Code != "not_found" {
		t.Fatal("нет вопроса")
	}
}

// Запись есть, а подходящей расшифровки в демо нет — честный черновик без выдуманного ответа.
func TestMeetingWithoutFixture(t *testing.T) {
	s := newStore(t)
	q := askSmirnova(t, s, "Где взять шаблон презентации к экватору?")
	olga := ctxFor(t, s, "expert.smirnova")
	slots, _ := MeetingSlots(olga, q.ID)
	m, _ := ScheduleMeeting(olga, q.ID, slots[0])
	done, _ := FinishMeeting(olga, m.ID, true)
	for _, p := range s.Pages() {
		if p.ID == done.PageID && !strings.Contains(p.Body, "Расшифровки этой записи в демо нет") {
			t.Fatal(p.Body)
		}
	}
}
