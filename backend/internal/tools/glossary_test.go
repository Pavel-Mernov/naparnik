package tools

import (
	"strings"
	"testing"
)

// SC-34, BR-88: «Что значит „раскатить на сплит“?» — сплит из глоссария, раскатить — общее определение и предложение добавить.
func TestJargonFromGlossary(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	d := data(t, Run(c, "explain_term", map[string]any{"term": "сплит"}))
	if d["found"] != true || d["label"] != LabelTeamTerm || !strings.Contains(d["definition"].(string), "групп") {
		t.Fatal(d)
	}
	if src := SourcesFor(c, "explain_term", map[string]any{"term": "сплит"}); len(src) != 1 || src[0].PageID != "kb-05" {
		t.Fatal(src)
	}
	d = data(t, Run(c, "explain_term", map[string]any{"term": "раскатить"}))
	if d["found"] != false || d["label"] != LabelGeneralTerm || !strings.Contains(d["note"].(string), "Екатерина Белова") {
		t.Fatal(d)
	}
	for _, src := range SourcesFor(c, "explain_term", map[string]any{"term": "раскатить"}) {
		if src.PageID == "kb-05" {
			t.Fatal("общее определение не ссылается на глоссарий", src)
		}
	}
	// Общий термин из практики — с определением, но с пометкой «общее определение».
	if d := data(t, Run(c, "explain_term", map[string]any{"term": "воронка"})); d["found"] != false || d["definition"] == nil {
		t.Fatal(d)
	}
}

// BR-88: новый термин — черновик у бадди; после его «Ок» термин отвечается из глоссария.
func TestSuggestTermNeedsBuddyOK(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	if e := Check(c, "suggest_term", map[string]any{"term": "сплит", "definition": "x"}); e == nil {
		t.Fatal("сплит уже в глоссарии")
	}
	r := data(t, Run(c, "suggest_term", map[string]any{"term": "раскатить", "definition": "включить изменение для части или всех пользователей"}))
	if r["approver"] != "Екатерина Белова" {
		t.Fatal(r)
	}
	if d := data(t, Run(c, "explain_term", map[string]any{"term": "раскатить"})); d["found"] != false {
		t.Fatal("до «Ок» бадди термина в глоссарии нет")
	}
	if Run(c, "suggest_term", map[string]any{"term": "раскатить", "definition": "ещё раз"}).OK {
		t.Fatal("повторное предложение того же термина")
	}
	if _, e := ApprovePage(c, r["page_id"].(string), "", ""); e == nil {
		t.Fatal("стажёр не публикует за бадди")
	}
	buddy := ctxFor(t, s, "buddy.belova")
	if _, e := ApprovePage(buddy, r["page_id"].(string), "", ""); e != nil {
		t.Fatal(e)
	}
	if d := data(t, Run(c, "explain_term", map[string]any{"term": "раскатить"})); d["found"] != true || d["label"] != LabelTeamTerm {
		t.Fatal(d)
	}
	notified := false
	for _, n := range s.NotificationsFor("u-belova") {
		notified = notified || strings.Contains(n.Text, "раскатить")
	}
	if !notified {
		t.Fatal("бадди не узнала о термине")
	}
}

// BR-87: набор знаний — данные роли; страницы набора — источники ответа.
func TestRoleKnowledge(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	d := data(t, Run(c, "get_role_knowledge", nil))
	if d["role"] != "Стажёр-аналитик" || len(d["code_style"].([]map[string]string)) != 3 || len(d["data_map"].([]map[string]any)) != 4 {
		t.Fatal(d)
	}
	ids := map[string]bool{}
	for _, h := range SourcesFor(c, "get_role_knowledge", nil) {
		ids[h.PageID] = true
	}
	for _, want := range []string{"kb-05", "kb-30", "kb-04"} {
		if !ids[want] {
			t.Fatal("нет источника", want, ids)
		}
	}
	// Другая роль — другой набор, та же логика (BR-87).
	c.User.RoleSet = "backend"
	d = data(t, Run(c, "get_role_knowledge", nil))
	if len(d["service_map"].([]map[string]any)) != 4 || d["code_style"].([]map[string]string)[0]["rule"] == "" {
		t.Fatal(d)
	}
	if e := data(t, Run(c, "explain_term", map[string]any{"term": "ручка"})); e["found"] != true {
		t.Fatal(e)
	}
}
