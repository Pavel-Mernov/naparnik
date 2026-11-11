// Package practice — данные учебной практики: шаблоны, состояние и критерии проверки.
package practice

type Term struct {
	Term       string   `json:"term"`
	Definition string   `json:"definition,omitempty"`
	Label      string   `json:"label,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

type Rule struct {
	Title   string   `json:"title"`
	Sources []string `json:"sources"`
}

type Task struct {
	Text             string     `json:"text"`
	Environment      string     `json:"environment"`
	Dataset          string     `json:"dataset"`
	Criteria         []string   `json:"criteria"`
	CheckTypes       []string   `json:"check_types"`
	CheckTopics      []string   `json:"check_topics"`
	CriterionSources [][]string `json:"criterion_sources"`
	Sources          []string   `json:"sources"`
}

type Template struct {
	ID        string   `json:"id"`
	StepID    string   `json:"related_step"`
	Offer     string   `json:"offer_text"`
	General   []Term   `json:"general_terms"`
	Team      []Term   `json:"team_terms"`
	Rules     []Rule   `json:"team_rules"`
	Task      Task     `json:"training_task"`
	Checklist []string `json:"real_task_checklist"`
}

type Source struct {
	ID      string `json:"page_id"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Updated string `json:"updated_at"`
	Body    string `json:"body"`
}

type Check struct {
	Criterion int      `json:"criterion"`
	Passed    bool     `json:"passed"`
	Feedback  string   `json:"feedback"`
	Sources   []string `json:"sources,omitempty"`
}

type Review struct {
	Checks []Check `json:"checks"`
	Passed bool    `json:"passed"`
	Score  int     `json:"score"`
}

type Attempt struct {
	Number int    `json:"number"`
	Answer string `json:"answer"`
	Review Review `json:"review"`
}

type Event struct {
	Type  string `json:"type"`
	At    string `json:"at"`
	By    string `json:"by"`
	Stage string `json:"stage,omitempty"`
}

type State struct {
	InternID    string    `json:"intern_id"`
	TemplateID  string    `json:"template_id"`
	Stage       string    `json:"stage"`
	TaskStatus  string    `json:"task_status"`
	TaskText    string    `json:"task_text"`
	Version     int       `json:"version"`
	CreatedDay  string    `json:"created_day"`
	ApprovalDue string    `json:"approval_due"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	Attempts    []Attempt `json:"attempts"`
	Events      []Event   `json:"events"`
	HelpDraftID string    `json:"help_draft_id,omitempty"`
}

func Key(intern, template string) string { return intern + "/" + template }

type ModelInput struct {
	Task    Task     `json:"task"`
	Sources []Source `json:"sources"`
	Answer  string   `json:"answer,omitempty"`
}
