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
	Text        string `json:"text"`
	Reason      string `json:"reason"`
	Urgent      bool   `json:"urgent"`
	Status      string `json:"status"` // Sent | Answered
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
		seq: map[string]int{"ABS": 1041, "ACC": 2001, "Q": 100, "DRAFT": 0}}
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

func (s *Store) Questions() []Question {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Question, 0, len(s.questions))
	for _, q := range s.questions {
		out = append(out, *q)
	}
	return out
}
