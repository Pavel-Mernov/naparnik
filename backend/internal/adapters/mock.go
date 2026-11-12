package adapters

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// Mock — имитации сервисов на docs/04-mock-data. NextID — нумерация квитанций (ABS-…, ACC-…) из хранилища,
// чтобы номера переживали перезапуск. MOCK_FAIL=absence,access — сервис «упал» (SC-15).
type Mock struct {
	Data   *mockdata.Data
	NextID func(prefix string) string
}

func mockFail(service string) bool {
	return slices.Contains(strings.Split(os.Getenv("MOCK_FAIL"), ","), service)
}

// NewMockSet — все сервисы на мок-данных.
func NewMockSet(d *mockdata.Data, nextID func(string) string) Set {
	m := Mock{d, nextID}
	return Set{Absence: MockAbsence{m}, Access: MockAccess{m}, Calendar: MockCalendar{m}}
}

type MockAbsence struct{ Mock }

func (m MockAbsence) Catalog(context.Context) (AbsenceCatalog, error) {
	a := m.Data.Absence
	return AbsenceCatalog{RequestTypes: a.RequestTypes, SelfService: a.SelfService, ReasonCodes: a.ReasonCodes}, nil
}

func (m MockAbsence) Submit(context.Context, AbsenceSubmit) (Receipt, error) {
	if mockFail("absence") {
		return Receipt{}, ErrUnavailable
	}
	return Receipt{ID: m.NextID("ABS"), Status: "pending"}, nil
}

type MockAccess struct{ Mock }

func (m MockAccess) Catalog(context.Context) (AccessCatalog, error) {
	return AccessCatalog{Systems: m.Data.Access.Systems, ReasonCodes: m.Data.Access.ReasonCodes}, nil
}

func (m MockAccess) Granted(_ context.Context, userID string) ([]mockdata.AccessState, error) {
	return m.Data.Access.CurrentState[userID], nil
}

func (m MockAccess) Submit(context.Context, AccessSubmit) (Receipt, error) {
	if mockFail("access") {
		return Receipt{}, ErrUnavailable
	}
	return Receipt{ID: m.NextID("ACC"), Status: "pending"}, nil
}

type MockCalendar struct{ Mock }

func (m MockCalendar) Events(context.Context) ([]mockdata.Event, error) {
	return m.Data.Calendar.Events, nil
}

func (m MockCalendar) Busy(_ context.Context, userID string) ([]mockdata.TimeRange, error) {
	return m.Data.Calendar.Busy[userID], nil
}
