// Package agent — цикл AI-buddy: вопрос → модель выбирает инструменты → код проверяет права и выполняет
// (или готовит черновик) → модель отвечает. Источники и черновики отдаются интерфейсу отдельно от текста.
package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

//go:embed system.md
var SystemPrompt string

const maxToolRounds = 5

type Draft struct {
	ID     string         `json:"id"`
	UserID string         `json:"-"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
}

type Source struct {
	PageID    string `json:"page_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	UpdatedAt string `json:"updated_at"`
	Label     string `json:"label,omitempty"`
}

type Reply struct {
	Text     string   `json:"text"`
	Drafts   []Draft  `json:"drafts"`
	Sources  []Source `json:"sources"`
	Degraded bool     `json:"degraded"` // модель не ответила — показан запасной ответ (BR-70)
	Retry    bool     `json:"retry"`    // показать кнопку «Повторить», вопрос сохранён
	ToolLog  []string `json:"-"`        // для журнала событий (BR-73)
}

type Agent struct {
	Store    *store.Store
	Provider llm.Provider
	Timeout  time.Duration

	mu       sync.Mutex
	drafts   map[string]Draft
	sessions map[string]*chat // история разговора по логину: «сделай за меня» помнит, о чём шла речь
}

// chat — разговор пользователя с моделью. Ограничен по числу реплик, чтобы контекст не рос бесконечно.
type chat struct {
	session llm.Session
	turns   int
}

const maxTurns = 20

func New(s *store.Store, p llm.Provider, timeout time.Duration) *Agent {
	return &Agent{Store: s, Provider: p, Timeout: timeout, drafts: map[string]Draft{}, sessions: map[string]*chat{}}
}

func (a *Agent) contextFor(u *mockdata.User) string {
	s := a.Store
	lines := []string{
		"## Контекст",
		fmt.Sprintf("Сегодня: %s (%s).", s.Today.Format(time.DateOnly), weekdayRU[s.Today.Weekday()]),
		fmt.Sprintf("Пользователь: %s (id %s), %s, роли: %s.", u.Name, u.ID, u.Position, strings.Join(u.Roles, ", ")),
		"Отдел: " + s.Data.Team.Department.Name + ".",
	}
	if store.HasRole(u, store.RoleIntern) {
		lines = append(lines, fmt.Sprintf("Неделя стажировки: %d.", s.Week(u)))
		lines = append(lines, fmt.Sprintf("Бадди: %s (id %s). Руководитель: %s (id %s).", s.Name(u.BuddyID), u.BuddyID, s.Name(u.LeadID), u.LeadID))
	}
	return strings.Join(lines, "\n")
}

var weekdayRU = map[time.Weekday]string{time.Monday: "понедельник", time.Tuesday: "вторник", time.Wednesday: "среда",
	time.Thursday: "четверг", time.Friday: "пятница", time.Saturday: "суббота", time.Sunday: "воскресенье"}

