package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

func newAgent(t *testing.T, mode string, timeout time.Duration) *Agent {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s := store.New(d, today)
	return New(s, llm.NewStub(mode, today), timeout)
}

func ask(t *testing.T, a *Agent, login, text string) Reply {
	t.Helper()
	r, err := a.Ask(context.Background(), login, text)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// SC-03: ответ из базы с источником; людям ничего не ушло.
func TestAnswerFromKB(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Что такое витрина и где лежит витрина объявлений?")
	if !strings.Contains(r.Text, "ads_mart") || !strings.Contains(r.Text, "06:00") || len(r.Sources) == 0 || r.Sources[0].Author != "Михаил Орлов" {
		t.Fatalf("ответ: %+v", r)
	}
	if len(r.Drafts) != 0 || len(a.Store.Questions()) != 0 {
		t.Fatal("вопрос из базы не должен уходить людям")
	}
}

// SC-04 / SC-07, BR-11: нет в базе → черновик вопроса человеку, без подтверждения никуда не ушёл.
func TestNoAnswerMakesQuestionDraft(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Где взять шаблон презентации для экватора?")
	if len(r.Drafts) != 1 || r.Drafts[0].Tool != "send_question" || r.Drafts[0].Args["addressee_id"] != "u-belova" {
		t.Fatalf("черновик: %+v", r)
	}
	if len(a.Store.Questions()) != 0 {
		t.Fatal("SC-05: без подтверждения вопрос не отправляется")
	}
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); !res.OK || len(a.Store.Questions()) != 1 {
		t.Fatalf("после подтверждения: %+v", res)
	}
}

// SC-14, SC-29, BR-43: отгул — черновик → подтверждение → квитанция; чужой черновик не подтвердить.
func TestAbsenceDraftAndConfirm(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Мне нужен отгул в пятницу")
	if !strings.Contains(r.Text, "тестовая инструкция") || len(r.Drafts) != 1 {
		t.Fatalf("отгул: %+v", r)
	}
	if len(a.Store.Requests("u-sokolov")) != 0 {
		t.Fatal("заявка до подтверждения")
	}
	if res := a.Confirm("intern.egorova", r.Drafts[0].ID); res.Error == nil || res.Error.Code != "not_found" {
		t.Fatal("чужой черновик")
	}
	if _, ok := a.Draft(r.Drafts[0].ID); !ok {
		t.Fatal("черновик должен остаться после чужой попытки")
	}
	res := a.Confirm("intern.sokolov", r.Drafts[0].ID)
	if !res.OK || res.Receipt.(map[string]string)["id"] != "ABS-1042" {
		t.Fatalf("квитанция: %+v", res)
	}
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); !res.OK || len(a.Store.Requests("u-sokolov")) != 1 {
		t.Fatal("повтор возвращает квитанцию без новой заявки")
	}
}

// SC-15: сервис недоступен — черновик сохраняется для повтора.
func TestUnavailableKeepsDraft(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Мне нужен отгул в пятницу")
	t.Setenv("MOCK_FAIL", "absence")
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); res.Error.Code != "unavailable" {
		t.Fatalf("ожидалась недоступность: %+v", res)
	}
	t.Setenv("MOCK_FAIL", "")
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); !res.OK {
		t.Fatalf("повтор должен пройти: %+v", res.Error)
	}
}

// SC-16, BR-40: недоступное не предлагается даже черновиком.
func TestForbiddenNotDrafted(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Покажи данные витрины с персональными данными клиентов, нужен доступ")
	if len(r.Drafts) != 0 || !strings.Contains(r.Text, "Андрей Ковалёв") {
		t.Fatalf("отказ с адресатом: %+v", r)
	}
}

// SC-17, SC-28, BR-60, BR-61: границы — до модели.
func TestGuard(t *testing.T) {
	a := New(nil, panicProvider{}, time.Second) // модель не должна вызываться вовсе
	a.Store = newAgent(t, "stub_ok", time.Second).Store
	if r := ask(t, a, "intern.sokolov", "Меня возьмут в штат после стажировки?"); !strings.Contains(r.Text, "решают люди") {
		t.Fatal(r.Text)
	}
	if r := ask(t, a, "intern.sokolov", "Вот мои паспортные данные 4510 123456, внеси"); !strings.Contains(r.Text, "не обрабатываю") || strings.Contains(r.Text, "4510") {
		t.Fatal(r.Text)
	}
	if got := MaskPII("карта 4276 1234 5678 9012, паспорт 45 10 123456"); strings.Contains(got, "4276") || strings.Contains(got, "123456") {
		t.Fatal(got)
	}
}

