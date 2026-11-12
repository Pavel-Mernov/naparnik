// Package store — состояние прототипа: мок-данные плюс то, что меняется во время демо
// (статусы шагов, заявки, вопросы). PostgreSQL сохраняет изменения транзакционно; тесты работают в памяти.
package store

import (
	"database/sql"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/practice"
)

const (
	RoleIntern = "intern"
	RoleBuddy  = "buddy"
	RoleExpert = "expert"
	RoleLead   = "lead"
)

// Request — заявка в имитации корпоративного сервиса (в стиле Jira): номер, статус, согласующий, история.
// Статусы: pending → approved (отгул) | granted (доступ) | rejected с кодом причины.
type Request struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"` // absence | access
	Type       string  `json:"type,omitempty"`
	Date       string  `json:"date,omitempty"`
	System     string  `json:"system,omitempty"`
	Title      string  `json:"title,omitempty"` // название системы — для ленты руководителя
	Comment    string  `json:"comment,omitempty"`
	EmployeeID string  `json:"employee_id"`
	ApproverID string  `json:"approver_id"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	Source     string  `json:"source"`                // agent — «сделаю за тебя», self — «сделай сам» (BR-42)
	StepID     string  `json:"step_id,omitempty"`     // шаг плана, ради которого заявка (SC-27)
	ReasonCode string  `json:"reason_code,omitempty"` // код причины отказа
	Decision   string  `json:"decision,omitempty"`    // комментарий согласующего
	DecidedAt  string  `json:"decided_at,omitempty"`
	History    []Event `json:"history"`
}

// Event — строка в истории заявки, как журнал активности в трекере.
type Event struct {
	At   string `json:"at"`
	By   string `json:"by"`
	Text string `json:"text"`
}

type Question struct {
	ID          string `json:"id"`
	AskedBy     string `json:"asked_by"`
	AddresseeID string `json:"addressee_id"`
	Title       string `json:"title"` // суть одной строкой: заголовок страницы базы и строка в списках
	Text        string `json:"text"`  // полный сформулированный вопрос (BR-14)
	Reason      string `json:"reason"`
	Urgent      bool   `json:"urgent"`
	Status      string `json:"status"` // Sent | MeetingScheduled | NoRecording | Answered | Open (BR-37)
	Answer      string `json:"answer,omitempty"`
	AnsweredBy  string `json:"answered_by,omitempty"`
	AnsweredAt  string `json:"answered_at,omitempty"`
	PageID      string `json:"page_id,omitempty"` // черновик или страница базы из ответа (P2)
	// Hint — подсказка только адресату (BR-10a): «по теме больше всех писал …». Стажёру не показывается.
	Hint      string `json:"hint,omitempty"`
	MeetingID string `json:"meeting_id,omitempty"` // встреча по вопросу (US-14)
	// Route — смены адресата: «не ко мне» (BR-17), переадресация бадди (BR-16), назначение владельцем базы (BR-37).
	Route []RouteStep `json:"route,omitempty"`
}

type RouteStep struct {
	At   string `json:"at"`
	From string `json:"from"`
	To   string `json:"to"`
	By   string `json:"by"`
	Kind string `json:"kind"` // not_mine | redirect | assign
}

type Store struct {
	mu            sync.Mutex
	db            *sql.DB
	inTransaction bool
	pages         []mockdata.Page
	drafts        map[string]ActionDraft
	operations    map[string]Operation
	practices     map[string]practice.State
	Data          *mockdata.Data
	Today         time.Time
	steps         map[string]map[string]*mockdata.TrackerStep // intern -> step -> статус в трекере
	requests      []*Request
	questions     []*Question
	seq           map[string]int
	events        []LogEvent
	notifications []Notification
	tgLinks       map[string]int64
	meetings      []*Meeting
	kbDecisions   map[string]string      // поддержка базы: решения по дублям и созданные из «посева» вопросы
	plans         map[string]*PlanState  // жизненный цикл плана адаптации (BR-81)
	profiles      map[string]ProfileEdit // правки профиля: телефон, фото
}

func New(d *mockdata.Data, today time.Time) *Store {
	s := &Store{Data: d, Today: today, practices: map[string]practice.State{}, steps: map[string]map[string]*mockdata.TrackerStep{},
		pages: append([]mockdata.Page(nil), d.KB.Pages...), drafts: map[string]ActionDraft{}, operations: map[string]Operation{}, tgLinks: map[string]int64{}, kbDecisions: map[string]string{}, plans: map[string]*PlanState{}, profiles: map[string]ProfileEdit{},
		seq: map[string]int{"ABS": 1041, "ACC": 2001, "MTG": 302, "Q": 100, "DRAFT": 0, "kb": 19}}
	for _, p := range d.Tracker.Plans {
		m := map[string]*mockdata.TrackerStep{}
		for i := range p.Steps {
			st := p.Steps[i]
			m[st.StepID] = &st
		}
		s.steps[p.InternID] = m
		s.plans[p.InternID] = s.initialPlan(p)
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

// UserBuddy — бадди стажёра (пусто, если нет).
func (s *Store) UserBuddy(id string) string {
	if u, ok := s.User(id); ok {
		return u.BuddyID
	}
	return ""
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

// UpdateRequest применяет изменение к заявке под блокировкой.
func (s *Store) UpdateRequest(id string, fn func(*Request)) (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.requests {
		if r.ID == id {
			fn(r)
			out := *r
			out.History = slices.Clone(r.History)
			return out, true
		}
	}
	return Request{}, false
}

func (s *Store) Requests(employeeID string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Request
	for _, r := range s.requests {
		if employeeID == "" || r.EmployeeID == employeeID {
			c := *r
			c.History = slices.Clone(r.History)
			out = append(out, c)
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
			q.Answer = s.publicMeetingText(q.Answer)
			return *q, true
		}
	}
	return Question{}, false
}

// Pages — копия списка страниц базы (опубликованные и черновики).
func (s *Store) Pages() []mockdata.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mockdata.Page(nil), s.pages...)
}

func (s *Store) AddPage(p mockdata.Page) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Body = s.publicMeetingText(p.Body)
	s.pages = append(s.pages, p)
}

// UpdatePage применяет изменение к странице под блокировкой.
func (s *Store) UpdatePage(id string, fn func(*mockdata.Page)) (mockdata.Page, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.pages {
		if s.pages[i].ID == id {
			fn(&s.pages[i])
			s.pages[i].Body = s.publicMeetingText(s.pages[i].Body)
			return s.pages[i], true
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
