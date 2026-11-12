package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/practice"
)

// Stub — заглушка модели без внешнего API (docs/04-mock-data/llm-stub.json):
//
//	stub_ok      — простые правила вместо модели: вызывает те же инструменты и собирает ответ;
//	stub_slow    — то же, но с задержкой (LLM_STUB_SLOW_SEC, по умолчанию 15 с);
//	stub_timeout — не отвечает, пока не истечёт таймаут запроса;
//	stub_error   — сразу ошибка провайдера (как HTTP 500).
//
// Нужна для тестов и CI без ключа, демо без интернета и проверки экспертами без трат.
type Stub struct {
	mode  string
	today time.Time
}

var ErrStub = errors.New("stub_error: провайдер вернул HTTP 500")

func NewStub(mode string, today time.Time) *Stub { return &Stub{mode: mode, today: today} }

func (s *Stub) Start(_, context string, tools []ToolSpec) Session {
	allowed := map[string]bool{}
	for _, t := range tools {
		allowed[t.Name] = true
	}
	session := &stubSession{stub: s, allowed: allowed}
	if allowed["practice_generate"] || allowed["practice_review"] {
		session.practiceInput = &practice.ModelInput{}
		_ = json.Unmarshal([]byte(context), session.practiceInput)
	}
	return session
}

type stubSession struct {
	practiceInput     *practice.ModelInput
	stub              *Stub
	allowed           map[string]bool
	question          string
	seq               int
	accessSystem      string // последний разрешённый доступ, чтобы понять «сделай за меня»
	accessCreate      bool
	accessExplanation string
	wantDay           bool // спросили дату отгула — следующий ответ с датой продолжает оформление
}

