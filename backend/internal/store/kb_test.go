package store

import (
	"strings"
	"testing"
)

// BR-33: черновик kb-draft-01 (Лебедева, 03.11) — напоминание на 3-й рабочий день, владельцу — на 6-й, по одному разу.
func TestKBDraftReminders(t *testing.T) {
	s := watchStore(t)
	s.Watch(at("2026-11-05 12:00"))
	if len(notes(s, "u-lebedeva", "draft_review")) != 0 {
		t.Fatal("рано напоминать")
	}
	s.Watch(at("2026-11-06 12:00"))
	if n := notes(s, "u-lebedeva", "draft_review"); len(n) != 1 || !strings.Contains(n[0].Text, "3 рабочих дня") {
		t.Fatalf("напоминание автору: %+v", n)
	}
	if len(notes(s, s.KBOwner(), "draft_review")) != 0 {
		t.Fatal("владельцу рано")
	}
	s.Watch(at("2026-11-11 12:00"))
	s.Watch(at("2026-11-12 12:00"))
	if n := notes(s, s.KBOwner(), "draft_review"); len(n) != 1 || !strings.Contains(n[0].Text, "Лебедева") {
		t.Fatalf("владельцу: %+v", n)
	}
	if len(notes(s, "u-lebedeva", "draft_review")) != 1 {
		t.Fatal("автору — одно напоминание")
	}
}

// BR-38: проверка находит обе пары из данных и спорную пару; известные по стартовым данным не дублируются.
func TestKBCheckFindsDuplicates(t *testing.T) {
	s := watchStore(t)
	found := map[string]bool{}
	for _, p := range s.FindDuplicates() {
		found[strings.Join(p.Pages, "|")] = true
	}
	if !found["kb-14|kb-15"] || !found["kb-17|kb-18"] || len(found) > 4 {
		t.Fatalf("пары: %v", found)
	}
	s.Watch(at("2026-11-12 12:00"))
	if len(s.AutoPairs()) != 0 || s.LastKBCheck() != "" {
		t.Fatal("до плановой даты проверки нет")
	}
	s.Watch(at("2027-01-15 11:00"))
	fresh := s.AutoPairs()
	for _, p := range fresh {
		if k := strings.Join(p.Pages, "|"); k == "kb-14|kb-15" || k == "kb-17|kb-18" {
			t.Fatal("пара из стартовых данных предложена второй раз:", k)
		}
	}
	if s.LastKBCheck() != "2027-01-15" || s.NextKBCheck() != "2027-05-15" {
		t.Fatalf("даты: %s → %s", s.LastKBCheck(), s.NextKBCheck())
	}
	if n := notes(s, s.KBOwner(), "kb_check"); len(n) != 1 || !strings.Contains(n[0].Text, "Плановая проверка") {
		t.Fatalf("сводка владельцу: %+v", n)
	}
	s.Watch(at("2027-01-15 12:00"))
	if len(notes(s, s.KBOwner(), "kb_check")) != 1 {
		t.Fatal("проверка — раз в 4 месяца")
	}
}
