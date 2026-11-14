package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// Приёмка BR-49: прогретый кэш не переживает смену грифа или правку текста той же длины.
func TestClassificationChangeInvalidatesCache(t *testing.T) {
	old := KBCache
	KBCache = cache.NewMemory()
	defer func() { KBCache = old }()
	s := newStore(t)
	s.UpdatePage("kb-04", func(p *mockdata.Page) {
		p.Title = "Canary"
		p.Body = "Canary secret-one"
		p.Classification = mockdata.ClassInternal
	})
	c := ctxFor(t, s, "intern.sokolov")
	query := map[string]any{"query": "Canary"}
	Run(c, "search_kb", query)
	s.UpdatePage("kb-04", func(p *mockdata.Page) { p.Body = "Canary secret-two" })
	raw, _ := json.Marshal(Run(c, "search_kb", query))
	if !strings.Contains(string(raw), "secret-two") || strings.Contains(string(raw), "secret-one") {
		t.Fatal(string(raw))
	}
	s.UpdatePage("kb-04", func(p *mockdata.Page) { p.Classification = mockdata.ClassConfidential })
	raw, _ = json.Marshal(Run(c, "search_kb", query))
	if strings.Contains(string(raw), "secret-two") || !strings.Contains(string(raw), ConfidentialLabel) {
		t.Fatal(string(raw))
	}
}

// secret — фраза из текста kb-41, которой не должно быть ни в одном результате для внешней модели.
const secret = "пилот подписки"

// BR-49: конфиденциальная страница находится поиском, но во внешнюю модель уходит без текста.
func TestConfidentialPageHiddenFromModel(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	c.Question = "Какой план запусков раздела объявлений на квартал?"
	res := Run(c, "search_kb", map[string]any{"query": c.Question})
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("текст конфиденциальной страницы ушёл бы модели: %s", raw)
	}
	hits := res.Data.(map[string]any)["hits"].([]Hit)
	if len(hits) == 0 || hits[0].PageID != "kb-41" || hits[0].Label != ConfidentialLabel || hits[0].Body != ConfidentialBody {
		t.Fatalf("выдача: %+v", hits)
	}
}

// BR-49: человек с правами на пространство видит страницу целиком — гриф ограничивает только модель.
func TestConfidentialPageVisibleToPeople(t *testing.T) {
	_, pages := KBPages(ctxFor(t, newStore(t), "intern.sokolov"))
	for _, p := range pages {
		if p.ID == "kb-41" {
			if p.Classification != mockdata.ClassConfidential || !strings.Contains(p.Body, secret) {
				t.Fatalf("карточка: %+v", p)
			}
		} else if p.Classification != mockdata.ClassInternal {
			t.Errorf("%s: гриф по умолчанию %q", p.ID, p.Classification)
		}
	}
}

// BR-49: конфиденциальная страница не идёт в глоссарий — её текст ушёл бы модели в explain_term.
func TestConfidentialNotInGlossary(t *testing.T) {
	s := newStore(t)
	s.UpdatePage("kb-05", func(p *mockdata.Page) { p.Classification = mockdata.ClassConfidential })
	c := ctxFor(t, s, "intern.sokolov")
	for _, e := range glossary(c) {
		if e.Page != nil && e.Page.ID == "kb-05" {
			t.Fatalf("термин %q из конфиденциальной страницы", e.Term)
		}
	}
}

// BR-49: страница из ответа наследует самый закрытый гриф среди источников.
func TestClassificationInherited(t *testing.T) {
	if mockdata.StricterClass("", mockdata.ClassConfidential) != mockdata.ClassConfidential ||
		mockdata.StricterClass(mockdata.ClassInternal, "") != mockdata.ClassInternal ||
		mockdata.StricterClass(mockdata.ClassConfidential, mockdata.ClassInternal) != mockdata.ClassConfidential {
		t.Fatal("StricterClass")
	}
	c := ctxFor(t, newStore(t), "expert.orlov")
	if got := inheritedClass(c, "kb-41"); got != mockdata.ClassConfidential {
		t.Fatalf("ответ рядом с kb-41: %q", got)
	}
	if got := inheritedClass(c, "kb-04"); got != "" {
		t.Fatalf("ответ рядом с kb-04: %q", got)
	}
}
