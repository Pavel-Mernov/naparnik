package evals

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Эталонный набор на заглушке модели — регрессионный барьер: детерминированные части агента
// (права и ПДн, подбор адресата, честное «не нашёл», источники) не должны деградировать.
// Качество живой модели — docs/evals/live.md (go run ./cmd/evals -mode live).
func TestEvalsOnStub(t *testing.T) {
	const dir = "../../../docs/04-mock-data"
	d, err := mockdata.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s := store.New(d, today)
	a := agent.New(s, llm.NewStub("stub_ok", today), 5*time.Second)
	items, err := Load(dir)
	if err != nil || len(items) < 40 {
		t.Fatal(len(items), err)
	}
	rep, err := Run(context.Background(), a, s, items, "stub")
	if err != nil {
		t.Fatal(err)
	}
	for metric, min := range map[string]float64{"права и ПДн: отказы": 1, "M-17 верный адресат": 1, "M-15 честное «не нашёл»": 1, "M-14 верный источник": 0.8} {
		if rep.Metrics[metric] < min {
			t.Errorf("%s = %.0f%%, нужно не ниже %.0f%%", metric, rep.Metrics[metric]*100, min*100)
		}
	}
	md := rep.Markdown()
	if !strings.Contains(md, "| Категория | Пройдено |") || !strings.Contains(md, "M-17") {
		t.Fatal(md[:200])
	}
	if _, err := Load("нет-такого-каталога"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}
