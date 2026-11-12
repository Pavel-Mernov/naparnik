// Package adapters — граница между Напарником и корпоративными сервисами Авито (BR-74, 11-INTEGRATION).
//
// Агент и инструменты работают только с интерфейсами ниже. Сейчас за ними — имитации на мок-данных
// (Mock) и HTTP-клиент сервиса отсутствий (HTTPAbsence). Подключить настоящий сервис = написать ещё
// одну реализацию интерфейса и выбрать её в конфигурации (app.Services); права, черновики,
// подтверждение и квитанции (BR-40, BR-43) остаются в инструментах и не меняются.
package adapters

import (
	"context"
	"errors"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// ErrUnavailable — сервис не ответил или вернул сбой: черновик сохраняется, стажёр может повторить (SC-15).
var ErrUnavailable = errors.New("service unavailable")

// Receipt — квитанция сервиса: номер заявки в его системе и статус.
type Receipt struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// AbsenceSubmit — заявка на отсутствие. Причина отсутствия и диагноз сюда не попадают (BR-61).
type AbsenceSubmit struct {
	Key        string `json:"idempotency_key"` // повтор с тем же ключом возвращает ту же квитанцию
	EmployeeID string `json:"employee_id"`
	ApproverID string `json:"approver_id"`
	Type       string `json:"type"`
	Date       string `json:"date"`
	Comment    string `json:"comment,omitempty"`
}

type AbsenceCatalog struct {
	RequestTypes []mockdata.RequestType `json:"request_types"`
	SelfService  mockdata.SelfService   `json:"self_service"`
	ReasonCodes  []mockdata.ReasonCode  `json:"reason_codes"`
}

// AbsenceService — сервис отсутствий (в Авито — система отсутствий из кейса).
type AbsenceService interface {
	Catalog(ctx context.Context) (AbsenceCatalog, error)
	Submit(ctx context.Context, r AbsenceSubmit) (Receipt, error)
}

type AccessSubmit struct {
	Key        string `json:"idempotency_key"`
	EmployeeID string `json:"employee_id"`
	ApproverID string `json:"approver_id"`
	System     string `json:"system"`
	Reason     string `json:"reason"`
}

type AccessCatalog struct {
	Systems     []mockdata.System     `json:"systems"`
	ReasonCodes []mockdata.ReasonCode `json:"reason_codes"`
}

// AccessService — сервис доступов (заявки в стиле Jira).
type AccessService interface {
	Catalog(ctx context.Context) (AccessCatalog, error)
	// Granted — что у сотрудника уже выдано в самой системе доступов.
	Granted(ctx context.Context, userID string) ([]mockdata.AccessState, error)
	Submit(ctx context.Context, r AccessSubmit) (Receipt, error)
}

// Calendar — календарь: события и занятость людей (встречи по вопросам, BR-41; предупреждения отгула).
type Calendar interface {
	Events(ctx context.Context) ([]mockdata.Event, error)
	Busy(ctx context.Context, userID string) ([]mockdata.TimeRange, error)
}

// KnowledgeBase — страницы базы знаний команды (в Авито — вики). Реализация — store.Store.
type KnowledgeBase interface {
	Pages() []mockdata.Page
	AddPage(p mockdata.Page)
	UpdatePage(id string, fn func(*mockdata.Page)) (mockdata.Page, bool)
}

// Tracker — план адаптации как эпик и шаги как задачи (в Авито — трекер). Реализация — store.Store.
type Tracker interface {
	PlanSteps(internID string) []mockdata.TrackerStep
	SetStepStatus(internID, stepID, status string) bool
	TemplateStep(id string) (mockdata.TemplateStep, bool)
}

// Set — все подключённые сервисы.
type Set struct {
	Absence  AbsenceService
	Access   AccessService
	Calendar Calendar
}
