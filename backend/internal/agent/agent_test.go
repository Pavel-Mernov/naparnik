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
	if res := a.Confirm("intern.sokolov", r.Drafts[0].ID); res.Error == nil {
		t.Fatal("черновик исполняется один раз")
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

// SC-26, BR-70: модель недоступна или молчит — запасной ответ из базы, вопрос сохранён.
func TestFallback(t *testing.T) {
	r := ask(t, newAgent(t, "stub_error", time.Second), "intern.sokolov", "Что такое витрина и где лежит витрина объявлений?")
	if !r.Degraded || !r.Retry || len(r.Sources) == 0 || !strings.Contains(r.Text, "без пересказа") {
		t.Fatalf("stub_error: %+v", r)
	}
	r = ask(t, newAgent(t, "stub_timeout", 30*time.Millisecond), "intern.sokolov", "zzz qqq")
	if !r.Degraded || !strings.Contains(r.Text, "не ответила вовремя") || !strings.Contains(r.Text, "Повторить") {
		t.Fatalf("stub_timeout: %+v", r)
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