// Ask обрабатывает сообщение пользователя.
func (a *Agent) Ask(ctx context.Context, login, text string) (Reply, error) {
	u, found := a.Store.UserByLogin(login)
	if !found {
		return Reply{}, fmt.Errorf("нет пользователя %q", login)
	}
	if answer, blocked := guard(text); blocked {
		return Reply{Text: answer, Drafts: []Draft{}, Sources: []Source{}}, nil
	}
	tc := tools.Ctx{Store: a.Store, User: u}
	var specs []llm.ToolSpec
	for _, t := range tools.For(u) {
		specs = append(specs, llm.ToolSpec{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	reply := Reply{Drafts: []Draft{}, Sources: []Source{}}
	a.mu.Lock()
	c := a.sessions[login]
	if c == nil || c.turns >= maxTurns {
		c = &chat{session: a.Provider.Start(SystemPrompt, a.contextFor(u), specs)}
		a.sessions[login] = c
	}
	c.turns++
	a.mu.Unlock()
	session := c.session
	step, err := session.SendUser(ctx, text)
	for round := 0; err == nil && len(step.ToolCalls) > 0; round++ {
		if round == maxToolRounds {
			step = llm.Step{Text: "Не получилось довести запрос до конца. Попробуй сформулировать иначе."}
			break
		}
		results := make([]llm.ToolResult, 0, len(step.ToolCalls))
		for _, call := range step.ToolCalls {
			results = append(results, llm.ToolResult{Call: call, Content: a.handle(tc, call, &reply)})
		}
		step, err = session.SendToolResults(ctx, results)
	}
	if err != nil {
		// История могла оборваться посреди вызова инструментов — начинаем разговор заново.
		a.Reset(login)
		return a.fallback(tc, text, err), nil
	}
	reply.Text = step.Text
	if step.Refused {
		reply.Text = "С этим запросом помочь не могу."
	}
	return reply, nil
}

func (a *Agent) handle(tc tools.Ctx, call llm.ToolCall, reply *Reply) string {
	reply.ToolLog = append(reply.ToolLog, call.Name)
	t, known := tools.ByName(call.Name)
	var res any
	if known && t.Confirm && tools.Allowed(t, tc.User) {
		if e := tools.Check(tc, call.Name, call.Input); e != nil {
			raw, _ := json.Marshal(tools.Result{Error: e})
			return string(raw)
		}
		d := Draft{ID: a.Store.NextID("DRAFT"), UserID: tc.User.ID, Tool: call.Name, Args: call.Input}
		a.mu.Lock()
		a.drafts[d.ID] = d
		a.mu.Unlock()
		reply.Drafts = append(reply.Drafts, d)
		res = map[string]any{"ok": true, "status": "needs_confirmation", "draft_id": d.ID, "draft": call.Input}
	} else {
		r := tools.Run(tc, call.Name, call.Input)
		res = r
		if call.Name == "search_kb" && r.OK {
			for _, h := range r.Data.(map[string]any)["hits"].([]tools.Hit) {
				reply.Sources = append(reply.Sources, Source{h.PageID, h.Title, h.Author, h.UpdatedAt, h.Label})
			}
		}
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}

// fallback — BR-70 и режим stub_error: модель недоступна → найденные фрагменты базы без пересказа.
func (a *Agent) fallback(tc tools.Ctx, text string, err error) Reply {
	reply := Reply{Drafts: []Draft{}, Sources: []Source{}, Degraded: true, Retry: true}
	reason := "Модель сейчас недоступна."
	if ctxErr := context.DeadlineExceeded; strings.Contains(err.Error(), ctxErr.Error()) {
		reason = "Модель не ответила вовремя."
	}
	hits := tools.SearchKB(tc, text)
	if len(hits) == 0 {
		reply.Text = reason + " Твой вопрос сохранён — нажми «Повторить». План и заявки работают без модели."
		return reply
	}
	reply.Text = reason + " Вот что нашёл в базе без пересказа:\n" + hits[0].Body
	for _, h := range hits {
		reply.Sources = append(reply.Sources, Source{h.PageID, h.Title, h.Author, h.UpdatedAt, h.Label})
	}
	return reply
}

// Reset — начать разговор заново (смена учётки, ошибка модели, кнопка «Новый разговор»).
func (a *Agent) Reset(login string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, login)
}

// Confirm — человек нажал «Подтвердить»: выполнить черновик и вернуть квитанцию (BR-43).
// Если сервис недоступен, черновик сохраняется — можно повторить (SC-15).
func (a *Agent) Confirm(login, draftID string) tools.Result {
	u, found := a.Store.UserByLogin(login)
	a.mu.Lock()
	d, ok := a.drafts[draftID]
	a.mu.Unlock()
	if !found || !ok || d.UserID != u.ID {
		return tools.Result{Error: &tools.Error{Code: "not_found", Message: "Черновик не найден."}}
	}
	res := tools.Run(tools.Ctx{Store: a.Store, User: u}, d.Tool, d.Args)
	if res.Error == nil || res.Error.Code != "unavailable" {
		a.mu.Lock()
		delete(a.drafts, draftID)
		a.mu.Unlock()
	}
	return res
}

// Draft возвращает черновик (для интерфейса).
func (a *Agent) Draft(id string) (Draft, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.drafts[id]
	return d, ok
}
