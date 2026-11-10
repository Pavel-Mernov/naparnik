package tools

import (
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// Ответ Михаила на Q2 из docs/04-mock-data/expert-answers.json.
const orlovAnswerQ2 = "Дашборд в Redash берёт данные из ads_mart, а в ads_mart просмотры считаются без ботов и только по активным объявлениям. В сырой выгрузке боты и снятые объявления остаются, поэтому конверсия ниже примерно на 8–12%. Сравнивай с той же фильтрацией: is_bot = false и status = 'active'."

func askQ2(t *testing.T, s *store.Store) string {
	t.Helper()
	r := Run(ctxFor(t, s, "intern.sokolov"), "send_question", map[string]any{
		"text": "Почему конверсия в дашборде отличается от выгрузки из DWH?", "addressee_id": "u-orlov"})
	if !r.OK {
		t.Fatal(r.Error)
	}
	return r.Data.(*store.Question).ID
}

// SC-04 → SC-12: ответ эксперта → черновик → «Ок» → второй стажёр получает ответ из базы.
func TestAnswerToPageToKnowledgeEffect(t *testing.T) {
	s := newStore(t)
	qid := askQ2(t, s)
	orlov := ctxFor(t, s, "expert.orlov")

	q, page, e := AnswerQuestion(orlov, qid, orlovAnswerQ2)
	if e != nil {
		t.Fatal(e)
	}
	// US-10: стажёр получает ответ сразу, не дожидаясь публикации
	if q.Status != "Answered" || q.Answer == "" || q.PageID != page.ID {
		t.Fatalf("вопрос: %+v", q)
	}
	// BR-31, BR-30: черновик «Вопрос-ответ», автор — человек, номер как в мок-данных
	if page.ID != "kb-20" || page.Type != "qa" || page.Status != "Draft" || page.AuthorID != "u-orlov" || page.Folder == "" {
		t.Fatalf("черновик: %+v", page)
	}
	// BR-02: черновик не виден в поиске
	polina := ctxFor(t, s, "intern.egorova")
	for _, h := range SearchKB(polina, "Почему конверсия в дашборде не совпадает с DWH?") {
		if h.PageID == "kb-20" {
			t.Fatal("черновик попал в выдачу")
		}
	}
	if drafts := DraftsFor(orlov); len(drafts) != 1 || drafts[0].ID != "kb-20" {
		t.Fatalf("черновики Михаила: %+v", drafts)
	}
	if len(DraftsFor(ctxFor(t, s, "expert.smirnova"))) != 0 {
		t.Fatal("чужие черновики не видны")
	}

	// BR-30: публикует только автор
	if _, e := ApprovePage(ctxFor(t, s, "buddy.belova"), "kb-20", "", ""); e == nil || e.Code != "forbidden" {
		t.Fatalf("публикация не автором: %+v", e)
	}
	pub, e := ApprovePage(orlov, "kb-20", "", "")
	if e != nil || pub.Status != "Published" {
		t.Fatalf("публикация: %+v %+v", pub, e)
	}
	if _, e := ApprovePage(orlov, "kb-20", "", ""); e != nil {
		t.Fatal("повторная публикация")
	}

	// SC-12, US-13: Полина через месяц — ответ из базы, автор Михаил
	hits := SearchKB(polina, "Почему конверсия в дашборде не совпадает с DWH?")
	if len(hits) == 0 || hits[0].PageID != "kb-20" || hits[0].Author != "Михаил Орлов" || !strings.Contains(hits[0].Body, "is_bot") {
		t.Fatalf("эффект базы: %+v", hits)
	}
}

// SC-07: бадди отвечает и правит черновик перед публикацией.
func TestBuddyAnswerWithEdits(t *testing.T) {
	s := newStore(t)
	r := Run(ctxFor(t, s, "intern.sokolov"), "send_question", map[string]any{"text": "Где взять шаблон презентации для экватора?", "addressee_id": "u-belova"})
	belova := ctxFor(t, s, "buddy.belova")
	_, page, e := AnswerQuestion(belova, r.Data.(*store.Question).ID, "Шаблон даёт куратор ближе к сроку.")
	if e != nil {
		t.Fatal(e)
	}
	pub, e := ApprovePage(belova, page.ID, "Шаблон презентации для экватора", "Шаблон присылает куратор за месяц до экватора.")
	if e != nil || pub.Title != "Шаблон презентации для экватора" || !strings.Contains(pub.Body, "за месяц") {
		t.Fatalf("правки: %+v %+v", pub, e)
	}
}

func TestAnswerRules(t *testing.T) {
	s := newStore(t)
	qid := askQ2(t, s)
	cases := []struct {
		login, id, text, code string
	}{
		{"expert.smirnova", qid, "ответ", "forbidden"},  // не адресат
		{"expert.orlov", qid, "  ", "validation"},       // пустой ответ
		{"expert.orlov", "Q-999", "ответ", "not_found"}, // нет вопроса
	}
	for _, tc := range cases {
		if _, _, e := AnswerQuestion(ctxFor(t, s, tc.login), tc.id, tc.text); e == nil || e.Code != tc.code {
			t.Errorf("%s %s: %+v", tc.login, tc.id, e)
		}
	}
	if _, _, e := AnswerQuestion(ctxFor(t, s, "expert.orlov"), qid, "ответ"); e != nil {
		t.Fatal(e)
	}
	if _, _, e := AnswerQuestion(ctxFor(t, s, "expert.orlov"), qid, "ещё"); e == nil || e.Code != "validation" {
		t.Fatal("повторный ответ")
	}
	if _, e := ApprovePage(ctxFor(t, s, "expert.orlov"), "kb-404", "", ""); e == nil || e.Code != "not_found" {
		t.Fatal("нет черновика")
	}
}

// BR-32: папка по похожим страницам, похожая страница предлагается к дополнению.
func TestPlacePage(t *testing.T) {
	c := ctxFor(t, newStore(t), "expert.orlov")
	folder, similar := placePage(c, "Как получить доступ к Redash для новичков?")
	if similar == "" || folder == "" {
		t.Fatalf("похожая страница не найдена: %q %q", folder, similar)
	}
	if folder, similar := placePage(c, "zzz"); folder != defaultFolder || similar != "" {
		t.Fatalf("без совпадений — папка по умолчанию: %q %q", folder, similar)
	}
}

// Модель пишет вопрос письмом — заголовок страницы берётся из сути, а не из приветствия.
func TestQuestionTitle(t *testing.T) {
	letter := "Привет! Я Даниил, стажёр.\n\nСуть: конверсия в дашборде расходится с выгрузкой из ads_mart.\n\nКонтекст: 1–7 ноября."
	cases := map[[2]string]string{
		{"Почему конверсия расходится с DWH?", letter}: "Почему конверсия расходится с DWH?",
		{"", letter}: "конверсия в дашборде расходится с выгрузкой из ads_mart.",
		{"", "Привет!\nГде шаблон презентации?"}: "Где шаблон презентации?",
		{"", "Привет!"}: "Привет!",
	}
	for in, want := range cases {
		if got := QuestionTitle(in[0], in[1]); got != want {
			t.Errorf("%q: %q, ожидалось %q", in, got, want)
		}
	}
	if got := QuestionTitle(strings.Repeat("я", 200), ""); len([]rune(got)) != 121 {
		t.Errorf("обрезка: %d", len([]rune(got)))
	}
	s := newStore(t)
	r := Run(ctxFor(t, s, "intern.sokolov"), "send_question", map[string]any{"text": letter, "addressee_id": "u-orlov"})
	_, page, _ := AnswerQuestion(ctxFor(t, s, "expert.orlov"), r.Data.(*store.Question).ID, "ответ")
	if page.Title != "конверсия в дашборде расходится с выгрузкой из ads_mart." || page.Folder == defaultFolder {
		t.Fatalf("страница из письма: %q, папка %q", page.Title, page.Folder)
	}
}
