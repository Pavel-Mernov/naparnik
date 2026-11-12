package tools

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
)

func ask(t *testing.T, s *store.Store, to, text string) store.Question {
	t.Helper()
	r := Run(ctxFor(t, s, "intern.sokolov"), "send_question", map[string]any{"addressee_id": to, "text": text})
	if !r.OK {
		t.Fatal(r.Error)
	}
	return *r.Data.(*store.Question)
}

// SC-08: «не ко мне → Михаил»; без указания — следующий кандидат без отказавшихся, иначе бадди (BR-17).
func TestNotMine(t *testing.T) {
	s := newStore(t)
	q := ask(t, s, "u-lebedeva", "Как посчитать retention по когортам в нашем дашборде?")
	nat := ctxFor(t, s, "expert.lebedeva")
	if _, e := NotMine(ctxFor(t, s, "expert.orlov"), q.ID, ""); e == nil || e.Code != "forbidden" {
		t.Fatal("только адресат")
	}
	if _, e := NotMine(nat, q.ID, "u-sokolov"); e == nil || e.Code != "validation" {
		t.Fatal("стажёру нельзя")
	}
	out, e := NotMine(nat, q.ID, "u-orlov")
	if e != nil || out.AddresseeID != "u-orlov" || len(out.Route) != 1 || out.Route[0].Kind != "not_mine" || out.Route[0].From != "u-lebedeva" {
		t.Fatalf("не ко мне → Михаил: %+v %v", out, e)
	}
	// Михаил тоже «не ко мне» без указания — Наталья и Михаил исключены, дальше бадди или другой эксперт
	out2, e := NotMine(ctxFor(t, s, "expert.orlov"), q.ID, "")
	if e != nil || out2.AddresseeID == "u-lebedeva" || out2.AddresseeID == "u-orlov" || len(out2.Route) != 2 {
		t.Fatalf("следующий кандидат: %+v %v", out2, e)
	}
	// уведомления: стажёр видит смену адресата, новый адресат — вопрос
	got := map[string]bool{}
	for _, n := range s.NotificationsFor("u-sokolov") {
		if n.Type == "question_rerouted" && strings.Contains(n.Text, "Михаил Орлов") {
			got["intern"] = true
		}
	}
	for _, n := range s.NotificationsFor("u-orlov") {
		if n.Type == "question_new" && strings.Contains(n.Text, "Тебе передали вопрос") {
			got["new"] = true
		}
	}
	if !got["intern"] || !got["new"] {
		t.Fatalf("уведомления: %v", got)
	}
	if _, e := NotMine(nat, "Q-404", ""); e == nil || e.Code != "not_found" {
		t.Fatal("нет вопроса")
	}
}

// SC-09: бадди переадресует из ленты; стажёр и прежний адресат уведомлены (BR-16).
func TestBuddyRedirect(t *testing.T) {
	s := newStore(t)
	q := ask(t, s, "u-lebedeva", "Как устроена разметка категорий объявлений в витрине?")
	if _, e := Redirect(ctxFor(t, s, "expert.orlov"), q.ID, "u-orlov"); e == nil || e.Code != "forbidden" {
		t.Fatal("переадресует только бадди")
	}
	kat := ctxFor(t, s, "buddy.belova")
	if _, e := Redirect(kat, q.ID, "u-lebedeva"); e == nil || e.Code != "validation" {
		t.Fatal("тому же адресату")
	}
	if _, e := Redirect(kat, q.ID, "u-nobody"); e == nil || e.Code != "not_found" {
		t.Fatal("нет человека")
	}
	out, e := Redirect(kat, q.ID, "u-orlov")
	if e != nil || out.AddresseeID != "u-orlov" || out.Route[0].Kind != "redirect" {
		t.Fatalf("%+v %v", out, e)
	}
	if n := s.NotificationsFor("u-lebedeva")[0]; n.Type != "question_taken" {
		t.Fatalf("прежнему адресату: %+v", n)
	}
	// BR-10a: вопрос по теме директора у бадди — «не ко мне» нельзя, переадресовать можно (в т. ч. директору)
	q18 := ask(t, s, "u-belova", "Планируют ли менять North Star раздела или добавлять метрику качества контактов?")
	if _, e := NotMine(kat, q18.ID, ""); e == nil || e.Code != "validation" {
		t.Fatal("у бадди нет «не ко мне» для вопроса по теме директора")
	}
	if out, e := Redirect(kat, q18.ID, "u-volkov"); e != nil || out.AddresseeID != "u-volkov" || out.Hint != "" {
		t.Fatalf("бадди решил — директору: %+v %v", out, e)
	}
	// отвеченный вопрос не переадресуется
	_, _, _ = AnswerQuestion(ctxFor(t, s, "expert.orlov"), q.ID, "ответ")
	if _, e := Redirect(kat, q.ID, "u-smirnova"); e == nil || e.Code != "validation" {
		t.Fatal("после ответа")
	}
}

