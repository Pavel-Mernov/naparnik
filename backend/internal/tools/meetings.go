package tools

import (
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Ответ через встречу (F3, SC-10, SC-11, US-14…16): эксперт выбирает встречу → 3 слота в рабочие часы обоих (BR-41)
// → встреча с напоминанием включить запись (BR-35) → после встречи выжимка ответа с таймкодами в черновик страницы (BR-36)
// или, если записи нет, одно предложение ответить текстом; отказ — вопрос «Открыт» у владельца базы (BR-37).
// ВКС в прототипе — заглушка: запись и транскрипт берутся из docs/04-mock-data/meetings.json.

const meetingLen = 30 * time.Minute

func questionFor(c Ctx, id string) (store.Question, *Error) {
	for _, q := range c.Store.Questions() {
		if q.ID == id {
			if q.AddresseeID != c.User.ID {
				return q, &Error{Code: "forbidden", Message: "Встречу по вопросу назначает его адресат."}
			}
			return q, nil
		}
	}
	return store.Question{}, &Error{Code: "not_found", Message: "Вопрос не найден."}
}

// busy — занятость человека: события календаря, явная занятость и уже назначенные встречи.
func busy(c Ctx, userID string) [][2]time.Time {
	var out [][2]time.Time
	add := func(a, b string) {
		s, e1 := time.Parse(time.RFC3339, a)
		e, e2 := time.Parse(time.RFC3339, b)
		if e1 == nil && e2 == nil {
			out = append(out, [2]time.Time{s, e})
		}
	}
	for _, e := range c.Store.Data.Calendar.Events {
		if containsStr(e.Participants, userID) {
			end := e.End
			if end == "" {
				if s, err := time.Parse(time.RFC3339, e.Start); err == nil {
					end = s.Add(time.Hour).Format(time.RFC3339)
				}
			}
			add(e.Start, end)
		}
	}
	for _, r := range c.Store.Data.Calendar.Busy[userID] {
		add(r.Start, r.End)
	}
	for _, m := range c.Store.Meetings() {
		if containsStr(m.Participants, userID) {
			add(m.Start, m.End)
		}
	}
	return out
}

func free(slot time.Time, ranges [][2]time.Time) bool {
	end := slot.Add(meetingLen)
	for _, r := range ranges {
		if slot.Before(r[1]) && end.After(r[0]) {
			return false
		}
	}
	return true
}

// MeetingSlots — 3 слота по 30 минут в пересечении рабочих часов обоих (BR-41), начиная со следующего рабочего дня:
// первый свободный до обеда и после обеда каждого дня — чтобы было из чего выбрать.
func MeetingSlots(c Ctx, questionID string) ([]string, *Error) {
	q, e := questionFor(c, questionID)
	if e != nil {
		return nil, e
	}
	if q.Status != "Sent" {
		return nil, &Error{Code: "validation", Message: "По этому вопросу уже есть ответ или встреча."}
	}
	return slotsFor(c, q), nil
}

func slotsFor(c Ctx, q store.Question) []string {
	me, _ := c.Store.User(q.AddresseeID)
	asker, _ := c.Store.User(q.AskedBy)
	if me == nil || asker == nil {
		return nil
	}
	busyAll := append(busy(c, me.ID), busy(c, asker.ID)...)
	var out []string
	day := time.Date(c.Store.Today.Year(), c.Store.Today.Month(), c.Store.Today.Day(), 0, 0, 0, 0, store.MSK)
	for d := 1; d <= 10 && len(out) < 3; d++ {
		date := day.AddDate(0, 0, d)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		s1, e1, ok1 := store.WorkingWindow(me.WorkingHours, date)
		s2, e2, ok2 := store.WorkingWindow(asker.WorkingHours, date)
		if !ok1 || !ok2 {
			continue
		}
		start, end := later(s1, s2), earlier(e1, e2)
		noon := time.Date(date.Year(), date.Month(), date.Day(), 13, 0, 0, 0, store.MSK)
		for _, half := range [][2]time.Time{{start, earlier(noon, end)}, {later(noon, start), end}} {
			for t := half[0]; !t.Add(meetingLen).After(half[1]); t = t.Add(30 * time.Minute) {
				if free(t, busyAll) {
					out = append(out, t.Format(time.RFC3339))
					break
				}
			}
			if len(out) == 3 {
				break
			}
		}
	}
	return out
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// ScheduleMeeting — эксперт выбрал слот (US-14). Встреча создаётся только по его решению (BR-41).
func ScheduleMeeting(c Ctx, questionID, slot string) (store.Meeting, *Error) {
	return atomic(c, func(c Ctx) (store.Meeting, *Error) {
		q, e := questionFor(c, questionID)
		if e != nil {
			return store.Meeting{}, e
		}
		if q.Status != "Sent" {
			return store.Meeting{}, &Error{Code: "validation", Message: "По этому вопросу уже есть ответ или встреча."}
		}
		if !containsStr(slotsFor(c, q), slot) {
			return store.Meeting{}, &Error{Code: "validation", Message: "Этот слот уже занят — выбери другой."}
		}
		start, _ := time.Parse(time.RFC3339, slot)
		m := &store.Meeting{ID: c.Store.NextID("MTG"), QuestionID: q.ID, OrganizerID: c.User.ID, Participants: []string{c.User.ID, q.AskedBy},
			Title: "Вопрос стажёра: " + q.Title, Start: slot, End: start.Add(meetingLen).Format(time.RFC3339), Status: "scheduled"}
		c.Store.AddMeeting(m)
		c.Store.UpdateQuestion(q.ID, func(x *store.Question) { x.Status, x.MeetingID = "MeetingScheduled", m.ID })
		c.Store.Emit(store.EvMeetingScheduled, c.User.ID, m.ID, map[string]string{"asker": q.AskedBy, "start": slot})
		return *m, nil
	})
}

// fixtureFor — запись из заглушки ВКС, подходящая к вопросу (по смыслу заголовка и описания встречи).
func fixtureFor(c Ctx, q store.Question) *mockdata.MeetingFixture {
	query := Stems(q.Title + " " + q.Text)
	var best *mockdata.MeetingFixture
	bestScore := 0
	for i, f := range c.Store.Data.Meetings.Meetings {
		if f.RecordingStatus != "recorded" || f.ExpectedSummary == nil {
			continue
		}
		text := f.Title + " " + f.Description
		for _, mq := range c.Store.Data.Questions.Questions {
			if mq.ID == f.QuestionID {
				text += " " + mq.Text
			}
		}
		if n := overlap(query, Stems(text)); n > bestScore {
			best, bestScore = &c.Store.Data.Meetings.Meetings[i], n
		}
	}
	if bestScore < 2 {
		return nil
	}
	return best
}

// FinishMeeting — встреча прошла (в проде — сигнал ВКС; в демо — отметка организатора).
// Запись есть → выжимка только по исходному вопросу с таймкодами без URL записи (BR-36) → черновик страницы, стажёр видит ответ.
// Записи нет → один раз предлагаем ответить текстом (BR-37).
func FinishMeeting(c Ctx, meetingID string, recorded bool) (store.Meeting, *Error) {
	return atomic(c, func(c Ctx) (store.Meeting, *Error) {
		var m *store.Meeting
		for _, x := range c.Store.Meetings() {
			if x.ID == meetingID {
				m = &x
			}
		}
		switch {
		case m == nil:
			return store.Meeting{}, &Error{Code: "not_found", Message: "Встреча не найдена."}
		case m.OrganizerID != c.User.ID:
			return store.Meeting{}, &Error{Code: "forbidden", Message: "Итог встречи отмечает организатор."}
		case m.Status != "scheduled":
			return store.Meeting{}, &Error{Code: "validation", Message: "Итог встречи уже отмечен."}
		}
		q, _ := questionFor(c, m.QuestionID)
		if !recorded {
			out, _ := c.Store.UpdateMeeting(m.ID, func(x *store.Meeting) { x.Status = "no_recording" })
			c.Store.UpdateQuestion(q.ID, func(x *store.Question) { x.Status = "NoRecording" })
			c.Store.Emit(store.EvMeetingNoRecording, c.User.ID, m.ID, map[string]string{"question": q.ID})
			return out, nil
		}
		fx := fixtureFor(c, q)
		title, body, url := q.Title, "", "https://vks.mock.local/rec/"+m.ID
		if fx != nil {
			title = fx.ExpectedSummary.DraftTitle
			body = fx.ExpectedSummary.DraftBody
			if fx.Recording != nil {
				url = fx.Recording.URL
			}
			body += "\n\nОтвет на встрече: " + strings.Join(fx.ExpectedSummary.AnswerTimecodes, ", ") + "."
		} else {
			body = "Встреча записана.\nРасшифровки этой записи в демо нет — допиши ответ своими словами перед публикацией."
		}
		folder, similar := placePage(c, title)
		page := mockdata.Page{ID: nextPageID(c), Space: c.Store.Data.Team.Department.KBSpaceID, Folder: folder, Type: "meeting", Status: "Draft",
			Title: title, AuthorID: c.User.ID, UpdatedAt: c.Store.Today.Format(time.DateOnly), Body: body, SourceQuestion: q.ID, SimilarPageID: similar}
		c.Store.AddPage(page)
		out, _ := c.Store.UpdateMeeting(m.ID, func(x *store.Meeting) {
			x.Status, x.RecordingURL, x.PageID = "recorded", url, page.ID
			if fx != nil {
				x.FixtureID = fx.ID
			}
		})
		c.Store.UpdateQuestion(q.ID, func(x *store.Question) {
			x.Status, x.Answer, x.AnsweredBy, x.AnsweredAt, x.PageID = "Answered", body, c.User.ID, time.Now().Format(time.RFC3339), page.ID
		})
		c.Store.Emit(store.EvQuestionAnswered, c.User.ID, q.ID, map[string]string{"asker": q.AskedBy, "page": page.ID, "via": "meeting"})
		return out, nil
	})
}

// LeaveOpen — адресат отказался отвечать текстом после встречи без записи: вопрос «Открыт» у владельца базы (BR-37).
func LeaveOpen(c Ctx, questionID string) (store.Question, *Error) {
	return atomic(c, func(c Ctx) (store.Question, *Error) {
		q, e := questionFor(c, questionID)
		if e != nil {
			return q, e
		}
		if q.Status != "NoRecording" {
			return q, &Error{Code: "validation", Message: "Оставить открытым можно вопрос после встречи без записи."}
		}
		out, _ := c.Store.UpdateQuestion(q.ID, func(x *store.Question) { x.Status = "Open" })
		c.Store.Emit(store.EvQuestionOpen, c.User.ID, q.ID, map[string]string{"asker": q.AskedBy, "owner": c.Store.Data.Team.Department.KBOwnerID})
		return out, nil
	})
}
