package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// BR-42: «сделай сам» — стажёр оформляет заявку в форме сервиса, проверки те же, что у агента.
func TestCreateSelf(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	res := CreateSelf(c, "absence", map[string]any{"type": "day_off", "date": "2026-11-16", "comment": "семейные дела"})
	r, isReq := res.Data.(store.Request)
	if !res.OK || !isReq || r.ID != "ABS-1042" || r.Source != SourceSelf || r.Status != "pending" || r.ApproverID != "u-kovalev" {
		t.Fatalf("отгул сам: %+v", res)
	}
	if !strings.Contains(r.History[0].Text, "сам") || len(r.History) != 2 {
		t.Fatalf("история: %+v", r.History)
	}
	acc := CreateSelf(c, "access", map[string]any{"system": "sys-redash", "reason": "шаг плана"})
	if a := acc.Data.(store.Request); !acc.OK || a.ID != "ACC-2002" || a.StepID != "s-04" {
		t.Fatalf("доступ сам: %+v", acc)
	}
	cases := []struct {
		login, kind string
		args        map[string]any
		code        string
	}{
		{"intern.sokolov", "car", nil, "validation"},
		{"intern.sokolov", "absence", map[string]any{"type": "day_off", "date": "2026-11-01"}, "validation"},
		{"intern.sokolov", "access", map[string]any{"system": "sys-dwh-pii"}, "forbidden"}, // BR-44
		{"lead.kovalev", "absence", map[string]any{"type": "day_off", "date": "2026-11-16"}, "forbidden"},
	}
	for _, tc := range cases {
		if r := CreateSelf(ctxFor(t, s, tc.login), tc.kind, tc.args); r.OK || r.Error.Code != tc.code {
			t.Errorf("%s %s: %+v", tc.login, tc.kind, r.Error)
		}
	}
}

// Согласование по status_flow сервисов: решает только согласующий, отказ — с кодом причины.
func TestDecide(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	abs := CreateSelf(c, "absence", map[string]any{"type": "day_off", "date": "2026-11-16"}).Data.(store.Request)
	acc := Run(c, "request_access", map[string]any{"system": "sys-redash", "reason": "s-04"}).Data.(*store.Request)
	if acc.Source != SourceAgent {
		t.Fatal("через Напарника — источник agent")
	}
	lead := ctxFor(t, s, "lead.kovalev")

	if _, e := Decide(c, abs.ID, "approve", "", ""); e == nil || e.Code != "forbidden" {
		t.Fatal("стажёр сам себе не согласует")
	}
	bad := []struct{ id, decision, reason, comment, code string }{
		{"ABS-1", "approve", "", "", "not_found"},
		{abs.ID, "maybe", "", "", "validation"},
		{abs.ID, "reject", "", "", "validation"},
		{abs.ID, "reject", "NOT_NEEDED_YET", "", "validation"}, // код другого сервиса
		{abs.ID, "reject", "OTHER", "", "validation"},          // «Другое» без комментария
	}
	for _, tc := range bad {
		if _, e := Decide(lead, tc.id, tc.decision, tc.reason, tc.comment); e == nil || e.Code != tc.code {
			t.Errorf("%+v: %+v", tc, e)
		}
	}
	r, e := Decide(lead, abs.ID, "reject", "DATE_CONFLICT", "перенеси на вторник")
	if e != nil || r.Status != "rejected" || r.ReasonCode != "DATE_CONFLICT" || !strings.Contains(r.History[2].Text, "вторник") {
		t.Fatalf("отказ: %+v %+v", r, e)
	}
	if _, e := Decide(lead, abs.ID, "approve", "", ""); e == nil || e.Code != "validation" {
		t.Fatal("повторное решение")
	}
	g, e := Decide(lead, acc.ID, "approve", "", "")
	if e != nil || g.Status != "granted" || len(g.History) != 4 {
		t.Fatalf("выдача доступа: %+v %+v", g, e)
	}
	// после выдачи доступ виден стажёру как выданный
	sys := Run(c, "get_required_accesses", nil).Data.(map[string]any)["systems"].([]map[string]string)
	for _, x := range sys {
		if x["system"] == "sys-redash" && x["status"] != "granted" {
			t.Fatalf("статус доступа: %v", x)
		}
	}
}

