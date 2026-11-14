package tools

import "testing"

// BR-34: владелец по умолчанию — руководитель отдела; он назначает другого, тот получает «Поддержку базы».
func TestKBOwnerChange(t *testing.T) {
	s := newStore(t)
	lead, expert := ctxFor(t, s, "lead.kovalev"), ctxFor(t, s, "expert.orlov")
	if info := KBOwner(lead); info.OwnerID != "u-kovalev" || !info.CanChange || !info.IsOwner {
		t.Fatalf("по умолчанию: %+v", info)
	}
	if _, e := SetKBOwner(expert, expert.User.ID); e == nil || e.Code != "forbidden" {
		t.Fatalf("эксперт сам себя: %+v", e)
	}
	if _, e := SetKBOwner(lead, "u-sokolov"); e == nil || e.Code != "validation" {
		t.Fatalf("стажёр: %+v", e)
	}
	if info, e := SetKBOwner(lead, expert.User.ID); e != nil || info.OwnerID != expert.User.ID || info.IsOwner {
		t.Fatalf("назначение: %+v %+v", info, e)
	}
	if _, e := KBMaintenance(expert); e != nil {
		t.Fatal("новый владелец видит поддержку базы:", e)
	}
	if _, e := KBMaintenance(lead); e == nil {
		t.Fatal("прежний — уже нет")
	}
	if KBOwner(lead).CanChange != true {
		t.Fatal("руководитель может вернуть себе")
	}
	got := 0
	for _, n := range s.NotificationsFor(expert.User.ID) {
		if n.Type == "kb_owner" {
			got++
		}
	}
	if got != 1 {
		t.Fatal("новому владельцу — уведомление")
	}
	if m, e := CheckKBNow(expert); e != nil || m.LastCheck == "" || len(m.Duplicates) < 2 {
		t.Fatalf("проверка сейчас: %+v %+v", m, e)
	}
	if _, e := CheckKBNow(lead); e == nil {
		t.Fatal("проверку запускает владелец")
	}
}
