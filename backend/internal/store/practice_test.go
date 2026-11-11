package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/practice"
)

// Старое состояние без поля практики читается, копии и откаты не меняют историю пользователя.
func TestPracticeSnapshotCompatibilityAndCopies(t *testing.T) {
	d, e := mockdata.Load("../../../docs/04-mock-data")
	if e != nil {
		t.Fatal(e)
	}
	s := New(d, time.Now())
	s.AddRequest(&Request{ID: "KEEP"})
	raw, _ := json.Marshal(s.snapshot())
	var old map[string]any
	_ = json.Unmarshal(raw, &old)
	delete(old, "practices")
	old["version"] = 1
	raw, _ = json.Marshal(old)
	if e := s.decode(raw); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.Practice("u-sokolov", "pr-01"); ok || len(s.Requests("")) != 1 {
		t.Fatal("старые данные потеряны")
	}
	v := practice.State{InternID: "u-sokolov", TemplateID: "pr-01", Attempts: []practice.Attempt{{Answer: "PRIVATE"}}}
	s.PutPractice(v)
	v.Attempts[0].Answer = "changed"
	got, _ := s.Practice("u-sokolov", "pr-01")
	if got.Attempts[0].Answer != "PRIVATE" {
		t.Fatal(got)
	}
	got.Attempts[0].Answer = "changed again"
	got, _ = s.Practice("u-sokolov", "pr-01")
	if got.Attempts[0].Answer != "PRIVATE" {
		t.Fatal(got)
	}
}
