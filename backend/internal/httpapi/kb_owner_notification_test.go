package httpapi

import (
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// BR-34: передача Ольга → Екатерина создаёт личное уведомление, повторное назначение — новое,
// даже в тот же момент времени. Открытие чужого профиля не даёт доступ к его уведомлениям.
func TestKBOwnerNotificationDelivery(t *testing.T) {
	old := store.Now
	defer func() { store.Now = old }()
	fixed, _ := time.Parse(time.RFC3339, "2026-11-15T12:00:00+03:00")
	store.Now = func() time.Time { return fixed }
	_, h := newServer(t)
	assign := func(id string) {
		t.Helper()
		code, body := call(t, h, "PUT", "/api/kb/owner", "lead.kovalev", `{"user_id":"`+id+`"}`)
		if code != 200 {
			t.Fatal(code, body)
		}
	}
	notes := func(user string) []any {
		t.Helper()
		code, body := call(t, h, "GET", "/api/notifications", user, "")
		if code != 200 {
			t.Fatal(code, body)
		}
		list, _ := body["notifications"].([]any)
		return list
	}
	assign("u-smirnova")
	assign("u-belova")
	first := notes("buddy.belova")
	if len(first) != 1 {
		t.Fatal(first)
	}
	n := first[0].(map[string]any)
	if n["type"] != "kb_owner" || n["view"] != "knowledge" || n["read"] != false || !strings.Contains(n["text"].(string), "назначил(а) тебя владельцем") {
		t.Fatal(n)
	}
	if len(notes("expert.orlov")) != 0 || len(notes("lead.kovalev")) != 0 {
		t.Fatal("уведомление видно другой учётке")
	}
	if code, _ := call(t, h, "POST", "/api/notifications/read", "buddy.belova", `{}`); code != 200 {
		t.Fatal(code)
	}
	assign("u-belova")
	if same := notes("buddy.belova"); len(same) != 1 || same[0].(map[string]any)["read"] != true {
		t.Fatal("повтор того же назначения создал уведомление", same)
	}
	assign("u-smirnova")
	assign("u-belova")
	again := notes("buddy.belova")
	if len(again) != 2 {
		t.Fatal("повторное назначение потеряло уведомление", again)
	}
	latest := again[0].(map[string]any)
	if latest["id"] == n["id"] || latest["read"] != false || latest["at"] != n["at"] {
		t.Fatal("новое назначение должно иметь собственный ID даже в ту же секунду", again)
	}
	if len(notes("expert.smirnova")) != 2 {
		t.Fatal("уведомления Ольги должны остаться у Ольги")
	}
}
