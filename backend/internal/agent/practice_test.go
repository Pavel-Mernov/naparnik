package agent

import (
	"strings"
	"testing"
	"time"
)

// SC-30/32: чат предлагает ту же практику и уважает выбор «сразу к задаче», без отправок людям.
func TestPracticeChatOfferAndSkip(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "По плану у меня дашборд конверсии карточки. С чего начать?")
	if len(r.Drafts) != 0 || !strings.Contains(r.Text, "Практика перед задачей") || len(a.Store.Questions()) != 0 {
		t.Fatal(r)
	}
	r = ask(t, a, "intern.sokolov", "давай сразу к задаче")
	st, ok := a.Store.Practice("u-sokolov", "pr-01")
	if !ok || st.Stage != "real_task" || !strings.Contains(r.Text, "чек") && !strings.Contains(r.Text, "Возьми формулу") {
		t.Fatal(st, r)
	}
	// Тема практики в причине доступа не должна перехватить готовый сценарий SC-33.
	r = ask(t, a, "intern.sokolov", "Как попасть в витрины DWH для практики?")
	if !strings.Contains(r.Text, "Витрины DWH отдела") {
		t.Fatal(r)
	}
}
