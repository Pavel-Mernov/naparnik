package llm

import (
	"context"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/practice"
)

type practiceProvider struct{ step Step }

func (p practiceProvider) Start(string, string, []ToolSpec) Session { return practiceSession{p.step} }

type practiceSession struct{ step Step }

func (s practiceSession) SendUser(context.Context, string) (Step, error) { return s.step, nil }
func (s practiceSession) SendToolResults(context.Context, []ToolResult) (Step, error) {
	return s.step, nil
}

// BR-104: число/номера критериев проверяет код, фальшивые итоговые passed/score модели игнорируются.
func TestPracticeReviewValidation(t *testing.T) {
	input := practice.ModelInput{Task: practice.Task{Criteria: []string{"Критерий"}}}
	good := map[string]any{"checks": []any{map[string]any{"criterion": 1, "passed": false, "feedback": "Нужна правка"}}, "passed": true, "score": 100}
	p := practiceProvider{Step{ToolCalls: []ToolCall{{Name: "practice_review", Input: good}}}}
	r, e := ReviewPractice(context.Background(), p, input)
	if e != nil || r.Passed || r.Score != 0 {
		t.Fatal(r, e)
	}
	for _, checks := range []any{[]any{}, []any{map[string]any{"criterion": 2, "passed": true, "feedback": "ok"}}, []any{map[string]any{"criterion": 1, "passed": true}}, []any{map[string]any{"criterion": 1, "passed": true, "feedback": ""}}} {
		p := practiceProvider{Step{ToolCalls: []ToolCall{{Name: "practice_review", Input: map[string]any{"checks": checks}}}}}
		if _, e := ReviewPractice(context.Background(), p, input); e == nil {
			t.Fatal(checks)
		}
	}
	for _, step := range []Step{{Text: "Всё отлично"}, {Refused: true}, {ToolCalls: []ToolCall{{Name: "execute_sql"}}}} {
		if _, e := ReviewPractice(context.Background(), practiceProvider{step}, input); e == nil {
			t.Fatal(step)
		}
	}
}

// BR-103: генератор обязан оставить указанный синтетический набор, произвольный текст не принимается.
func TestPracticeGenerationValidation(t *testing.T) {
	input := practice.ModelInput{Task: practice.Task{Text: "Учебная задача на sandbox.training", Dataset: "sandbox.training (синтетика)"}}
	for _, text := range []string{"", "Иди в продовую витрину"} {
		p := practiceProvider{Step{ToolCalls: []ToolCall{{Name: "practice_generate", Input: map[string]any{"text": text}}}}}
		if _, e := GeneratePractice(context.Background(), p, input); e == nil {
			t.Fatal(text)
		}
	}
	text, e := GeneratePractice(context.Background(), NewStub("stub_ok", today), input)
	if e != nil || text != input.Task.Text {
		t.Fatal(text, e)
	}
}
