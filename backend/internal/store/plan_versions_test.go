package store

import (
	"encoding/json"
	"testing"
)

// Старое JSONB-состояние без версий планов мигрирует с прогрессом и создаёт исходный снимок истории.
func TestLegacyPlanMigrationAndHistoryCopies(t *testing.T) {
	s := watchStore(t)
	before := s.PlanSteps("u-sokolov")
	snap := s.snapshot()
	for _, p := range snap.Plans {
		p.Version = 0
		p.History = nil
		p.ActiveSteps = nil
		p.ReviewStatus = ""
	}
	raw, e := json.Marshal(snap)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.decode(raw); e != nil {
		t.Fatal(e)
	}
	p, _ := s.Plan("u-sokolov")
	if p.Version != 1 || p.ActiveRevision != 1 || len(p.History) != 1 || len(p.Steps) != len(s.Data.Template.Steps) {
		t.Fatal(p)
	}
	p.History[0].Steps[0].DueDate = "broken"
	p.ActiveSteps[0].DueDate = "broken"
	again, _ := s.Plan("u-sokolov")
	if again.History[0].Steps[0].DueDate == "broken" || again.ActiveSteps[0].DueDate == "broken" {
		t.Fatal("история меняется через копию")
	}
	for i, st := range s.PlanSteps("u-sokolov") {
		if st != before[i] {
			t.Fatal(st, before[i])
		}
	}
	draft, _ := s.Plan("u-egorova")
	if draft.ReviewStatus != PlanReview || draft.ActiveRevision != 0 {
		t.Fatal(draft)
	}
}