func TestCatalog(t *testing.T) {
	s := newStore(t)
	in := Catalog(ctxFor(t, s, "intern.sokolov"))
	sys := in["access"].(map[string]any)["systems"].([]SystemView)
	now, later := 0, 0
	for _, x := range sys {
		if x.System == "sys-dwh-pii" {
			t.Fatal("BR-40: чужая система не показывается")
		}
		if x.System == "sys-kb-analytics" && x.Status != "granted" {
			t.Fatalf("уже выданный доступ: %+v", x)
		}
		if x.Now {
			now++
		} else {
			later++
		}
	}
	if len(sys) != 8 || now != 5 || later != 3 {
		t.Fatalf("системы отдела: всего %d, сейчас %d, позже %d", len(sys), now, later)
	}
	// заявка на систему следующих недель допустима — решает руководитель
	if r := CreateSelf(ctxFor(t, s, "intern.sokolov"), "access", map[string]any{"system": "sys-ab-platform-ro", "reason": "заранее"}); !r.OK {
		t.Fatalf("доступ на будущее: %+v", r.Error)
	}
	lead := Catalog(ctxFor(t, s, "lead.kovalev"))
	if lead["access"].(map[string]any)["systems"] != nil && len(lead["access"].(map[string]any)["systems"].([]map[string]string)) != 0 {
		t.Fatal("руководителю системы стажёра не нужны")
	}
	if len(s.Data.Absence.ReasonCodes) != 4 || len(s.Data.Access.ReasonCodes) != 3 {
		t.Fatal("коды причин из мок-данных")
	}
}

// База знаний: код-стайл находится по разным формулировкам, профильные страницы — выше памяток.
func TestSearchCodeStyle(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	for _, q := range []string{"Какой у нас код-стайл для SQL и Python?", "code style SQL Python", "стиль кода SQL Python"} {
		hits := SearchKB(c, q)
		if len(hits) < 2 || hits[0].PageID != "kb-30" || hits[1].PageID != "kb-31" {
			t.Errorf("%q: %+v", q, hits)
		}
	}
	if hits := SearchKB(c, "Python код-стайл"); hits[0].PageID != "kb-31" {
		t.Errorf("Python: %s", hits[0].PageID)
	}
}

// SC-04: модель передаёт в find_expert расширенный вопрос — адресат не должен уплывать
// к автору памяток из-за общих слов («методика», «расчёт», «цифры»).
func TestExpertStableForRephrasedQ2(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	for _, q := range []string{
		"Почему конверсия в дашборде отличается от выгрузки из DWH? Какая методика расчёта считается верной и как сверить цифры?",
		"Почему конверсия в дашборде отличается от выгрузки из DWH? Нужно понять, какие расхождения методик/фильтров и логики расчёта могут давать разницу.",
		"Почему конверсия в дашборде отличается от выгрузки из DWH? Где смотреть методику расчёта конверсии и правила сверки дашборда с DWH?",
		"Почему конверсия в дашборде отличается от выгрузки из DWH? Нужно понять методологию расчёта и где искать расхождение.",
	} {
		if id, conf, _ := FindExpert(c, q); id != "u-orlov" || conf < ExpertThreshold {
			t.Errorf("%q → %s %.2f", q, id, conf)
		}
	}
}