// SC-22: открытый вопрос назначен Ольге; дубли: «объединить» и «не дубль»; черновики и «устарело» в списке.
func TestKBMaintenance(t *testing.T) {
	s := newStore(t)
	lead := ctxFor(t, s, "lead.kovalev")
	if _, e := KBMaintenance(ctxFor(t, s, "intern.sokolov")); e == nil || e.Code != "forbidden" {
		t.Fatal("только владелец базы")
	}
	s.Emit(store.EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "stale"})
	m, e := KBMaintenance(lead)
	if e != nil || len(m.Open) != 1 || !m.Open[0].Seed || len(m.Duplicates) != 2 || len(m.Drafts) != 1 || len(m.Stale) != 1 || m.Stale[0].Marks != 1 || m.NextCheck == "" {
		t.Fatalf("поддержка базы: %+v %v", m, e)
	}
	// назначить открытый вопрос из стартовых данных Ольге
	q, e := AssignOpen(lead, "q-seed-open", "u-smirnova")
	if e != nil || q.AddresseeID != "u-smirnova" || q.Status != "Sent" {
		t.Fatalf("назначение: %+v %v", q, e)
	}
	if _, e := AssignOpen(lead, "q-seed-open", "u-orlov"); e == nil {
		t.Fatal("повторное назначение")
	}
	if _, e := AssignOpen(ctxFor(t, s, "expert.orlov"), q.ID, "u-orlov"); e == nil || e.Code != "forbidden" {
		t.Fatal("назначает владелец")
	}
	// дубли
	if _, e := DecideDuplicate(lead, "dp-01", "maybe"); e == nil {
		t.Fatal("кривое решение")
	}
	d, e := DecideDuplicate(lead, "dp-01", "merge")
	if e != nil || d.Decision != "merge" {
		t.Fatal(e)
	}
	a, _ := pageByID(lead, "kb-14")
	b, _ := pageByID(lead, "kb-15")
	if a.Title != "Доступ к Redash: роли viewer и editor" || b.Status != "Merged" || b.MergedInto != "kb-14" {
		t.Fatalf("объединение: %+v / %+v", a, b)
	}
	for _, h := range SearchKB(ctxFor(t, s, "intern.sokolov"), "Доступ в Redash для новичков") {
		if h.PageID == "kb-15" {
			t.Fatal("объединённая страница не в поиске")
		}
	}
	if n := s.NotificationsFor("u-lebedeva"); len(n) == 0 || n[0].Type != "pages_merged" {
		t.Fatalf("автору: %+v", n)
	}
	if _, e := DecideDuplicate(lead, "dp-01", "merge"); e == nil {
		t.Fatal("повторное решение")
	}
	if d, e := DecideDuplicate(lead, "dp-02", "not_duplicate"); e != nil || d.Decision != "not_duplicate" {
		t.Fatal(e)
	}
	if _, e := DecideDuplicate(lead, "dp-99", "merge"); e == nil || e.Code != "not_found" {
		t.Fatal("нет пары")
	}
	if _, e := DecideDuplicate(ctxFor(t, s, "expert.orlov"), "dp-02", "merge"); e == nil || e.Code != "forbidden" {
		t.Fatal("не владелец")
	}
	// напоминание автору черновика
	if e := RemindDraft(lead, "kb-draft-01"); e != nil {
		t.Fatal(e)
	}
	if n := s.NotificationsFor("u-lebedeva")[0]; n.Type != "draft_review" {
		t.Fatalf("напоминание: %+v", n)
	}
	if e := RemindDraft(lead, "kb-404"); e == nil {
		t.Fatal("нет черновика")
	}
	if e := RemindDraft(ctxFor(t, s, "expert.orlov"), "kb-draft-01"); e == nil {
		t.Fatal("не владелец")
	}
}

// Пара без предложенного заголовка: объединение не стирает заголовок первой страницы.
func TestMergeKeepsTitle(t *testing.T) {
	s := newStore(t)
	lead := ctxFor(t, s, "lead.kovalev")
	m, _ := KBMaintenance(lead)
	if m.Duplicates[1].Note == "" || m.Duplicates[1].ProposedTitle != "" {
		t.Fatalf("причина «не дубль»: %+v", m.Duplicates[1])
	}
	before, _ := pageByID(lead, "kb-17")
	if _, e := DecideDuplicate(lead, "dp-02", "merge"); e != nil {
		t.Fatal(e)
	}
	if after, _ := pageByID(lead, "kb-17"); after.Title != before.Title {
		t.Fatal(after.Title)
	}
}

// Модель дописала к вопросу свои слова — подбор держится за слова стажёра (evals e-23, e-24).
func TestFindExpertPrefersInternWords(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	c.Question = "Как исключить ботов из выборки эксперимента?"
	d := data(t, Run(c, "find_expert", map[string]any{"question": "Как исключить ботов? Какие правила фильтрации ботов используем в продуктовой аналитике и витринах?"}))
	if d["addressee_id"] != "u-smirnova" {
		t.Fatal(d)
	}
	c.Question = "Как посчитать retention по когортам?" // evals e-21: расширенный вопрос модели уводил к Орлову
	d = data(t, Run(c, "find_expert", map[string]any{"question": "Как посчитать retention по когортам? Нужны правила расчёта: как формируются когорты, окно наблюдения, какие события считаются возвратом, где смотреть данные."}))
	if d["addressee_id"] != "u-lebedeva" {
		t.Fatal(d)
	}
}

// Модель расширила запрос к базе — страница по словам стажёра всё равно найдена (evals e-09, e-17).
func TestSearchKBUsesInternWords(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	c.Question = "Что писать в еженедельном отчёте?"
	hits := data(t, Run(c, "search_kb", map[string]any{"query": "еженедельный отчёт стажёра что писать"}))["hits"].([]Hit)
	if len(hits) == 0 || hits[0].PageID != "kb-12" {
		t.Fatal(hits)
	}
}

// «A/B-тест» и «retention» не теряются при разборе слов (evals e-19, e-21).
func TestExpertForABAndRetention(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	for q, want := range map[string]string{"Сколько держать A/B-тест при маленьком трафике?": "u-smirnova", "Как посчитать retention по когортам?": "u-lebedeva"} {
		if id, conf, _ := FindExpert(c, q); id != want || conf < ExpertThreshold {
			t.Fatal(q, id, conf)
		}
	}
}
