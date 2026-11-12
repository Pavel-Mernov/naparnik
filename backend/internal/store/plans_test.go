package store

import (
	"encoding/json"
	"testing"
	"time"
)

// BR-81: состояние плана переживает перезапуск; старый снимок без планов берёт их из трекера.
func TestPlanStateSnapshot(t *testing.T) {
	s := watchStore(t)
	s.UpdatePlan("u-egorova", func(p *PlanState) { p.Comment = "ok"; p.Steps[0].Removed = true })
	raw, _ := json.Marshal(s.snapshot())
	r := watchStore(t)
	if err := r.decode(raw); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.Plan("u-egorova"); p.Comment != "ok" || !p.Steps[0].Removed || p.Status != PlanDraft {
		t.Fatal(p)
	}
	old := s.snapshot()
	old.Plans = nil
	raw, _ = json.Marshal(old)
	if err := r.decode(raw); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.Plan("u-egorova"); p.Comment != "" || p.Status != PlanDraft || len(p.Steps) == 0 {
		t.Fatal("из трекера", p)
	}
	if p, _ := r.Plan("u-sokolov"); p.Status != PlanApproved {
		t.Fatal(p)
	}
	// Простой не считается по черновику плана.
	r.Watch(time.Date(2027, 1, 20, 12, 0, 0, 0, MSK))
	for _, n := range r.NotificationsFor("u-belova") {
		if n.Type == "idle_signal" && n.ID[:13] == "idle:u-egorov" {
			t.Fatal("простой по неутверждённому черновику")
		}
	}
}