// Кэш поиска и подбора эксперта: второй одинаковый запрос — из кэша; публикация страницы меняет версию базы,
// и кэш не отдаёт устаревшую выдачу; у другого пользователя — свой ключ (права, BR-40).
func TestKBCache(t *testing.T) {
	KBCache = cache.NewMemory()
	defer func() { KBCache = nil }()
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	h0, m0 := CacheStats()
	first := Run(c, "search_kb", map[string]any{"query": "Что такое витрина?"})
	second := Run(c, "search_kb", map[string]any{"query": "  что такое  ВИТРИНА? "})
	h1, m1 := CacheStats()
	if h1-h0 != 1 || m1-m0 != 1 {
		t.Fatalf("попадания %d, промахи %d", h1-h0, m1-m0)
	}
	a := first.Data.(map[string]any)["hits"].([]Hit)
	b := second.Data.(map[string]any)["hits"].([]Hit)
	if len(a) == 0 || a[0].PageID != b[0].PageID || a[0].Body != b[0].Body {
		t.Fatal("из кэша — тот же результат того же типа")
	}
	e1 := Run(c, "find_expert", map[string]any{"question": "Почему конверсия в дашборде отличается от выгрузки из DWH?"})
	e2 := Run(c, "find_expert", map[string]any{"question": "Почему конверсия в дашборде отличается от выгрузки из DWH?"})
	if e1.Data.(map[string]any)["addressee_id"] != "u-orlov" || e2.Data.(map[string]any)["addressee_id"] != "u-orlov" {
		t.Fatal("эксперт из кэша")
	}
	// новая версия базы — новый ключ
	v1 := kbVersion(c)
	s.AddPage(mockdata.Page{ID: "kb-99", Space: "space-analytics", Folder: "f-dwh", Status: "Published", Title: "Витрина — что это", Body: "витрина", AuthorID: "u-orlov", UpdatedAt: "2026-11-11"})
	if kbVersion(c) == v1 {
		t.Fatal("версия базы меняется")
	}
	h2, _ := CacheStats()
	Run(c, "search_kb", map[string]any{"query": "Что такое витрина?"})
	if h3, _ := CacheStats(); h3 != h2 {
		t.Fatal("после публикации — не из кэша")
	}
	if kbKey(c, "search", "Что такое витрина?") != kbKey(c, "search", "витрина, что это такое") {
		t.Fatal("одни и те же основы слов — один ключ")
	}
	if kbKey(ctxFor(t, s, "intern.egorova"), "search", "витрина") == kbKey(c, "search", "витрина") {
		t.Fatal("у другого пользователя — свой ключ")
	}
}

func TestRequestHuman(t *testing.T) {
	cases := map[string]store.Request{
		"отгул на пятницу, 13 ноября":      {Kind: "absence", Type: "day_off", Date: "2026-11-13"},
		"отпуск на понедельник, 16 ноября": {Kind: "absence", Type: "vacation", Date: "2026-11-16"},
		"отсутствие":                       {Kind: "absence"},
		"доступ к Redash (дашборды)":       {Kind: "access", Title: "Redash (дашборды)"},
		"доступ к sys-x":                   {Kind: "access", System: "sys-x"},
	}
	for want, r := range cases {
		if got := requestHuman(&r); got != want {
			t.Errorf("%q, ожидалось %q", got, want)
		}
	}
}

// SC-35, BR-10a: вопрос по теме директора уходит бадди; имя директора стажёр не видит, бадди — видит подсказку.
func TestSeniorRoutesToBuddy(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	q18 := "Планируют ли менять North Star раздела или добавлять метрику качества контактов? Хочу заложить это в дашборд"
	if id, _, _ := FindExpert(c, q18); id != "u-volkov" {
		t.Fatalf("по авторству лучший — директор: %s", id)
	}
	r := Run(c, "find_expert", map[string]any{"question": q18})
	raw, _ := json.Marshal(r)
	if d := r.Data.(map[string]any); d["addressee_id"] != "u-belova" || d["is_buddy"] != true || strings.Contains(string(raw), "Волков") {
		t.Fatalf("стажёру — бадди, без имени директора: %s", raw)
	}
	// напрямую директору — нельзя, ни черновиком, ни отправкой
	if e := Check(c, "send_question", map[string]any{"addressee_id": "u-volkov", "text": q18}); e == nil || e.Code != "validation" {
		t.Fatal("черновик директору")
	}
	if res := sendQuestion(c, map[string]any{"addressee_id": "u-volkov", "text": q18}); res.Error == nil {
		t.Fatal("отправка директору")
	}
	res := Run(c, "send_question", map[string]any{"addressee_id": "u-belova", "text": q18})
	q := res.Data.(*store.Question)
	if !strings.Contains(q.Hint, "Сергей Волков, директор по аналитике") {
		t.Fatalf("подсказка бадди: %q", q.Hint)
	}
	// обычный вопрос бадди — без подсказки; руководитель отдела не «старший» — ему можно
	if q2 := Run(c, "send_question", map[string]any{"addressee_id": "u-belova", "text": "Где шаблон презентации к экватору?"}).Data.(*store.Question); q2.Hint != "" {
		t.Fatal(q2.Hint)
	}
	if isSenior(c, "u-kovalev") || !isSenior(c, "u-volkov") || isSenior(c, "u-nobody") {
		t.Fatal("уровни руководства")
	}
	// не стажёр может писать директору
	if e := seniorBlock(ctxFor(t, s, "buddy.belova"), "u-volkov"); e != nil {
		t.Fatal("бадди переадресует директору сам")
	}
}
