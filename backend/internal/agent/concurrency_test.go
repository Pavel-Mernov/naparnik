package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/llm"
)

// Двойной клик не создаёт вторую заявку даже без PostgreSQL (например, в тестовом режиме).
func TestConcurrentConfirm(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Мне нужен отгул в пятницу")
	var wg sync.WaitGroup
	failed := make(chan bool, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); failed <- !a.Confirm("intern.sokolov", r.Drafts[0].ID).OK }()
	}
	wg.Wait()
	close(failed)
	for fail := range failed {
		if fail {
			t.Fatal("повтор должен вернуть результат")
		}
	}
	if len(a.Store.Requests("")) != 1 {
		t.Fatal("созданы дубликаты")
	}
}

type serialProvider struct{ overlapping atomic.Bool }
type serialSession struct {
	provider *serialProvider
	active   atomic.Int32
}

func (p *serialProvider) Start(string, string, []llm.ToolSpec) llm.Session {
	return &serialSession{provider: p}
}
func (s *serialSession) SendUser(context.Context, string) (llm.Step, error) {
	if s.active.Add(1) != 1 {
		s.provider.overlapping.Store(true)
	}
	time.Sleep(time.Millisecond)
	s.active.Add(-1)
	return llm.Step{Text: "ok"}, nil
}
func (s *serialSession) SendToolResults(context.Context, []llm.ToolResult) (llm.Step, error) {
	return llm.Step{}, nil
}

// История SDK изменяется целым раундом, а не перемешивается между вкладками одного пользователя.
func TestConcurrentConversationIsSerialized(t *testing.T) {
	base := newAgent(t, "stub_ok", time.Second)
	p := &serialProvider{}
	a := New(base.Store, p, time.Second)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = a.Ask(context.Background(), "intern.sokolov", "привет") }()
	}
	wg.Wait()
	if p.overlapping.Load() {
		t.Fatal("сессия модели используется одновременно")
	}
}