type panicProvider struct{}

func (panicProvider) Start(string, string, []llm.ToolSpec) llm.Session {
	panic("модель не должна вызываться")
}

// SC-26, BR-70: модель недоступна или молчит — тот же порядок действий по правилам, вопрос не теряется.
func TestFallback(t *testing.T) {
	r := ask(t, newAgent(t, "stub_error", time.Second), "intern.sokolov", "Что такое витрина и где лежит витрина объявлений?")
	if !r.Degraded || !r.Retry || len(r.Sources) == 0 || !strings.Contains(r.Text, "упрощённом режиме") || len(r.Actions) == 0 {
		t.Fatalf("stub_error: %+v", r)
	}
	r = ask(t, newAgent(t, "stub_timeout", 30*time.Millisecond), "intern.sokolov", "Что мне сегодня делать?")
	if !r.Degraded || !strings.Contains(r.Text, "не ответила вовремя") || !strings.Contains(r.Text, "Неделя") {
		t.Fatalf("stub_timeout: %+v", r)
	}
	// Без правил — последний рубеж: фрагменты базы без пересказа и «Повторить».
	a := newAgent(t, "stub_error", time.Second)
	a.Backup = nil
	r = ask(t, a, "intern.sokolov", "Что такое витрина и где лежит витрина объявлений?")
	if !r.Degraded || !r.Retry || len(r.Sources) == 0 || !strings.Contains(r.Text, "без пересказа") || len(r.Actions) == 0 {
		t.Fatalf("без правил: %+v", r)
	}
	r = ask(t, a, "intern.sokolov", "zzz qqq")
	if !strings.Contains(r.Text, "Повторить") {
		t.Fatalf("без правил, пусто: %+v", r)
	}
}

// BR-70, BR-43: модель упала — отгул всё равно оформляется в два сообщения через черновик и подтверждение.
func TestFallbackKeepsActions(t *testing.T) {
	a := newAgent(t, "stub_error", time.Second)
	r := ask(t, a, "intern.sokolov", "Нужен отгул")
	if !r.Degraded || len(r.Drafts) != 0 || !strings.Contains(r.Text, "На какую дату") {
		t.Fatalf("уточнение даты: %+v", r)
	}
	r = ask(t, a, "intern.sokolov", "в пятницу")
	if len(r.Drafts) != 1 || r.Drafts[0].Tool != "create_absence_request" || r.Retry || len(r.Actions) != 0 {
		t.Fatalf("черновик отгула: %+v", r)
	}
	if len(a.Store.Requests("u-sokolov")) != 0 {
		t.Fatal("без подтверждения ничего не уходит")
	}
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); !res.OK || len(a.Store.Requests("u-sokolov")) != 1 {
		t.Fatalf("подтверждение: %+v", res)
	}
	// Вопроса нет в базе — правила готовят вопрос человеку, тоже черновиком.
	r = ask(t, a, "intern.sokolov", "Как согласовать бюджет на рекламу в Тайланде?")
	if len(r.Drafts) != 1 || r.Drafts[0].Tool != "send_question" || !strings.HasPrefix(strings.SplitN(r.Text, "\n\n", 2)[1], "В базе ответа на это нет") {
		t.Fatalf("вопрос человеку: %+v", r)
	}
}

// Кнопки быстрых действий — только то, что роли доступно.
func TestQuickActionsByRole(t *testing.T) {
	a := newAgent(t, "stub_error", time.Second)
	intern := ask(t, a, "intern.sokolov", "Что такое витрина?")
	labels := []string{}
	for _, x := range intern.Actions {
		labels = append(labels, x.Label)
	}
	if strings.Join(labels, "|") != "Спросить человека|Что сегодня по плану|Оформить отгул|Какие доступы нужны" ||
		intern.Actions[0].Text != "Не помогло, нужен человек: Что такое витрина?" {
		t.Fatalf("стажёр: %+v", intern.Actions)
	}
	lead := ask(t, a, "lead.kovalev", "Как согласовать бюджет на рекламу в Тайланде?")
	if len(lead.Actions) != 0 || len(lead.Drafts) != 0 || strings.Contains(lead.Text, "недоступно") {
		t.Fatalf("руководитель: %+v", lead)
	}
}

