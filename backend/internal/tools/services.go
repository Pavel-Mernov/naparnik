package tools

import (
	"context"
	"errors"
	"time"

	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Хранилище реализует интерфейсы базы знаний и трекера — проверка при компиляции.
var (
	_ adapters.KnowledgeBase = (*store.Store)(nil)
	_ adapters.Tracker       = (*store.Store)(nil)
)

// Services — какие реализации сервисов подключены. По умолчанию — имитации на мок-данных; app.New подменяет
// по конфигурации (например, ABSENCE_SERVICE_URL → HTTP-клиент настоящего сервиса отсутствий).
// Фабрика получает хранилище текущей транзакции: номера квитанций моков берутся из него.
var Services = func(s *store.Store) adapters.Set { return adapters.NewMockSet(s.Data, s.NextID) }

func (c Ctx) svc() adapters.Set { return Services(c.Store) }

func svcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

// serviceError — сбой сервиса для стажёра: черновик сохранён, можно повторить (SC-15).
func serviceError(err error, what string) Result {
	if errors.Is(err, adapters.ErrUnavailable) {
		return fail("unavailable", what+" временно недоступен. Черновик сохранён — можно повторить.")
	}
	return fail("unavailable", what+" ответил ошибкой. Черновик сохранён — можно повторить.")
}

func absenceCatalog(c Ctx) adapters.AbsenceCatalog {
	ctx, cancel := svcCtx()
	defer cancel()
	cat, _ := c.svc().Absence.Catalog(ctx)
	return cat
}

func accessCatalog(c Ctx) adapters.AccessCatalog {
	ctx, cancel := svcCtx()
	defer cancel()
	cat, _ := c.svc().Access.Catalog(ctx)
	return cat
}

func grantedAccess(c Ctx) []mockdata.AccessState {
	ctx, cancel := svcCtx()
	defer cancel()
	st, _ := c.svc().Access.Granted(ctx, c.User.ID)
	return st
}

func calendarEvents(c Ctx) []mockdata.Event {
	ctx, cancel := svcCtx()
	defer cancel()
	ev, _ := c.svc().Calendar.Events(ctx)
	return ev
}

func calendarBusy(c Ctx, userID string) []mockdata.TimeRange {
	ctx, cancel := svcCtx()
	defer cancel()
	b, _ := c.svc().Calendar.Busy(ctx, userID)
	return b
}
