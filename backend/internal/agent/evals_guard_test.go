package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

// Страховки, найденные прогоном evals.json (docs/evals): они не зависят от того, как сформулировала модель.

func guardCtx(t *testing.T, question string) tools.Ctx {
	a := newAgent(t, "stub_ok", time.Second)
	u, _ := a.Store.UserByLogin("intern.sokolov")
	return tools.Ctx{Store: a.Store, User: u, Question: question}
}

// M-15, SC-04: поиск пуст — ответ начинается с «в базе нет» и называет человека, даже если модель промолчала.
func TestHonestMissNamesExpert(t *testing.T) {
	q := "Как исключить ботов из выборки эксперимента?"
	r := &Reply{searched: true}
	got := honestMiss(guardCtx(t, q), r, "Могу помочь сформулировать вопрос.", q)
	if !strings.HasPrefix(got, "В базе ответа на это нет.") || !strings.Contains(got, "Ольга Смирнова") {
		t.Fatal(got)
	}
	// Модель сама честно сказала — не дублируем.
	if got := honestMiss(guardCtx(t, q), &Reply{searched: true, expertAsked: true}, "В базе по этому вопросу пусто.", q); strings.Count(got, "в базе")+strings.Count(got, "В базе") != 1 {
		t.Fatal(got)
	}
}

// Нашлись соседние страницы, но модель готовит вопрос человеку — значит, ответа в базе нет, и это сказано.
func TestDraftAfterSearchIsHonest(t *testing.T) {
	r := &Reply{searched: true, found: true, Sources: []Source{{PageID: "kb-10"}}, Drafts: []Draft{{Tool: "send_question"}}}
	if got := honestMiss(guardCtx(t, "x"), r, "Черновик вопроса Наталье готов.", "x"); !strings.HasPrefix(got, "В базе ответа на это нет.") {
		t.Fatal(got)
	}
}

// Нашлось соседнее, модель подобрала эксперта, но не сказала, что ответа нет (evals e-24).
func TestExpertAfterSearchIsHonest(t *testing.T) {
	r := &Reply{searched: true, found: true, expertAsked: true, Sources: []Source{{PageID: "kb-10"}}}
	if got := honestMiss(guardCtx(t, "x"), r, "Можно спросить у Ольги Смирновой.", "x"); !strings.HasPrefix(got, "В базе ответа на это нет.") {
		t.Fatal(got)
	}
}

// BR-05: пометка тестовой инструкции попадает в ответ, даже если модель её опустила.
func TestTestInstructionLabelAlwaysShown(t *testing.T) {
	r := &Reply{Sources: []Source{{PageID: "kb-02", Title: "Отгулы", Label: tools.TestInstructionLabel}}}
	if got := testLabel(r, "Отгул — за 2 рабочих дня."); !strings.Contains(got, tools.TestInstructionLabel) {
		t.Fatal(got)
	}
	if got := testLabel(r, "Это тестовая инструкция."); strings.Contains(got, "Источник «") {
		t.Fatal("пометка уже есть — не дублируем:", got)
	}
}

// BR-01, M-16: ответ инструмента правил приходит с источником, без вызова поиска.
func TestRulesToolsCarrySource(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	u, _ := a.Store.UserByLogin("intern.sokolov")
	r := Reply{}
	a.handle(tools.Ctx{Store: a.Store, User: u}, llm.ToolCall{Name: "get_absence_rules", Input: map[string]any{"type": "day_off"}}, &r)
	for _, src := range r.Sources {
		if src.PageID == "kb-02" && src.Label != "" {
			return
		}
	}
	t.Fatalf("нет источника kb-02 с пометкой: %+v (%v)", r.Sources, r.ToolLog)
}

// BR-60: с вопросом о штате и оценке стажёр идёт к конкретным людям.
func TestBoundaryNamesPeople(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	for _, q := range []string{"Меня возьмут в штат?", "Как меня оценивает руководитель?", "Сколько получают стажёры в других отделах?"} {
		r := ask(t, a, "intern.sokolov", q)
		if !strings.Contains(r.Text, "Екатерина Белова") || !strings.Contains(r.Text, "Андрей Ковалёв") || len(r.ToolLog) != 0 {
			t.Fatal(q, r.Text)
		}
	}
}
