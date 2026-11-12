package tools

import (
	"context"
	"github.com/worker4556/naparnik/backend/internal/flags"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type stubAbsence struct {
	adapters.AbsenceService
	rc  adapters.Receipt
	err error
	got adapters.AbsenceSubmit
}

func (s *stubAbsence) Submit(_ context.Context, r adapters.AbsenceSubmit) (adapters.Receipt, error) {
	s.got = r
	return s.rc, s.err
}

// BR-74: настоящий сервис подключается реализацией интерфейса; права, квитанция и сбой — те же.
func TestToolsUseConnectedService(t *testing.T) {
	old := Services
	defer func() { Services = old }()
	st := newStore(t)
	stub := &stubAbsence{rc: adapters.Receipt{ID: "HR-777", Status: "pending"}}
	Services = func(s *store.Store) adapters.Set {
		set := adapters.NewMockSet(s.Data, s.NextID)
		stub.AbsenceService = set.Absence // справочник — из мока
		set.Absence = stub
		return set
	}
	c := ctxFor(t, st, "intern.sokolov")
	r := Run(c, "create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-20"})
	if !r.OK || r.Receipt.(map[string]string)["id"] != "HR-777" || stub.got.ApproverID != "u-kovalev" || stub.got.Key == "" {
		t.Fatal(r, stub.got)
	}
	stub.err = adapters.ErrUnavailable
	if r := Run(c, "create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-23"}); r.OK || r.Error.Code != "unavailable" {
		t.Fatal(r)
	}
	if len(st.Requests("u-sokolov")) != 1 {
		t.Fatal("при сбое заявка не создаётся")
	}
}

// Выключенная флагом функция не видна ни модели, ни MCP, и не выполняется.
func TestToolsRespectFlags(t *testing.T) {
	t.Setenv("FEATURE_FLAGS", "glossary=off,meetings=off")
	old := flags.Default
	flags.Default = flags.FromEnv()
	defer func() { flags.Default = old }()
	st := newStore(t)
	c := ctxFor(t, st, "intern.sokolov")
	for _, tl := range For(c.User) {
		if tl.Name == "explain_term" || tl.Name == "get_role_knowledge" {
			t.Fatal("выключенный инструмент передан модели", tl.Name)
		}
	}
	if r := Run(c, "explain_term", map[string]any{"term": "сплит"}); r.OK {
		t.Fatal("выключенный инструмент выполнился")
	}
	if !Run(c, "search_kb", map[string]any{"query": "отгул"}).OK {
		t.Fatal("остальное работает")
	}
	if _, e := MeetingSlots(c, "Q-1"); e == nil || e.Code != "forbidden" {
		t.Fatal(e)
	}
}
