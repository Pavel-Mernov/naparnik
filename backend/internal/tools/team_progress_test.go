package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// team_progress: руководитель и бадди видят своих стажёров — отстающие первыми, без текстов вопросов;
// стажёру инструмент не выдаётся.
func TestTeamProgress(t *testing.T) {
	s := newStore(t)
	for _, login := range []string{"lead.kovalev", "buddy.belova"} {
		c := ctxFor(t, s, login)
		res := teamProgress(c, nil)
		data := res.Data.(map[string]any)
		interns := data["interns"].([]internOverview)
		if !res.OK || len(interns) == 0 {
			t.Fatalf("%s: %+v", login, res)
		}
		for i := 1; i < len(interns); i++ {
			if interns[i].OverdueSteps > interns[i-1].OverdueSteps {
				t.Fatalf("%s: отстающие не первыми: %+v", login, interns)
			}
		}
		raw, _ := json.Marshal(res)
		for _, q := range s.Questions() {
			if q.Text != "" && strings.Contains(string(raw), q.Text) {
				t.Fatalf("%s: текст вопроса в ответе", login)
			}
		}
	}
	if tool, _ := ByName("team_progress"); Allowed(tool, ctxFor(t, s, "intern.sokolov").User) {
		t.Fatal("стажёру team_progress не положен")
	}
}
