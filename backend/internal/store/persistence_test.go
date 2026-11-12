package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

func TestTransactionRollbackAndCopies(t *testing.T) {
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	s := New(d, time.Now())
	if err := s.Transaction(func(work *Store) error {
		work.AddRequest(&Request{ID: work.NextID("ABS")})
		return errors.New("reject")
	}); err == nil || len(s.Requests("")) != 0 {
		t.Fatal("частичная запись")
	}
	var request *Request
	if err := s.Transaction(func(work *Store) error {
		request = &Request{ID: work.NextID("ABS"), History: []Event{{Text: "initial"}}}
		work.AddRequest(request)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request.History[0].Text = "changed"
	if got := s.Requests(""); got[0].ID != "ABS-1042" || got[0].History[0].Text != "initial" {
		t.Fatal("указатель меняет состояние")
	}
	if s.Close() != nil || s.Refresh(context.Background()) != nil {
		t.Fatal("memory store")
	}
	if err := s.decode([]byte(`{"version":4}`)); err == nil {
		t.Fatal("неизвестная версия")
	}
	if err := s.decode([]byte(`not json`)); err == nil {
		t.Fatal("битый документ")
	}
}

func TestDatabaseFailureDoesNotFallBackToMemory(t *testing.T) {
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, err := OpenPostgres(ctx, d, time.Now(), "postgres://localhost:1/unavailable?sslmode=disable"); err == nil || s != nil {
		t.Fatal("не должно быть незаметного отката в память")
	}
}
