// Package store — состояние прототипа: мок-данные плюс то, что меняется во время демо
// (статусы шагов, заявки, вопросы). Живёт в памяти; следующий шаг — PostgreSQL за тем же интерфейсом.
package store

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

const (
	RoleIntern = "intern"
	RoleBuddy  = "buddy"
	RoleExpert = "expert"
	RoleLead   = "lead"
)

type Request struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"` // absence | access
	Type       string `json:"type,omitempty"`
	Date       string `json:"date,omitempty"`
	System     string `json:"system,omitempty"`
	Comment    string `json:"comment,omitempty"`
	EmployeeID string `json:"employee_id"`
	ApproverID string `json:"approver_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type Question struct {
	ID          string `json:"id"`
	AskedBy     string `json:"asked_by"`
	AddresseeID string `json:"addressee_id"`
	Title       string `json:"title"` // суть одной строкой: заголовок страницы базы и строка в списках
	Text        string `json:"text"`  // полный сформулированный вопрос (BR-14)
	Reason      string `json:"reason"`
	Urgent      bool   `json:"urgent"`
	Status      string `json:"status"` // Sent | Answered
	Answer      string `json:"answer,omitempty"`
	AnsweredBy  string `json:"answered_by,omitempty"`
	AnsweredAt  string `json:"answered_at,omitempty"`
	PageID      string `json:"page_id,omitempty"` // черновик или страница базы из ответа (P2)
}

type Store struct {
	mu        sync.Mutex
	Data      *mockdata.Data
	Today     time.Time
	steps     map[string]map[string]*mockdata.TrackerStep // intern -> step -> статус в трекере
	requests  []*Request
	questions []*Question
	seq       map[string]int
}

func New(d *mockdata.Data, today time.Time) *Store {
	s := &Store{Data: d, Today: today, steps: map[string]map[string]*mockdata.TrackerStep{},
		seq: map[string]int{"ABS": 1041, "ACC": 2001, "Q": 100, "DRAFT": 0, "kb": 19}}
	for _, p := range d.Tracker.Plans {
		m := map[string]*mockdata.TrackerStep{}
		for i := range p.Steps {
			st := p.Steps[i]
			m[st.StepID] = &st
		}
		s.steps[p.InternID] = m
	}
	return s
}

func (s *Store) NextID(prefix string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq[prefix]++
	return fmt.Sprintf("%s-%d", prefix, s.seq[prefix])
}

func (s *Store) UserByLogin(login string) (*mockdata.User, bool) {
	for i := range s.Data.Team.Users {
		if s.Data.Team.Users[i].Login == login {
			return &s.Data.Team.Users[i], true
		}
	}
	return nil, false
}

func (s *Store) User(id string) (*mockdata.User, bool) {
	for i := range s.Data.Team.Users {
		if s.Data.Team.Users[i].ID == id {
			return &s.Data.Team.Users[i], true
		}
	}
	return nil, false
}

func (s *Store) Name(id string) string {
	if u, ok := s.User(id); ok {
		return u.Name
	}
	return id
}

func HasRole(u *mockdata.User, role string) bool { return slices.Contains(u.Roles, role) }

// CanSeeIntern: сам стажёр, его бадди и руководитель (02-ROLES).
func (s *Store) CanSeeIntern(viewer *mockdata.User, internID string) bool {
	if viewer.ID == internID {
		return true
	}
	intern, ok := s.User(internID)
	if !ok {
		return false
	}
	return viewer.ID == intern.BuddyID || viewer.ID == intern.LeadID
}

// Week — неделя стажировки, начиная с 1.
func (s *Store) Week(u *mockdata.User) int {
	start, err := time.Parse(time.DateOnly, u.StartDate)
	if err != nil || s.Today.Before(start) {
		return 1
	}
	return int(s.Today.Sub(start).Hours()/24)/7 + 1
}

// ReadableSpaces — пространства базы знаний отдела пользователя (BR-02, BR-40).
func (s *Store) ReadableSpaces() map[string]bool {
	out := map[string]bool{}
	for _, sp := range s.Data.KB.Spaces {
		if sp.DepartmentID == s.Data.Team.Department.ID {
			out[sp.ID] = true
		}
	}
	return out
}

func (s *Store) TemplateStep(id string) (mockdata.TemplateStep, bool) {
	for _, st := range s.Data.Template.Steps {
		if st.ID == id {
			return st, true
		}
	}
	return mockdata.TemplateStep{}, false
}

// PlanSteps — шаги плана стажёра в порядке шаблона, со статусами из трекера.
func (s *Store) PlanSteps(internID string) []mockdata.TrackerStep {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.steps[internID]
	var out []mockdata.TrackerStep
	for _, t := range s.Data.Template.Steps {
		if st, ok := m[t.ID]; ok {
			out = append(out, *st)
		}
	}
	return out
}

func (s *Store) SetStepStatus(internID, stepID, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.steps[internID][stepID]
	if ok {
		st.Status = status
	}
	return ok
}

func (s *Store) AddRequest(r *Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r)
}

func (s *Store) Requests(employeeID string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Request
	for _, r := range s.requests {
		if employeeID == "" || r.EmployeeID == employeeID {
			out = append(out, *r)
		}
	}
	return out
}

func (s *Store) AddQuestion(q *Question) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.questions = append(s.questions, q)
}

// UpdateQuestion применяет изменение к вопросу под блокировкой.
func (s *Store) UpdateQuestion(id string, fn func(*Question)) (Question, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, q := range s.questions {
		if q.ID == id {
			fn(q)
			return *q, true
		}
	}
	return Question{}, false
}

// Pages — копия списка страниц базы (опубликованные и черновики).
func (s *Store) Pages() []mockdata.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mockdata.Page(nil), s.Data.KB.Pages...)
}

func (s *Store) AddPage(p mockdata.Page) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Data.KB.Pages = append(s.Data.KB.Pages, p)
}

// UpdatePage применяет изменение к странице под блокировкой.
func (s *Store) UpdatePage(id string, fn func(*mockdata.Page)) (mockdata.Page, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Data.KB.Pages {
		if s.Data.KB.Pages[i].ID == id {
			fn(&s.Data.KB.Pages[i])
			return s.Data.KB.Pages[i], true
		}
	}
	return mockdata.Page{}, false
}

func (s *Store) Questions() []Question {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Question, 0, len(s.questions))
	for _, q := range s.questions {
		out = append(out, *q)
	}
	return out
}
