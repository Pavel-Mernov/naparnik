// Package app собирает приложение из переменных окружения: мок-данные, состояние, модель, агент.
package app

import (
	"context"
	"os"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type App struct {
	Store *store.Store
	Agent *agent.Agent
	LLM   llm.Config
}

// New: MOCK_DATA_DIR (по умолчанию ../docs/04-mock-data), DEMO_TODAY (по умолчанию из мок-данных), LLM_* — см. .env.example.
func New() (*App, error) {
	dir := os.Getenv("MOCK_DATA_DIR")
	if dir == "" {
		dir = "../docs/04-mock-data"
	}
	data, err := mockdata.Load(dir)
	if err != nil {
		return nil, err
	}
	todayStr := os.Getenv("DEMO_TODAY")
	if todayStr == "" {
		todayStr = data.DemoToday
	}
	today, err := time.Parse(time.DateOnly, todayStr)
	if err != nil {
		today = time.Now()
	}
	cfg, err := llm.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	s := store.New(data, today)
	if url := os.Getenv("DATABASE_URL"); url != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s, err = store.OpenPostgres(ctx, data, today, url)
		if err != nil {
			return nil, err
		}
	}
	return &App{Store: s, Agent: agent.New(s, llm.New(cfg, today), cfg.Timeout), LLM: cfg}, nil
}