func TestUnknownUser(t *testing.T) {
	if _, err := newAgent(t, "stub_ok", time.Second).Ask(context.Background(), "nobody", "x"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

// Контекст для модели: кто, неделя, бадди и руководитель.
func TestContextFor(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	u, _ := a.Store.UserByLogin("intern.sokolov")
	ctx := a.contextFor(u)
	for _, want := range []string{"2026-11-11 (среда)", "Неделя стажировки: 1", "Екатерина Белова", "Андрей Ковалёв"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("нет %q в контексте:\n%s", want, ctx)
		}
	}
	lead, _ := a.Store.UserByLogin("lead.kovalev")
	if strings.Contains(a.contextFor(lead), "Неделя") {
		t.Error("у руководителя нет недели стажировки")
	}
}

// Зацикливание и отказ модели.
type scriptedProvider struct{ steps []llm.Step }

func (p *scriptedProvider) Start(string, string, []llm.ToolSpec) llm.Session { return &scripted{p: p} }

type scripted struct {
	p *scriptedProvider
	i int
}

func (s *scripted) next() (llm.Step, error) {
	st := s.p.steps[min(s.i, len(s.p.steps)-1)]
	s.i++
	return st, nil
}
func (s *scripted) SendUser(context.Context, string) (llm.Step, error) { return s.next() }
func (s *scripted) SendToolResults(context.Context, []llm.ToolResult) (llm.Step, error) {
	return s.next()
}

func TestLoopLimitAndRefusal(t *testing.T) {
	base := newAgent(t, "stub_ok", time.Second)
	loop := New(base.Store, &scriptedProvider{steps: []llm.Step{{ToolCalls: []llm.ToolCall{{ID: "1", Name: "get_plan"}}}}}, time.Second)
	if r := ask(t, loop, "intern.sokolov", "?"); !strings.Contains(r.Text, "Не получилось") {
		t.Fatal(r.Text)
	}
	refuse := New(base.Store, &scriptedProvider{steps: []llm.Step{{Refused: true}}}, time.Second)
	if r := ask(t, refuse, "intern.sokolov", "?"); !strings.Contains(r.Text, "помочь не могу") {
		t.Fatal(r.Text)
	}
}

// Разговор продолжается: вторая реплика идёт в ту же сессию; ошибка и Reset начинают заново.
type countingProvider struct{ starts int }

func (p *countingProvider) Start(string, string, []llm.ToolSpec) llm.Session {
	p.starts++
	return &scripted{p: &scriptedProvider{steps: []llm.Step{{Text: "ок"}}}}
}

func TestConversationIsKept(t *testing.T) {
	base := newAgent(t, "stub_ok", time.Second)
	p := &countingProvider{}
	a := New(base.Store, p, time.Second)
	ask(t, a, "intern.sokolov", "Мне нужен отгул в пятницу")
	ask(t, a, "intern.sokolov", "Сделай за меня")
	if p.starts != 1 {
		t.Fatalf("разговор должен продолжаться, сессий: %d", p.starts)
	}
	ask(t, a, "intern.egorova", "Привет")
	if p.starts != 2 {
		t.Fatal("у другого пользователя — своя сессия")
	}
	a.Reset("intern.sokolov")
	ask(t, a, "intern.sokolov", "Снова")
	if p.starts != 3 {
		t.Fatal("после Reset — новая сессия")
	}
	for i := 0; i < maxTurns; i++ {
		ask(t, a, "intern.sokolov", "ещё")
	}
	if p.starts != 4 {
		t.Fatalf("после %d реплик — новая сессия, сессий: %d", maxTurns, p.starts)
	}
}

func TestErrorResetsConversation(t *testing.T) {
	a := newAgent(t, "stub_error", time.Second)
	ask(t, a, "intern.sokolov", "x")
	if len(a.sessions) != 0 {
		t.Fatal("после ошибки модели разговор сбрасывается")
	}
}

// Модель пообещала черновик, но не вызвала инструмент — агент один раз просит довести дело,
// и стажёр получает черновик с кнопкой «Подтвердить».
func TestPromisedDraftIsCreated(t *testing.T) {
	base := newAgent(t, "stub_ok", time.Second)
	promise := llm.Step{Text: "Проверил дату. Сейчас покажу черновик — проверь и подтверди."}
	call := llm.Step{ToolCalls: []llm.ToolCall{{ID: "1", Name: "create_absence_request", Input: map[string]any{"type": "day_off", "date": "2026-11-17"}}}}
	done := llm.Step{Text: "Черновик готов, нажми «Подтвердить»."}
	a := New(base.Store, &scriptedProvider{steps: []llm.Step{promise, call, done}}, time.Second)
	r := ask(t, a, "intern.sokolov", "Отгул 17 ноября, сделай за меня")
	if len(r.Drafts) != 1 || r.Drafts[0].Tool != "create_absence_request" || r.Text != done.Text {
		t.Fatalf("черновик не создан: %+v", r)
	}
	// обычный ответ без обещаний — без повторного запроса к модели
	plain := New(base.Store, &scriptedProvider{steps: []llm.Step{{Text: "Отгул подают за 2 рабочих дня."}, call}}, time.Second)
	if r := ask(t, plain, "intern.sokolov", "Как оформить отгул?"); len(r.Drafts) != 0 {
		t.Fatalf("лишний черновик: %+v", r)
	}
	// у руководителя нет инструментов с подтверждением — не подталкиваем
	if r := ask(t, New(base.Store, &scriptedProvider{steps: []llm.Step{promise, call}}, time.Second), "lead.kovalev", "?"); len(r.Drafts) != 0 {
		t.Fatalf("руководитель: %+v", r)
	}
	for text, want := range map[string]bool{
		"Черновик готов: отгул 17 ноября": true, "Подготовлю черновик заявки": true, "Нажми «Подтвердить»": true,
		"Отгул подают за 2 рабочих дня": false, "Черновик плана утверждает руководитель": false,
	} {
		if promisesDraft(text) != want {
			t.Errorf("%q: ожидалось %v", text, want)
		}
	}
}

// recordingProvider — сценарий модели, который запоминает всё, что ей передали результаты инструментов.
type recordingProvider struct {
	steps []llm.Step
	seen  []string
}

func (p *recordingProvider) Start(string, string, []llm.ToolSpec) llm.Session {
	return &recording{p: p}
}

type recording struct {
	p *recordingProvider
	i int
}

func (s *recording) next() (llm.Step, error) {
	st := s.p.steps[min(s.i, len(s.p.steps)-1)]
	s.i++
	return st, nil
}
func (s *recording) SendUser(context.Context, string) (llm.Step, error) { return s.next() }
func (s *recording) SendToolResults(_ context.Context, rs []llm.ToolResult) (llm.Step, error) {
	for _, r := range rs {
		s.p.seen = append(s.p.seen, r.Content)
	}
	return s.next()
}

// BR-49: текст конфиденциальной страницы не уходит во внешнюю модель, а стажёр получает ссылку и пометку.
func TestConfidentialNotSentToModel(t *testing.T) {
	base := newAgent(t, "stub_ok", time.Second)
	q := "Какой план запусков раздела объявлений на квартал?"
	p := &recordingProvider{steps: []llm.Step{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "search_kb", Input: map[string]any{"query": q}}}},
		{Text: "Нашёл страницу «План запусков раздела «Объявления» на I квартал 2027», Дмитрий Волков."},
	}}
	r := ask(t, New(base.Store, p, time.Second), "intern.sokolov", q)
	for _, c := range p.seen {
		if strings.Contains(c, "пилот подписки") {
			t.Fatalf("модель получила текст страницы: %s", c)
		}
	}
	if len(r.Sources) == 0 || r.Sources[0].PageID != "kb-41" || !strings.Contains(r.Text, "конфиденциально") {
		t.Fatalf("ответ: %+v", r)
	}
	// Запасной режим без модели — тоже ссылка, без текста и без служебной заглушки.
	off := ask(t, newAgent(t, "stub_error", time.Second), "intern.sokolov", "Что с запусками в разделе объявлений в первом квартале?")
	if strings.Contains(off.Text, "подписки") || strings.Contains(off.Text, "[Гриф") || !strings.Contains(off.Text, "конфиденциально") {
		t.Fatalf("упрощённый режим: %s", off.Text)
	}
}
