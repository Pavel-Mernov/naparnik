// Package app собирает приложение из переменных окружения: мок-данные, состояние, модель, агент.
package app

import (
	"context"
	"fmt"
	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/flags"
	"log"
	"os"
	"strconv"
	"strings"
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
	KV        cache.KV           // Redis (REDIS_URL) или память
	TG        *telegram.Service  // уведомления в Telegram (TG_BOT_TOKEN; без него — режим log)
	ChatLimit int                // LLM_RATE_PER_MIN, по умолчанию 20
	Events    *adapters.Exporter // выгрузка журнала событий (EVENTS_URL / EVENTS_FILE); nil — не выгружается
	// AbsenceSync — опрос решений в настоящем сервисе отсутствий (ABSENCE_SERVICE_URL); nil — имитация, опрашивать нечего.
	AbsenceSync         adapters.AbsenceStatusSource
	AbsenceSyncInterval time.Duration
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
	absence := absenceFromEnv()
	tools.Services = servicesWith(absence)
	flags.Default = flags.FromEnv()
	limit := 20
	if v, err := strconv.Atoi(os.Getenv("LLM_RATE_PER_MIN")); err == nil {
		limit = v
	}
	events, err := EventsExporter()
	if err != nil {
		return nil, err
	}
	return &App{Store: s, Agent: agent.New(s, llm.New(cfg, today), cfg.Timeout), LLM: cfg,
		KV: kv, TG: telegram.New(telegram.ConfigFromEnv(), s, kv), ChatLimit: limit, Events: events,
		AbsenceSync: absenceSource(absence), AbsenceSyncInterval: syncInterval()}, nil
}

// Services — какие реализации корпоративных сервисов подключить (BR-74). По умолчанию — имитации на мок-данных;
// ABSENCE_SERVICE_URL — настоящий сервис отсутствий по REST (токен — ABSENCE_SERVICE_TOKEN).
func Services() func(*store.Store) adapters.Set { return servicesWith(absenceFromEnv()) }

// absenceFromEnv — клиент настоящего сервиса отсутствий или nil, если ABSENCE_SERVICE_URL не задан.
func absenceFromEnv() *adapters.HTTPAbsence {
	url := os.Getenv("ABSENCE_SERVICE_URL")
	if url == "" {
		return nil
	}
	log.Printf("сервис отсутствий: %s", url)
	return adapters.NewHTTPAbsence(strings.TrimRight(url, "/"), os.Getenv("ABSENCE_SERVICE_TOKEN"))
}

// absenceSource — без настоящего сервиса опрашивать нечего (интерфейс с nil-указателем не равен nil — проверяем явно).
func absenceSource(h *adapters.HTTPAbsence) adapters.AbsenceStatusSource {
	if h == nil {
		return nil
	}
	return h
}

// syncInterval — ABSENCE_SYNC_SEC, по умолчанию раз в минуту.
func syncInterval() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("ABSENCE_SYNC_SEC")); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return time.Minute
}

func servicesWith(absence *adapters.HTTPAbsence) func(*store.Store) adapters.Set {
	return func(s *store.Store) adapters.Set {
		set := adapters.NewMockSet(s.Data, s.NextID)
		if absence != nil {
			set.Absence = absence
		}
		return set
	}
}

// EventsExporter — выгрузка журнала событий в аналитику: EVENTS_URL — HTTP-коллектор (Clickstream-шлюз,
// REST-прокси Kafka; токен EVENTS_TOKEN), EVENTS_FILE — JSONL-файл. Без них — только журнал в PostgreSQL.
func EventsExporter() (*adapters.Exporter, error) {
	var w adapters.Writer
	switch {
	case os.Getenv("EVENTS_URL") != "":
		w = adapters.HTTPCollector{URL: os.Getenv("EVENTS_URL"), Token: os.Getenv("EVENTS_TOKEN")}
	case os.Getenv("EVENTS_FILE") != "":
		f, err := os.OpenFile(os.Getenv("EVENTS_FILE"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		w = &adapters.JSONLines{Out: f}
	default:
		return nil, nil
	}
	exp := adapters.NewExporter(w, 10000)
	store.EventSink = exp.Sink
	return exp, nil
}
