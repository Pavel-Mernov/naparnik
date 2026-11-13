package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type fakeStatus map[string]adapters.AbsenceStatus

func (f fakeStatus) Status(_ context.Context, id string) (adapters.AbsenceStatus, error) {
	if st, ok := f[id]; ok {
		return st, nil
	}
	return adapters.AbsenceStatus{}, adapters.ErrUnavailable
}

func absenceFor(t *testing.T, s *store.Store, date string) string {
	t.Helper()
	res := Run(ctxFor(t, s, "intern.sokolov"), "create_absence_request", map[string]any{"type": "day_off", "date": date})
	if !res.OK {
		t.Fatalf("заявка: %+v", res.Error)
	}
	return res.Data.(*store.Request).ID
}

// Решение согласующего в настоящем сервисе отсутствий доходит до стажёра тем же путём, что и в интерфейсе.
func TestSyncAbsence(t *testing.T) {
	s := newStore(t)
	approved, rejected, other, waiting, broken := absenceFor(t, s, "2026-11-16"), absenceFor(t, s, "2026-11-17"),
		absenceFor(t, s, "2026-11-18"), absenceFor(t, s, "2026-11-19"), absenceFor(t, s, "2026-11-20")
	src := fakeStatus{
		approved: {Status: "approved", Comment: "Хорошего отдыха"},
		rejected: {Status: "rejected", ReasonCode: "TOO_LATE"},
		other:    {Status: "rejected", ReasonCode: "HR_POLICY_42"},
		waiting:  {Status: "pending"},
		// broken — сервис не ответил: заявка не трогается
	}
	_ = broken
	if n := SyncAbsence(context.Background(), s, src); n != 3 {
		t.Fatalf("изменилось %d, ждали 3", n)
	}
	got := map[string]store.Request{}
	for _, r := range s.Requests("") {
		got[r.ID] = r
	}
	if r := got[approved]; r.Status != "approved" || r.Decision != "Хорошего отдыха" || !strings.Contains(r.History[len(r.History)-1].Text, "Согласовано") {
		t.Fatalf("согласовано: %+v", r)
	}
	if r := got[rejected]; r.Status != "rejected" || r.ReasonCode != "TOO_LATE" {
		t.Fatalf("отклонено: %+v", r)
	}
	if r := got[other]; r.Status != "rejected" || r.ReasonCode != "OTHER" || r.Decision != "Отклонено в сервисе отсутствий" {
		t.Fatalf("неизвестный код причины: %+v", r)
	}
	if got[waiting].Status != "pending" || got[broken].Status != "pending" {
		t.Fatal("ждущие и недоступные заявки не меняются")
	}
	notified := false
	for _, n := range s.NotificationsFor("u-sokolov") {
		notified = notified || strings.Contains(strings.ToLower(n.Text), "согласован")
	}
	if !notified {
		t.Fatal("стажёр не получил уведомление о решении")
	}
	// Повторный опрос ничего не меняет: решения уже применены.
	if n := SyncAbsence(context.Background(), s, src); n != 0 {
		t.Fatalf("повтор: %d", n)
	}
}

func TestRunAbsenceSyncStops(t *testing.T) {
	s := newStore(t)
	id := absenceFor(t, s, "2026-11-16")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunAbsenceSync(ctx, s, fakeStatus{id: {Status: "approved"}}, 10*time.Millisecond); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r := s.Requests("u-sokolov"); len(r) > 0 && r[0].Status == "approved" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("опрос не остановился")
	}
	if r := s.Requests("u-sokolov"); r[0].Status != "approved" {
		t.Fatalf("опрос не применил решение: %+v", r[0])
	}
}
