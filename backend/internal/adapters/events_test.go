package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type memWriter struct {
	mu    sync.Mutex
	got   []ClickEvent
	fails int
}

func (m *memWriter) Write(_ context.Context, b []ClickEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fails > 0 {
		m.fails--
		return errors.New("down")
	}
	m.got = append(m.got, b...)
	return nil
}

func (m *memWriter) n() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.got) }

// BR-73 → аналитика: в коллектор уходят только сохранённые события, без текстов; сбой коллектора не теряет пачку.
func TestEventsExportAfterCommit(t *testing.T) {
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(d, time.Now())
	w := &memWriter{fails: 1}
	exp := NewExporter(w, 100)
	exp.FlushEvery = 10 * time.Millisecond
	store.EventSink = exp.Sink
	defer func() { store.EventSink = nil }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { exp.Run(ctx); close(done) }()

	_ = s.Transaction(func(s *store.Store) error { s.Emit(store.EvKBAnswer, "u-sokolov", "kb-02", nil); return nil })
	_ = s.Transaction(func(s *store.Store) error {
		s.Emit(store.EvStepDone, "u-sokolov", "s-01", nil)
		return errors.New("откат")
	})
	s.Emit(store.EvAnswerFeedback, "u-sokolov", "kb-02", map[string]string{"value": "helped"}) // вне транзакции
	deadline := time.Now().Add(2 * time.Second)
	for w.n() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if w.n() != 2 || w.got[0].Type != store.EvKBAnswer || w.got[1].Source != "naparnik" {
		t.Fatal("откатившееся событие не уходит; после сбоя пачка повторяется", w.got)
	}
	if st := exp.Stats(); st["sent"] != 2 || st["failed_batches"] != 1 {
		t.Fatal(st)
	}
	// Переполнение буфера — событие отбрасывается, транзакция не ждёт.
	small := NewExporter(w, 1)
	small.Sink([]store.LogEvent{{ID: "a"}, {ID: "b"}})
	if small.Stats()["dropped"] != 1 {
		t.Fatal(small.Stats())
	}
}

func TestEventWriters(t *testing.T) {
	var got map[string][]ClickEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	batch := []ClickEvent{{EventID: "EV-1", Type: "kb_answer"}}
	if err := (HTTPCollector{URL: srv.URL, Token: "t"}).Write(context.Background(), batch); err != nil || got["events"][0].EventID != "EV-1" {
		t.Fatal(err, got)
	}
	if err := (HTTPCollector{URL: srv.URL}).Write(context.Background(), batch); err == nil {
		t.Fatal("без токена — ошибка")
	}
	if err := (HTTPCollector{URL: "http://127.0.0.1:1"}).Write(context.Background(), batch); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := (&JSONLines{Out: &buf}).Write(context.Background(), append(batch, batch...)); err != nil || strings.Count(buf.String(), "\n") != 2 {
		t.Fatal(buf.String())
	}
}
