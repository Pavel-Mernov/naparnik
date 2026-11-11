// Package app собирает приложение из переменных окружения: мок-данные, состояние, модель, агент.
package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
	"github.com/worker4556/naparnik/backend/internal/telegram"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

type App struct {
	Store     *store.Store
	Agent     *agent.Agent
	LLM       llm.Config
	KV        cache.KV          // Redis (REDIS_URL) или память
	TG        *telegram.Service // уведомления в Telegram (TG_BOT_TOKEN; без него — режим log)
	ChatLimit int               // LLM_RATE_PER_MIN, по умолчанию 20
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
	// Как и с PostgreSQL: если REDIS_URL задан и недоступен — ошибка запуска, без тихого перехода в память.
	kv, err := cache.New(os.Getenv("REDIS_URL"))
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	tools.KBCache = kv // кэш поиска по базе и подбора эксперта
	limit := 20
	if v, err := strconv.Atoi(os.Getenv("LLM_RATE_PER_MIN")); err == nil {
		limit = v
	}
	return &App{Store: s, Agent: agent.New(s, llm.New(cfg, today), cfg.Timeout), LLM: cfg,
		KV: kv, TG: telegram.New(telegram.ConfigFromEnv(), s, kv), ChatLimit: limit}, nil
}
