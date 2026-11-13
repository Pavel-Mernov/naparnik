package tools

import (
	"strings"
	"testing"
)

// SC-02, US-06: карта команды — бадди первым, без стажёров и себя, страницы только читаемые (BR-40),
// черновик сообщения для копирования (BR-62).
func TestTeamMap(t *testing.T) {
	s := newStore(t)
	cards := TeamMap(ctxFor(t, s, "intern.sokolov"))
	if len(cards) == 0 || cards[0].ID != "u-belova" || !cards[0].MyBuddy {
		t.Fatalf("бадди не первым: %+v", cards)
	}
	if !strings.Contains(cards[0].Message, "Ты мой бадди") {
		t.Fatalf("сообщение бадди: %s", cards[0].Message)
	}
	readable := map[string]bool{}
	for _, p := range readablePages(ctxFor(t, s, "intern.sokolov")) {
		readable[p.ID] = true
	}
	var orlov *TeamCard
	for i, c := range cards {
		if c.ID == "u-volkov" && (!c.ViaBuddy || c.Message != "") {
			t.Fatalf("директору стажёр не пишет напрямую (BR-10a): %+v", c)
		}
		if c.ID == "u-sokolov" || c.Status == "Стажёр" {
			t.Fatalf("в карте стажёр или сам человек: %+v", c)
		}
		if len(c.Pages) > maxTeamPages {
			t.Fatalf("%s: страниц больше %d", c.Name, maxTeamPages)
		}
		for _, p := range c.Pages {
			if !readable[p.ID] {
				t.Fatalf("%s: недоступная страница %s", c.Name, p.ID)
			}
		}
		if c.ID == "u-orlov" {
			orlov = &cards[i]
		}
	}
	if orlov == nil || len(orlov.Pages) == 0 || orlov.Responsibility == "" ||
		!strings.HasPrefix(orlov.Message, "Привет, Михаил! Я Даниил Соколов") || !strings.Contains(orlov.Message, "отвечаешь за") {
		t.Fatalf("Михаил Орлов: %+v", orlov)
	}
}

func TestTeamMapMessages(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	if m := firstMessage(c, "Ольга Смирнова", "", false); !strings.Contains(m, "Хочу познакомиться") {
		t.Fatal(m)
	}
	if lowerFirst("A/B-платформа") != "A/B-платформа" || lowerFirst("Витрины") != "витрины" {
		t.Fatal("lowerFirst")
	}
}
