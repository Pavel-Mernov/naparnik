package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// Фейковый настоящий сервис отсутствий: справочник, заявки с ключом идемпотентности, авторизация, сбои.
type fakeAbsence struct {
	mu       sync.Mutex
	byKey    map[string]Receipt
	n        int
	down     bool
	catalogs int
}

func (f *fakeAbsence) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(401)
			return
		}
		if f.down {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/catalog":
			f.catalogs++
			_ = json.NewEncoder(w).Encode(AbsenceCatalog{RequestTypes: []mockdata.RequestType{{Type: "day_off", Title: "Отгул"}}})
		case "/requests":
			key := r.Header.Get("Idempotency-Key")
			if rc, ok := f.byKey[key]; ok {
				_ = json.NewEncoder(w).Encode(rc)
				return
			}
			var in AbsenceSubmit
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Type == "" || in.EmployeeID == "" {
				w.WriteHeader(422)
				return
			}
			f.n++
			rc := Receipt{ID: "HR-" + string(rune('0'+f.n)), Status: "pending"}
			f.byKey[key] = rc
			_ = json.NewEncoder(w).Encode(rc)
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestHTTPAbsence(t *testing.T) {
	f := &fakeAbsence{byKey: map[string]Receipt{}}
	srv := f.server(t)
	defer srv.Close()
	h := NewHTTPAbsence(srv.URL, "secret")
	ctx := context.Background()

	cat, err := h.Catalog(ctx)
	if err != nil || len(cat.RequestTypes) != 1 {
		t.Fatal(cat, err)
	}
	_, _ = h.Catalog(ctx)
	if f.catalogs != 1 {
		t.Fatal("справочник кэшируется", f.catalogs)
	}
	req := AbsenceSubmit{Key: "k1", EmployeeID: "u-sokolov", ApproverID: "u-kovalev", Type: "day_off", Date: "2026-11-20"}
	rc, err := h.Submit(ctx, req)
	if err != nil || rc.ID != "HR-1" || rc.Status != "pending" {
		t.Fatal(rc, err)
	}
	if again, _ := h.Submit(ctx, req); again.ID != rc.ID {
		t.Fatal("повтор с тем же ключом — та же квитанция", again)
	}
	if _, err := h.Submit(ctx, AbsenceSubmit{Key: "k2"}); err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatal("ошибка данных — не «недоступен»", err)
	}
	// Сервис лёг: заявка — ErrUnavailable, справочник — последний известный.
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	h.CatalogTTL = 0
	if _, err := h.Submit(ctx, req); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if cat, err := h.Catalog(ctx); err != nil || len(cat.RequestTypes) != 1 {
		t.Fatal("последний известный справочник", err)
	}
	if _, err := NewHTTPAbsence("http://127.0.0.1:1", "secret").Submit(ctx, req); !errors.Is(err, ErrUnavailable) {
		t.Fatal("нет сети — недоступен", err)
	}
	if _, err := NewHTTPAbsence(srv.URL, "bad").Catalog(ctx); err == nil {
		t.Fatal("без токена — ошибка")
	}
}

func TestMockSet(t *testing.T) {
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	set := NewMockSet(d, func(p string) string { n++; return p + "-X" })
	ctx := context.Background()
	if c, _ := set.Absence.Catalog(ctx); len(c.RequestTypes) == 0 {
		t.Fatal("типы отсутствий")
	}
	if c, _ := set.Access.Catalog(ctx); len(c.Systems) == 0 {
		t.Fatal("системы")
	}
	if rc, _ := set.Access.Submit(ctx, AccessSubmit{}); rc.ID != "ACC-X" {
		t.Fatal(rc)
	}
	if ev, _ := set.Calendar.Events(ctx); len(ev) == 0 {
		t.Fatal("события")
	}
	_, _ = set.Calendar.Busy(ctx, "u-orlov")
	_, _ = set.Access.Granted(ctx, "u-sokolov")
	t.Setenv("MOCK_FAIL", "absence,access")
	if _, err := set.Absence.Submit(ctx, AbsenceSubmit{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := set.Access.Submit(ctx, AccessSubmit{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	_ = time.Second
}