func (s *stubSession) wait(ctx context.Context) error {
	switch s.stub.mode {
	case "stub_error":
		return ErrStub
	case "stub_timeout":
		<-ctx.Done()
		return ctx.Err()
	case "stub_slow":
		sec, err := strconv.Atoi(os.Getenv("LLM_STUB_SLOW_SEC"))
		if err != nil {
			sec = 15
		}
		select {
		case <-time.After(time.Duration(sec) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *stubSession) call(name string, input map[string]any) ToolCall {
	s.seq++
	return ToolCall{ID: fmt.Sprintf("stub_%d", s.seq), Name: name, Input: input}
}

var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

var weekdays = map[string]time.Weekday{"понедельник": time.Monday, "вторник": time.Tuesday, "сред": time.Wednesday,
	"четверг": time.Thursday, "пятниц": time.Friday}

// ParseDay — дата из фразы: ISO, «завтра», «в пятницу» (ближайшая следующая).
func ParseDay(text string, today time.Time) string {
	if m := isoDate.FindString(text); m != "" {
		return m
	}
	low := strings.ToLower(text)
	if strings.Contains(low, "завтра") {
		return today.AddDate(0, 0, 1).Format(time.DateOnly)
	}
	for stem, wd := range weekdays {
		if strings.Contains(low, stem) {
			delta := (int(wd) - int(today.Weekday()) + 7) % 7
			if delta == 0 {
				delta = 7
			}
			return today.AddDate(0, 0, delta).Format(time.DateOnly)
		}
	}
	return ""
}

var systemKeywords = []struct{ word, system string }{
	{"персональн", "sys-dwh-pii"}, {"пдн", "sys-dwh-pii"}, {"redash", "sys-redash"}, {"витрин", "sys-dwh-marts-ro"},
	{"dwh", "sys-dwh-marts-ro"}, {"a/b", "sys-ab-platform-ro"}, {"база знаний", "sys-kb-analytics"},
}

// Только сценарные фразы заглушки. Реальная модель выбирает инструмент по system.md.
var accessIntent = regexp.MustCompile(`доступ|нет прав|как (туда )?попасть|как (подключиться|войти|зайти)`)
var accessOrder = regexp.MustCompile(`(^|\s)(оформи|запроси|подай|создай)(\s|$)|сделай за меня|сделаю за тебя|нужен доступ`)
var accessNoOrder = regexp.MustCompile(`(^|\s)не (оформ|запраш|созда|пода)|сделай сам|оформлю сам|самостоятельно|хочу сам|только (объясни|расскажи)|как (его |туда )?(оформить|получить|попасть)|кто согласует|с какой недели|когда`)
var accessChoice = regexp.MustCompile(`^(сделай за меня|сделаю за тебя|оформлю сам|сделай сам|самостоятельно|оформи заявку|подай заявку|создай заявку|оформи доступ|запроси доступ|нужен доступ|как получить доступ)[.!?]*$`)

func (s *stubSession) SendUser(ctx context.Context, text string) (Step, error) {
	if err := s.wait(ctx); err != nil {
		return Step{}, err
	}
	if s.practiceInput != nil {
		if s.allowed["practice_generate"] {
			return Step{ToolCalls: []ToolCall{s.call("practice_generate", map[string]any{"text": s.practiceInput.Task.Text})}}, nil
		}
		return Step{ToolCalls: []ToolCall{s.call("practice_review", stubPracticeReview(*s.practiceInput))}}, nil
	}
	s.question = text
	low := strings.ToLower(strings.TrimSpace(text))
	if s.wantDay && ParseDay(text, s.stub.today) != "" && !strings.Contains(low, "отгул") {
		low = "отгул " + low // «в пятницу» после «На какую дату нужен отгул?»
	}
	s.wantDay = false
	s.accessExplanation = ""
	s.accessCreate = accessOrder.MatchString(low) && !accessNoOrder.MatchString(low)
	previousSystem := s.accessSystem
	s.accessSystem = ""
	var calls []ToolCall
	switch {
	case strings.Contains(low, "сразу к задаче"):
		calls = append(calls, s.call("advance_practice", map[string]any{"action": "skip_all"}))
	case previousSystem != "" && accessChoice.MatchString(low):
		calls = append(calls, s.call("get_access_rules", map[string]any{"system": previousSystem}))
	case strings.HasPrefix(low, "не помогло"): // BR-03: ответ из базы не помог → сразу ищем человека
		calls = append(calls, s.call("find_expert", map[string]any{"question": text}))
	case strings.Contains(low, "отгул"):
		day := ParseDay(text, s.stub.today)
		if day == "" && s.allowed["create_absence_request"] {
			s.wantDay = true
			return Step{Text: "На какую дату нужен отгул? Например: «в пятницу» или «2026-11-20»."}, nil
		}
		if day == "" {
			return Step{Text: "На какую дату нужен отгул?"}, nil
		}
		calls = append(calls, s.call("get_absence_rules", map[string]any{"type": "day_off"}),
			s.call("precheck_absence", map[string]any{"type": "day_off", "date": day}),
			s.call("create_absence_request", map[string]any{"type": "day_off", "date": day}))
	case accessIntent.MatchString(low):
		for _, k := range systemKeywords {
			if strings.Contains(low, k.word) {
				calls = append(calls, s.call("get_access_rules", map[string]any{"system": k.system}))
				break
			}
		}
		if calls == nil {
			calls = append(calls, s.call("get_required_accesses", map[string]any{}))
		}
	case strings.Contains(low, "практик") || strings.Contains(low, "дашборд конверсии") || strings.Contains(low, "первая задача"):
		calls = append(calls, s.call("get_practice", map[string]any{}))
	case regexp.MustCompile(`сегодня|что делать|чем заняться`).MatchString(low):
		calls = append(calls, s.call("get_today", map[string]any{}))
	case regexp.MustCompile(`план|прогресс|где я`).MatchString(low):
		calls = append(calls, s.call("get_plan", map[string]any{}))
	case regexp.MustCompile(`кто есть кто|кто в команде|кто за что`).MatchString(low):
		calls = append(calls, s.call("team_directory", map[string]any{}))
	default:
		calls = append(calls, s.call("search_kb", map[string]any{"query": text}))
	}
	return s.filter(calls), nil
}

func (s *stubSession) filter(calls []ToolCall) Step {
	var out []ToolCall
	for _, c := range calls {
		if s.allowed[c.Name] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return Step{Text: "Это действие тебе недоступно."}
	}
	return Step{ToolCalls: out}
}

type stubResult struct {
	OK     bool            `json:"ok"`
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  *struct {
		Message string `json:"message"`
		WhoCan  string `json:"who_can"`
	} `json:"error"`
}

func (s *stubSession) SendToolResults(ctx context.Context, results []ToolResult) (Step, error) {
	if err := s.wait(ctx); err != nil {
		return Step{}, err
	}
	var parts []string
	for _, r := range results {
		var res stubResult
		_ = json.Unmarshal([]byte(r.Content), &res)
		if !res.OK && res.Error != nil {
			msg := res.Error.Message
			if res.Error.WhoCan != "" {
				msg += " Контакт: " + res.Error.WhoCan + "."
			}
			parts = append(parts, msg)
			continue
		}
		if res.Status == "needs_confirmation" {
			if s.accessExplanation != "" {
				parts = append(parts, s.accessExplanation)
				s.accessExplanation = ""
			}
			s.accessSystem = ""
			parts = append(parts, "Подготовил черновик — проверь и подтверди, тогда отправлю.")
			continue
		}
		text, next := s.describe(r.Call.Name, res.Data)
		if next != nil {
			return s.filter([]ToolCall{*next}), nil
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "Готово.")
	}
	return Step{Text: strings.Join(parts, "\n")}, nil
}

// describe — текст по результату инструмента; next — следующий вызов (поиск не дал ответа → ищем человека).
func (s *stubSession) describe(tool string, raw json.RawMessage) (string, *ToolCall) {
	switch tool {
	case "get_practice", "advance_practice":
		var v struct {
			Offer     string   `json:"offer"`
			Stage     string   `json:"stage"`
			Checklist []string `json:"checklist"`
		}
		_ = json.Unmarshal(raw, &v)
		if v.Stage == "real_task" {
			return "К реальной задаче:\n• " + strings.Join(v.Checklist, "\n• "), nil
		}
		return v.Offer + " Открой блок «Практика перед задачей» в «Мой день» или «Мой план»: там термины, учебное задание и разбор попыток. Задание появится после одобрения бадди.", nil
	case "get_access_rules":
		var d struct {
			Week     int                   `json:"week"`
			Approver struct{ Name string } `json:"approver"`
			Systems  []struct {
				System   string                  `json:"system"`
				Title    string                  `json:"title"`
				Status   string                  `json:"status"`
				WeekFrom int                     `json:"week_from"`
				Step     *struct{ Title string } `json:"step"`
			} `json:"systems"`
			SelfService string `json:"self_service_in_app"`
			Label       string `json:"label"`
		}
		_ = json.Unmarshal(raw, &d)
		var lines []string
		for _, sys := range d.Systems {
			line := fmt.Sprintf("«%s»: по плану со %d-й недели; сейчас у тебя %d-я неделя. Согласует %s.", sys.Title, sys.WeekFrom, d.Week, d.Approver.Name)
			if sys.Step != nil {
				line += " Шаг плана: «" + sys.Step.Title + "»."
			}
			switch sys.Status {
			case "granted":
				line += " Доступ уже выдан. Если система не открывается, обсуди проблему с бадди."
			case "pending":
				line += " Заявка уже на согласовании; повторная не нужна."
			default:
				line += " Можешь выбрать «сделай сам» — " + d.SelfService + " → вкладка «Доступ», выбери систему — или «сделаю за тебя»."
				if sys.WeekFrom > d.Week {
					line += " Можно подать заявку заранее, но руководитель может отклонить её как преждевременную."
				}
				if len(d.Systems) == 1 {
					s.accessSystem = sys.System
				}
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			return "Разрешённых для запроса систем не нашёл. Обсуди задачу с бадди.", nil
		}
		text := strings.Join(lines, "\n") + "\nИсточник: каталог сервиса доступов · " + d.Label + "."
		if s.accessCreate && s.accessSystem != "" {
			s.accessExplanation = text
			call := s.call("request_access", map[string]any{"system": s.accessSystem, "reason": "Доступ по плану адаптации"})
			return "", &call
		}
		return text, nil
	case "search_kb":
		var d struct {
			Found bool `json:"found"`
			Hits  []struct {
				Title     string `json:"title"`
				Body      string `json:"body"`
				Author    string `json:"author"`
				UpdatedAt string `json:"updated_at"`
				Label     string `json:"label"`
			} `json:"hits"`
		}
		_ = json.Unmarshal(raw, &d)
		if !d.Found {
			if !s.allowed["find_expert"] {
				return "В базе ответа на это нет. Подскажут коллеги — список в «Кто есть кто».", nil
			}
			c := s.call("find_expert", map[string]any{"question": s.question})
			return "", &c
		}
		h := d.Hits[0]
		label := ""
		if h.Label != "" {
			label = " · " + h.Label
		}
		return fmt.Sprintf("%s\nИсточник: «%s», %s, %s%s", h.Body, h.Title, h.Author, h.UpdatedAt, label), nil
	case "find_expert":
		var d struct {
			AddresseeID string `json:"addressee_id"`
			Name        string `json:"name"`
			Reason      string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &d)
		if !s.allowed["send_question"] || d.AddresseeID == "" {
			return fmt.Sprintf("По этой теме лучше спросить: %s — %s.", d.Name, d.Reason), nil
		}
		c := s.call("send_question", map[string]any{"text": s.question, "addressee_id": d.AddresseeID, "reason": d.Reason})
		return "", &c
	case "get_today":
		var d struct {
			Week  int `json:"week"`
			Steps []struct {
				Title, Status string
			} `json:"steps"`
			Events []struct{ Title, Start string } `json:"events"`
		}
		_ = json.Unmarshal(raw, &d)
		var lines []string
		for _, st := range d.Steps {
			mark := ""
			if st.Status == "Overdue" {
				mark = " (просрочен — нужна помощь?)"
			}
			lines = append(lines, "• "+st.Title+mark)
		}
		for _, e := range d.Events {
			lines = append(lines, "• "+e.Start+" — "+e.Title)
		}
		return fmt.Sprintf("Неделя %d. На сегодня:\n%s", d.Week, strings.Join(lines, "\n")), nil
	case "get_plan":
		var d struct {
			Progress int                     `json:"progress_pct"`
			Next     *struct{ Title string } `json:"next_step"`
		}
		_ = json.Unmarshal(raw, &d)
		next := "всё пройдено"
		if d.Next != nil {
			next = d.Next.Title
		}
		return fmt.Sprintf("Прогресс плана %d%%. Следующий шаг: %s.", d.Progress, next), nil
	case "precheck_absence":
		var d struct {
			Warnings []struct{ Message string } `json:"warnings"`
		}
		_ = json.Unmarshal(raw, &d)
		var ws []string
		for _, w := range d.Warnings {
			ws = append(ws, w.Message)
		}
		if len(ws) > 0 {
			return "Обрати внимание: " + strings.Join(ws, "; ") + ".", nil
		}
	case "get_absence_rules":
		var d struct {
			Rules  string                        `json:"rules"`
			Source struct{ Title, Label string } `json:"source"`
		}
		_ = json.Unmarshal(raw, &d)
		return fmt.Sprintf("%s\nИсточник: «%s» · %s. Можешь оформить сам — раздел «Заявки» → «Оформить самому» — или я сделаю за тебя.", d.Rules, d.Source.Title, d.Source.Label), nil
	case "team_directory":
		var d struct {
			People []struct{ Name, Responsibility string } `json:"people"`
		}
		_ = json.Unmarshal(raw, &d)
		var lines []string
		for _, p := range d.People {
			lines = append(lines, "• "+p.Name+" — "+p.Responsibility)
		}
		return "Кто есть кто:\n" + strings.Join(lines, "\n"), nil
	case "get_required_accesses":
		var d struct {
			Systems []struct{ Title, Status string } `json:"systems"`
		}
		_ = json.Unmarshal(raw, &d)
		var lines []string
		for _, sy := range d.Systems {
			lines = append(lines, "• "+sy.Title+": "+sy.Status)
		}
		return "Доступы на этой неделе:\n" + strings.Join(lines, "\n"), nil
	}
	return "", nil
}
