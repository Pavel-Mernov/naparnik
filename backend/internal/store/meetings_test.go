package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// BR-36: старое состояние уже могло содержать URL записи в статье и ответе.
// При чтении оно очищается; сама запись сохраняется в закрытых данных встречи.
func TestRestoreMeetingRecordingPrivacy(t *testing.T) {
	s := newTestStore(t)
	url := "https://vks.mock.local/rec/MTG-303"
	body := "Ответ по делу.\n\nОтвет на встрече: 05:48–11:20. Запись (только участникам): " + url
	v := s.snapshot()
	v.Meetings = []*Meeting{{ID: "MTG-303", QuestionID: "Q-101", PageID: "kb-old", RecordingURL: url, Participants: []string{"u-smirnova", "u-sokolov"}}}
	v.Pages = append(v.Pages, mockdata.Page{ID: "kb-old", Status: "Published", Body: body})
	v.Questions = []*Question{{ID: "Q-101", Answer: body}}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.decode(raw); err != nil {
			t.Fatal(err)
		}
		for _, p := range s.Pages() {
			if p.ID == "kb-old" && (p.Body != "Ответ по делу.\n\nОтвет на встрече: 05:48–11:20." || strings.Contains(p.Body, url)) {
				t.Fatal(p.Body)
			}
		}
		if strings.Contains(s.Questions()[0].Answer, url) || s.Meetings()[0].RecordingURL != url {
			t.Fatal("ответ не очищен или запись потеряна")
		}
		raw, err = json.Marshal(s.snapshot())
		if err != nil {
			t.Fatal(err)
		}
	}
	// Открытый до обновления редактор не должен вернуть URL в опубликованную статью.
	p, _ := s.UpdatePage("kb-old", func(p *mockdata.Page) { p.Body = body })
	q, _ := s.UpdateQuestion("Q-101", func(q *Question) { q.Answer = body })
	s.AddPage(mockdata.Page{ID: "copied", Body: body})
	if strings.Contains(p.Body, url) || strings.Contains(q.Answer, url) || strings.Contains(s.Pages()[len(s.Pages())-1].Body, url) {
		t.Fatal("повторная запись раскрывает URL")
	}
}

func TestMeetingRecordingMetadataCleansExistingContent(t *testing.T) {
	s := newTestStore(t)
	url := "https://vks.mock.local/rec/MTG-303"
	s.AddPage(mockdata.Page{ID: "old", Body: "Встреча записана: " + url + ".\nДопиши ответ."})
	s.AddMeeting(&Meeting{ID: "MTG-303"})
	s.UpdateMeeting("MTG-303", func(m *Meeting) { m.RecordingURL = url })
	if got := s.Pages()[len(s.Pages())-1].Body; got != "Встреча записана.\nДопиши ответ." {
		t.Fatal(got)
	}
}
