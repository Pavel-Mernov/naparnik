// Package mockdata читает синтетические данные команды из docs/04-mock-data.
//
// Источник правды — JSON-файлы команды; код их не дублирует. Поля, которые бэкенду пока не нужны,
// просто не описаны в структурах и игнорируются при разборе.
package mockdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	Proactivity              string `json:"proactivity"`
	QuestionTextVisibleBuddy bool   `json:"question_text_visible_to_buddy"`
}

type User struct {
	ID             string    `json:"id"`
	Login          string    `json:"login"`
	Name           string    `json:"name"`
	Position       string    `json:"position"`
	Roles          []string  `json:"roles"`
	Responsibility string    `json:"responsibility"`
	WorkingHours   string    `json:"working_hours"`
	BuddyFor       []string  `json:"buddy_for"`
	BuddyID        string    `json:"buddy_id"`
	LeadID         string    `json:"lead_id"`
	StartDate      string    `json:"start_date"`
	Settings       *Settings `json:"settings"`
	TGConnected    bool      `json:"tg_connected"`
}

type Department struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	KBSpaceID string `json:"kb_space_id"`
	LeadID    string `json:"lead_id"`
}

type Team struct {
	Department Department `json:"department"`
	Users      []User     `json:"users"`
}

type Space struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DepartmentID string `json:"department_id"`
}

type Page struct {
	ID              string `json:"id"`
	Space           string `json:"space"`
	Folder          string `json:"folder"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	Title           string `json:"title"`
	AuthorID        string `json:"author_id"`
	UpdatedAt       string `json:"updated_at"`
	TestInstruction bool   `json:"test_instruction"`
	Body            string `json:"body"`
}

type KB struct {
	Spaces []Space `json:"spaces"`
	Pages  []Page  `json:"pages"`
}

type Stage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Order int    `json:"order"`
}

type TemplateStep struct {
	ID             string `json:"id"`
	Stage          string `json:"stage"`
	Owner          string `json:"owner"` // intern | buddy | lead
	Type           string `json:"type"`
	Title          string `json:"title"`
	Source         string `json:"source"`
	PreparedAction string `json:"prepared_action"`
}

type PlanTemplate struct {
	Stages []Stage        `json:"stages"`
	Steps  []TemplateStep `json:"steps"`
}

type TrackerStep struct {
	StepID    string `json:"step_id"`
	IssueKey  string `json:"issue_key"`
	DueDate   string `json:"due_date"`
	Status    string `json:"status"` // Planned | Today | Done | Overdue
	ReceiptID string `json:"receipt_id"`
}

type Plan struct {
	ID       string        `json:"id"`
	EpicKey  string        `json:"epic_key"`
	InternID string        `json:"intern_id"`
	Status   string        `json:"status"`
	Steps    []TrackerStep `json:"steps"`
}

type Tracker struct {
	Plans []Plan `json:"plans"`
}

type Event struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Participants []string `json:"participants"`
	RelatedStep  string   `json:"related_step"`
}

type Calendar struct {
	Events []Event `json:"events"`
}

type RequestType struct {
	Type              string `json:"type"`
	Title             string `json:"title"`
	MinNoticeWorkdays int    `json:"min_notice_workdays"`
	MinNoticeDays     int    `json:"min_notice_days"`
}

type SelfService struct {
	Title      string   `json:"title"`
	Steps      []string `json:"steps"`
	Link       string   `json:"link"`
	SourcePage string   `json:"source_page"`
}

type AbsenceService struct {
	RequestTypes    []RequestType `json:"request_types"`
	ForbiddenFields []string      `json:"forbidden_fields"`
	SelfService     SelfService   `json:"self_service"`
}

type System struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	WeekFrom            *int   `json:"week_from"`
	Department          string `json:"department"`
	RequestableByIntern *bool  `json:"requestable_by_intern"`
}

type AccessState struct {
	System    string `json:"system"`
	Status    string `json:"status"`
	ReceiptID string `json:"receipt_id"`
}

type AccessService struct {
	Systems      []System                 `json:"systems"`
	CurrentState map[string][]AccessState `json:"current_state"`
}

type Data struct {
	DemoToday string
	Team      Team
	KB        KB
	Template  PlanTemplate
	Tracker   Tracker
	Calendar  Calendar
	Absence   AbsenceService
	Access    AccessService
}

func readJSON(dir, name string, v any) error {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("мок-данные %s: %w", name, err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("мок-данные %s: %w", name, err)
	}
	return nil
}

// Load читает все файлы мок-данных из каталога dir.
func Load(dir string) (*Data, error) {
	d := &Data{}
	var meta struct {
		Meta struct {
			DemoToday string `json:"demo_today"`
		} `json:"_meta"`
	}
	files := []struct {
		name string
		v    any
	}{
		{"team.json", &d.Team},
		{"kb-pages.json", &d.KB},
		{"adaptation-plan-template.json", &d.Template},
		{"tracker.json", &d.Tracker},
		{"calendar.json", &d.Calendar},
		{"absence-service.json", &d.Absence},
		{"access-service.json", &d.Access},
		{"team.json", &meta},
	}
	for _, f := range files {
		if err := readJSON(dir, f.name, f.v); err != nil {
			return nil, err
		}
	}
	d.DemoToday = meta.Meta.DemoToday
	return d, nil
}
