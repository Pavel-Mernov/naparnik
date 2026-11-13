package adapters

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Опрос статуса заявки: решение из сервиса, сбой сервиса — ErrUnavailable, номер экранируется в пути.
func TestHTTPAbsenceStatus(t *testing.T) {
	down := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			w.WriteHeader(503)
			return
		}
		switch r.URL.EscapedPath() {
		case "/requests/HR-1":
			_, _ = w.Write([]byte(`{"id":"HR-1","status":"rejected","reason_code":"TOO_LATE","comment":"Подай раньше"}`))
		case "/requests/HR%2F2":
			_, _ = w.Write([]byte(`{"status":"approved"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	h := NewHTTPAbsence(srv.URL, "")
	st, err := h.Status(context.Background(), "HR-1")
	if err != nil || st.Status != "rejected" || st.ReasonCode != "TOO_LATE" || st.Comment != "Подай раньше" {
		t.Fatalf("HR-1: %+v %v", st, err)
	}
	if st, err := h.Status(context.Background(), "HR/2"); err != nil || st.ID != "HR/2" || st.Status != "approved" {
		t.Fatalf("HR/2: %+v %v", st, err)
	}
	if _, err := h.Status(context.Background(), "HR-404"); err == nil {
		t.Fatal("404 — ошибка")
	}
	down = true
	if _, err := h.Status(context.Background(), "HR-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("сбой: %v", err)
	}
}
