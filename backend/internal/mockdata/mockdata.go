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

	"github.com/worker4556/naparnik/backend/internal/practice"
)

type Settings struct {
	Proactivity              string   `json:"proactivity"`
	QuestionTextVisibleBuddy bool     `json:"question_text_visible_to_buddy"`
	Reminders                []string `json:"reminders"` // за сколько напоминать о событии: 1d, 1h (BR-84)
}

type User struct {
	ID              string    `json:"id"`
	Login           string    `json:"login"`
	Name            string    `json:"name"`
	Position        string    `json:"position"`
	Roles           []string  `json:"roles"`
	Responsibility  string    `json:"responsibility"`
	ManagementLevel string    `json:"management_level"` // none | department_lead | head_of_heads | director (BR-10a, атрибут ABAC)
	WorkingHours    string    `json:"working_hours"`
	BuddyFor        []string  `json:"buddy_for"`
	BuddyID         string    `json:"buddy_id"`
	LeadID          string    `json:"lead_id"`
	StartDate       string    `json:"start_date"`
	Settings        *Settings `json:"settings"`
	TGConnected     bool      `json:"tg_connected"`
}

type Department struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	KBSpaceID string  `json:"kb_space_id"`
	LeadID    string  `json:"lead_id"`
	KBOwnerID string  `json:"kb_owner_id"` // владелец базы (BR-34): получает открытые вопросы (BR-37)
	Routing   Routing `json:"routing"`
}

// Routing — настройки маршрутизации отдела (BR-10a): уровни руководства, которые не предлагаются стажёру как эксперты.
type Routing struct {
	SeniorManagementLevels []string `json:"senior_management_levels"`
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
	SourceQuestion  string `json:"source_question,omitempty"` // для страниц «Вопрос-ответ» из ответа человека (P2)
	SimilarPageID   string `json:"similar_page_id,omitempty"` // BR-32: похожая страница, которую можно дополнить
	MergedInto      string `json:"merged_into,omitempty"`     // BR-39: после объединения дублей страница ведёт на общую
}

type Folder struct {
	ID    string `json:"id"`
	Space string `json:"space"`
	Title string `json:"title"`
}

type KB struct {
	Spaces      []Space         `json:"spaces"`
	Folders     []Folder        `json:"folders"`
	Pages       []Page          `json:"pages"`
	Maintenance MaintenanceSeed `json:"maintenance_seed"`
}

// MaintenanceSeed — исходное состояние «Поддержки базы» (SC-22): дубли, открытые вопросы, неподтверждённые черновики.
type MaintenanceSeed struct {
	DuplicatePairs []struct {
		ID            string   `json:"id"`
		Pages         []string `json:"pages"`
		ProposedTitle string   `json:"proposed_title"`
		Reason        string   `json:"reason"`
	} `json:"duplicate_pairs"`
	OpenQuestions []struct {
		ID      string `json:"id"`
		Text    string `json:"text"`
		AskedBy string `json:"asked_by"`
		Reason  string `json:"reason"`
	} `json:"open_questions"`
	UnconfirmedDrafts []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		AuthorID  string `json:"author_id"`
		CreatedAt string `json:"created_at"`
	} `json:"unconfirmed_drafts"`
	NextScheduledCheck string `json:"next_scheduled_check"`
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
	Events []Event                `json:"events"`
	Busy   map[string][]TimeRange `json:"busy"` // занятость людей сверх событий (для слотов встречи, BR-41)
}

type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// MeetingFixture — запись встречи из заглушки ВКС (meetings.json): транскрипт и ожидаемая выжимка.
// Реального распознавания речи в MVP нет: транскрипт — готовая фикстура (docs/04-mock-data/meetings.json).
type MeetingFixture struct {
	ID              string `json:"id"`
	QuestionID      string `json:"question_id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	RecordingStatus string `json:"recording_status"`
	Recording       *struct {
		URL         string `json:"url"`
		DurationSec int    `json:"duration_sec"`
	} `json:"recording"`
	TranscriptFile  string `json:"transcript_file"`
	Transcript      string `json:"-"`
	ExpectedSummary *struct {
		AnswerTimecodes []string `json:"answer_timecodes"`
		DraftTitle      string   `json:"draft_title"`
		DraftBody       string   `json:"draft_body"`
		Excluded        string   `json:"excluded"`
	} `json:"expected_summary"`
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

// ReasonCode — код причины отказа согласующего (absence-service.json, access-service.json).
type ReasonCode struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

type AbsenceService struct {
	RequestTypes    []RequestType `json:"request_types"`
	ForbiddenFields []string      `json:"forbidden_fields"`
	SelfService     SelfService   `json:"self_service"`
	ReasonCodes     []ReasonCode  `json:"reason_codes"`
}

type System struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	WeekFrom            *int   `json:"week_from"`
	Department          string `json:"department"`
	RequestableByIntern *bool  `json:"requestable_by_intern"`
}

type AccessState struct {
	System      string `json:"system"`
	Status      string `json:"status"`
	ReceiptID   string `json:"receipt_id"`
	RelatedStep string `json:"related_step"`
}

type AccessService struct {
	Systems      []System                 `json:"systems"`
	CurrentState map[string][]AccessState `json:"current_state"`
	ReasonCodes  []ReasonCode             `json:"reason_codes"`
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
	Practice  struct {
		Templates []practice.Template `json:"practices"`
	}
	Meetings struct {
		Meetings []MeetingFixture `json:"meetings"`
	}
	Questions struct {
		Questions []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"questions"`
	}
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
		{"practice.json", &d.Practice},
		{"meetings.json", &d.Meetings},
		{"questions.json", &d.Questions},
		{"team.json", &meta},
	}
	for _, f := range files {
		if err := readJSON(dir, f.name, f.v); err != nil {
			return nil, err
		}
	}
	d.DemoToday = meta.Meta.DemoToday
	for i, m := range d.Meetings.Meetings {
		if m.TranscriptFile == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, m.TranscriptFile))
		if err != nil {
			return nil, err
		}
		d.Meetings.Meetings[i].Transcript = string(raw)
	}
	return d, nil
}
