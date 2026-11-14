package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// Живые обновления: после изменения данных пользователя поток присылает «changed» за пару секунд.
func TestStreamSendsChanged(t *testing.T) {
	streamTick = 20 * time.Millisecond
	srv, h := newServer(t)
	ts := httptest.NewServer(h)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/stream", nil)
	req.Header.Set("X-User", "intern.sokolov")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatal(ct)
	}
	lines := bufio.NewScanner(resp.Body)
	var got []string
	for lines.Scan() {
		got = append(got, lines.Text())
		if lines.Text() == "event: ready" {
			srv.Store.Notify(store.Notification{UserID: "u-sokolov", Type: "answer", Text: "Ответ готов"})
		}
		if lines.Text() == "event: changed" {
			return
		}
	}
	t.Fatalf("нет события changed: %v", got)
}

func TestStreamNeedsUser(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "GET", "/api/stream", "", ""); code != 401 {
		t.Fatal(code)
	}
}
