package store

import "slices"

// Meeting — встреча по вопросу (US-14…16): эксперт выбрал слот, после встречи — запись и выжимка или «записи нет».
// Status: scheduled → recorded (черновик страницы из выжимки) | no_recording (предложение ответить текстом, BR-37).
type Meeting struct {
	ID           string   `json:"id"`
	QuestionID   string   `json:"question_id"`
	OrganizerID  string   `json:"organizer_id"`
	Participants []string `json:"participants"`
	Title        string   `json:"title"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Status       string   `json:"status"`
	RecordingURL string   `json:"recording_url,omitempty"`
	FixtureID    string   `json:"fixture_id,omitempty"` // какая запись ВКС из заглушки (meetings.json)
	PageID       string   `json:"page_id,omitempty"`
}

func (s *Store) AddMeeting(m *Meeting) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meetings = append(s.meetings, m)
}

func (s *Store) Meetings() []Meeting {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Meeting, 0, len(s.meetings))
	for _, m := range s.meetings {
		c := *m
		c.Participants = slices.Clone(m.Participants)
		out = append(out, c)
	}
	return out
}

func (s *Store) UpdateMeeting(id string, fn func(*Meeting)) (Meeting, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.meetings {
		if m.ID == id {
			fn(m)
			return *m, true
		}
	}
	return Meeting{}, false
}

func (s *Store) KBDecision(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kbDecisions[key]
}

func (s *Store) SetKBDecision(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kbDecisions[key] = value
}
