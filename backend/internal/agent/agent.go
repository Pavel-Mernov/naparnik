// Package agent — цикл AI-buddy: вопрос → модель выбирает инструменты → код проверяет права и выполняет
// (или готовит черновик) → модель отвечает. Источники и черновики отдаются интерфейсу отдельно от текста.
package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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
	ID      string          `json:"id"`
	UserID  string          `json:"-"`
	Tool    string          `json:"tool"`
	Args    map[string]any  `json:"args"`
	Version int             `json:"version,omitempty"`
	Status  string          `json:"status,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
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
	// Actions — быстрые действия, когда модель недоступна: та же дорога «черновик → подтвердить» через правила (BR-70).
	Actions []QuickAction `json:"actions,omitempty"`
	ToolLog []string      `json:"-"` // для журнала событий (BR-73)
	Trace   []string      `json:"-"` // вызовы с аргументами и коротким итогом — для прогона evals

	searched, found, expertAsked bool // искал ли в базе в этом ходе, нашёл ли, подбирал ли человека

}

// QuickAction — кнопка в чате: нажатие отправляет Text как сообщение пользователя.
type QuickAction struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type Agent struct {
	Store    *store.Store
	Provider llm.Provider
	Timeout  time.Duration
	// Backup — правила вместо модели (llm.Stub stub_ok): если модель упала или молчит, вопрос не теряется,
	// а проходит по тем же инструментам и проверкам. nil — только фрагменты базы.
	Backup llm.Provider

	mu        sync.Mutex
	userLocks map[string]*sync.Mutex
	sessions  map[string]*chat // история разговора по логину: «сделай за меня» помнит, о чём шла речь
	backups   map[string]*chat // разговор с правилами, пока модель недоступна: «на какую дату?» → «в пятницу»
}

// chat — разговор пользователя с моделью. Ограничен по числу реплик, чтобы контекст не рос бесконечно.
type chat struct {
	session llm.Session
	turns   int
}

const maxTurns = 20

// backupTimeout — правилам внешний API не нужен; отдельный срок, потому что срок модели уже истёк.
const backupTimeout = 5 * time.Second

func New(s *store.Store, p llm.Provider, timeout time.Duration) *Agent {
	a := &Agent{Store: s, Provider: p, Timeout: timeout, userLocks: map[string]*sync.Mutex{}, sessions: map[string]*chat{}, backups: map[string]*chat{}}
	if s != nil {
		a.Backup = llm.NewStub("stub_ok", s.Today)
	}
	return a
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
	// Руководителю и бадди — их стажёры с id: так get_plan(intern_id) работает по имени («как дела у Даниила»).
	var interns []string
	for _, other := range s.Data.Team.Users {
		if other.ID != u.ID && store.HasRole(&other, store.RoleIntern) && (other.LeadID == u.ID || other.BuddyID == u.ID) {
			interns = append(interns, fmt.Sprintf("%s (id %s)", other.Name, other.ID))
		}
	}
	if len(interns) > 0 {
		lines = append(lines, "Твои стажёры: "+strings.Join(interns, ", ")+".")
	}
	return strings.Join(lines, "\n")
}

var weekdayRU = map[time.Weekday]string{time.Monday: "понедельник", time.Tuesday: "вторник", time.Wednesday: "среда",
	time.Thursday: "четверг", time.Friday: "пятница", time.Saturday: "суббота", time.Sunday: "воскресенье"}

func (a *Agent) userLock(login string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.userLocks[login] == nil {
		a.userLocks[login] = &sync.Mutex{}
	}
	return a.userLocks[login]
}

// Ask обрабатывает сообщение пользователя.
func (a *Agent) Ask(ctx context.Context, login, text string) (Reply, error) {
	lock := a.userLock(login)
	lock.Lock()
	defer lock.Unlock()
	if err := a.Store.Refresh(ctx); err != nil {
		return Reply{}, fmt.Errorf("хранилище недоступно")
	}
	u, found := a.Store.UserByLogin(login)
	if !found {
		return Reply{}, fmt.Errorf("нет пользователя %q", login)
	}
	people := ""
	if store.HasRole(u, store.RoleIntern) && u.BuddyID != "" {
		people = "бадди — " + a.Store.Name(u.BuddyID) + ", руководитель — " + a.Store.Name(u.LeadID)
	}
	if answer, blocked := guard(text, people); blocked {
		return Reply{Text: answer, Drafts: []Draft{}, Sources: []Source{}}, nil
	}
	tc := tools.Ctx{Store: a.Store, User: u, Question: text}
	var specs []llm.ToolSpec
	for _, t := range tools.For(u) {
		specs = append(specs, llm.ToolSpec{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	parent := ctx
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
	step, err := a.run(ctx, session, tc, &reply, text)
	// Модель иногда обещает «сейчас покажу черновик», но не вызывает инструмент — ход заканчивается
	// пустым обещанием, и стажёру нечего подтверждать. Код замечает это и один раз просит довести дело.
	if err == nil && len(reply.Drafts) == 0 && promisesDraft(step.Text) && canDraft(u) {
		step, err = a.run(ctx, session, tc, &reply, draftNudge)
	}
	if err != nil {
		// История могла оборваться посреди вызова инструментов — начинаем разговор заново.
		a.mu.Lock()
		delete(a.sessions, login)
		a.mu.Unlock()
		return a.degraded(parent, login, tc, specs, text, err), nil
	}
	a.mu.Lock()
	delete(a.backups, login) // модель снова отвечает — разговор с правилами больше не нужен
	a.mu.Unlock()
	reply.Text = honestMiss(tc, &reply, step.Text, text)
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
		var d Draft
		err := a.Store.Transaction(func(s *store.Store) error {
			d = Draft{ID: s.NextID("DRAFT"), UserID: tc.User.ID, Tool: call.Name, Args: call.Input, Version: 1, Status: "pending"}
			s.PutDraft(store.ActionDraft{ID: d.ID, UserID: d.UserID, Tool: d.Tool, Args: d.Args})
			return nil
		})
		if err != nil {
			res = tools.Result{Error: &tools.Error{Code: "unavailable", Message: "Не удалось сохранить черновик. Повтори запрос."}}
		} else {
			reply.Drafts = append(reply.Drafts, d)
			res = map[string]any{"ok": true, "status": "needs_confirmation", "draft_id": d.ID, "draft": call.Input}
		}
	} else {
		r := tools.Run(tc, call.Name, call.Input)
		res = r
		if r.OK {
			hits := tools.SourcesFor(tc, call.Name, call.Input)
			switch call.Name {
			case "search_kb":
				hits = r.Data.(map[string]any)["hits"].([]tools.Hit)
				reply.searched, reply.found = true, reply.found || len(hits) > 0
			case "find_expert":
				reply.expertAsked = true
			}
			for _, h := range hits {
				reply.addSource(Source{h.PageID, h.Title, h.Author, h.UpdatedAt, h.Label})
			}
		}
	}
	raw, _ := json.Marshal(res)
	args, _ := json.Marshal(call.Input)
	out := []rune(string(raw))
	if len(out) > 160 {
		out = append(out[:160], '…')
	}
	reply.Trace = append(reply.Trace, call.Name+" "+string(args)+" → "+string(out))
	return string(raw)
}

// addSource — источник без повторов: поиск часто вызывают дважды с разными формулировками.
func (r *Reply) addSource(src Source) {
	for _, have := range r.Sources {
		if have.PageID == src.PageID {
			return
		}
	}
	r.Sources = append(r.Sources, src)
}

// run — один ход: сообщение модели и цикл вызова инструментов до текстового ответа.
func (a *Agent) run(ctx context.Context, session llm.Session, tc tools.Ctx, reply *Reply, text string) (llm.Step, error) {
	step, err := session.SendUser(ctx, text)
	for round := 0; err == nil && len(step.ToolCalls) > 0; round++ {
		if round == maxToolRounds {
			return llm.Step{Text: "Не получилось довести запрос до конца. Попробуй сформулировать иначе."}, nil
		}
		results := make([]llm.ToolResult, 0, len(step.ToolCalls))
		for _, call := range step.ToolCalls {
			results = append(results, llm.ToolResult{Call: call, Content: a.handle(tc, call, reply)})
		}
		step, err = session.SendToolResults(ctx, results)
	}
	return step, err
}

var notFoundRe = regexp.MustCompile(`(?i)не наш(ёл|ел|лось|лись|[её]лся)|ничего не|ничего нет|пусто|нет (в базе|ответа|страниц|информац)|ответа нет|в базе[^.]{0,25}(нет|не )|не описан`)

// missRe — строгий признак «в базе ответа нет» в словах модели (без случайных «ничего не нужно»).
var missRe = regexp.MustCompile(`(?i)в базе[^.]{0,40}(нет|не нашл|не описан|ничего)|не наш(ёл|ел|лось|лись)[^.]{0,20}в базе`)

// honestMiss — BR-02, M-15: если в этом ходе база ничего не нашла, ответ честно с этого начинается, а стажёр
// узнаёт, к кому идти (SC-04), даже когда модель сразу перескочила к черновику или забыла подобрать человека.
func honestMiss(tc tools.Ctx, r *Reply, text, question string) string {
	text = testLabel(r, text)
	asksPerson := false
	for _, d := range r.Drafts {
		asksPerson = asksPerson || d.Tool == "send_question"
	}
	// Искал в базе и всё равно подбирает человека или готовит ему вопрос — значит, найденное не ответило.
	// find_expert по описанию — для вопроса, которого в базе нет. Так и говорим.
	if r.searched && (asksPerson || r.expertAsked) && !notFoundRe.MatchString(text) {
		return "В базе ответа на это нет. " + text
	}
	// Модель сама признала, что в базе ответа нет, но человека не назвала — назовём (SC-04).
	admitted := missRe.MatchString(text)
	if !admitted && (!r.searched || r.found || len(r.Sources) > 0) {
		return text
	}
	if !admitted && !notFoundRe.MatchString(text) {
		text = "В базе ответа на это нет. " + text
	}
	if !r.expertAsked && len(r.Drafts) == 0 && store.HasRole(tc.User, store.RoleIntern) {
		if pick := tools.Run(tc, "find_expert", map[string]any{"question": question}); pick.OK {
			d := pick.Data.(map[string]any)
			text += fmt.Sprintf("\n\nПо этой теме лучше спросить: %s — %s. Подготовить вопрос?", d["name"], d["reason"])
		}
	}
	return text
}

// testLabel — BR-05: ответ по тестовой инструкции всегда с пометкой, даже если модель её не повторила.
func testLabel(r *Reply, text string) string {
	low := strings.ToLower(text)
	for _, src := range r.Sources {
		// BR-49: у конфиденциальной страницы ссылка вместо пересказа — напомним, если модель не сказала.
		if src.Label == tools.ConfidentialLabel && !strings.Contains(low, "конфиденц") {
			text += "\n\nСтраница «" + src.Title + "» под грифом «конфиденциально»: её текст не передаётся модели, открой её в базе."
			low = strings.ToLower(text)
			continue
		}
		if src.Label != "" && src.Label != tools.ConfidentialLabel && !strings.Contains(low, "тестов") {
			return text + "\n\nИсточник «" + src.Title + "» — " + src.Label + "."
		}
	}
	return text
}

const draftNudge = "[Служебная заметка системы, не от стажёра] Ты написал про черновик, но не вызвал инструмент, " +
	"поэтому черновика нет. Если данных хватает — вызови нужный инструмент (create_absence_request, request_access " +
	"или send_question) прямо сейчас и коротко скажи, что черновик ждёт подтверждения. Если данных не хватает — задай один уточняющий вопрос."

var promiseRe = regexp.MustCompile(`(?i)черновик[а-я]*\s+(готов|ниже|подготов|сейчас|покажу|ждёт|ждет)|(покажу|подготовлю|оформлю|соберу|сделаю)\s+(тебе\s+)?черновик|нажми\s+(кнопку\s+)?«?подтверд`)

// promisesDraft — в ответе обещан черновик или кнопка подтверждения.
func promisesDraft(text string) bool { return promiseRe.MatchString(text) }

func canDraft(u *mockdata.User) bool {
	for _, t := range tools.For(u) {
		if t.Confirm {
			return true
		}
	}
	return false
}

func degradedReason(err error) string {
	if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		return "Модель не ответила вовремя."
	}
	return "Модель сейчас недоступна."
}

// degraded — BR-70: модель упала или молчит. Вопрос проходит по правилам через те же инструменты, права
// и черновики с подтверждением — стажёр получает тот же порядок действий. Правила не справились —
// фрагменты базы без пересказа. В обоих случаях — кнопки быстрых действий.
func (a *Agent) degraded(ctx context.Context, login string, tc tools.Ctx, specs []llm.ToolSpec, text string, cause error) Reply {
	if a.Backup != nil {
		a.mu.Lock()
		c := a.backups[login]
		if c == nil || c.turns >= maxTurns {
			c = &chat{session: a.Backup.Start(SystemPrompt, a.contextFor(tc.User), specs)}
			a.backups[login] = c
		}
		c.turns++
		a.mu.Unlock()
		bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupTimeout)
		defer cancel()
		reply := Reply{Drafts: []Draft{}, Sources: []Source{}, Degraded: true}
		step, err := a.run(bctx, c.session, tc, &reply, text)
		if err == nil && !step.Refused && strings.TrimSpace(step.Text) != "" {
			reply.Text = degradedReason(cause) + " Отвечаю в упрощённом режиме, по правилам.\n\n" + honestMiss(tc, &reply, step.Text, text)
			reply.Retry = len(reply.Drafts) == 0 // «Повторить» спросит модель ещё раз; с черновиком повтор не нужен
			if len(reply.Drafts) == 0 {
				reply.Actions = quickActions(tc.User, text)
			}
			return reply
		}
		a.mu.Lock()
		delete(a.backups, login)
		a.mu.Unlock()
	}
	r := a.fallback(tc, text, cause)
	r.Actions = quickActions(tc.User, text)
	return r
}

// quickActions — частые действия роли фразами, которые понимают и модель, и правила.
func quickActions(u *mockdata.User, question string) []QuickAction {
	can := map[string]bool{}
	for _, t := range tools.For(u) {
		can[t.Name] = true
	}
	var out []QuickAction
	if can["send_question"] && strings.TrimSpace(question) != "" && !strings.HasPrefix(strings.ToLower(question), "не помогло") {
		out = append(out, QuickAction{"Спросить человека", "Не помогло, нужен человек: " + question})
	}
	if can["get_today"] {
		out = append(out, QuickAction{"Что сегодня по плану", "Что мне сегодня делать?"})
	}
	if can["create_absence_request"] {
		out = append(out, QuickAction{"Оформить отгул", "Нужен отгул"})
	}
	if can["team_progress"] {
		out = append(out, QuickAction{"Кто из стажёров отстаёт", "Кто из моих стажёров отстаёт?"})
	}
	if can["get_required_accesses"] {
		out = append(out, QuickAction{"Какие доступы нужны", "Какие доступы мне нужны?"})
	}
	same := strings.ToLower(strings.TrimSpace(question))
	return slices.DeleteFunc(out, func(x QuickAction) bool { return strings.ToLower(x.Text) == same }) // не повторять только что отправленное
}

// fallback — последний рубеж BR-70: ни модели, ни правил → найденные фрагменты базы без пересказа.
func (a *Agent) fallback(tc tools.Ctx, text string, err error) Reply {
	reply := Reply{Drafts: []Draft{}, Sources: []Source{}, Degraded: true, Retry: true}
	reason := degradedReason(err)
	hits := tools.SearchKB(tc, text)
	if len(hits) == 0 {
		reply.Text = reason + " Твой вопрос сохранён — нажми «Повторить». План и заявки работают без модели."
		return reply
	}
	reply.Text = reason + " Вот что нашёл в базе без пересказа:\n" + hits[0].Body
	if hits[0].Label == tools.ConfidentialLabel {
		reply.Text = reason + " Нашёл страницу «" + hits[0].Title + "» под грифом «конфиденциально» — открой её в базе."
	}
	for _, h := range hits {
		reply.Sources = append(reply.Sources, Source{h.PageID, h.Title, h.Author, h.UpdatedAt, h.Label})
	}
	return reply
}

// Reset — начать разговор заново (смена учётки, ошибка модели, кнопка «Новый разговор»).
func (a *Agent) Reset(login string) {
	lock := a.userLock(login)
	lock.Lock()
	defer lock.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, login)
	delete(a.backups, login)
}

// Confirm — человек нажал «Подтвердить»: выполнить черновик и вернуть квитанцию (BR-43).
// Если сервис недоступен, черновик сохраняется — можно повторить (SC-15).
// Draft возвращает только незавершённый черновик; квитанция остаётся для повторов.
func (a *Agent) Draft(id string) (Draft, bool) {
	d, ok := a.Store.ActionDraft(id)
	return draftView(d), ok && len(d.Result) == 0 && !d.Cancelled
}
