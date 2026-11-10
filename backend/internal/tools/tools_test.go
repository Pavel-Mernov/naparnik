package tools

import (
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	return store.New(d, today)
}

func ctxFor(t *testing.T, s *store.Store, login string) Ctx {
	t.Helper()
	u, ok := s.UserByLogin(login)
	if !ok {
		t.Fatalf("нет учётки %s", login)
	}
	return Ctx{Store: s, User: u}
}

func data(t *testing.T, r Result) map[string]any {
	t.Helper()
	if !r.OK {
		t.Fatalf("ожидался успех, получено %+v", r.Error)
	}
	m, _ := r.Data.(map[string]any)
	return m
}

// SC-03, BR-01: ответ из базы с источником, автором и датой.
func TestSearchKBReturnsSourceWithAuthorAndDate(t *testing.T) {
	hits := SearchKB(ctxFor(t, newStore(t), "intern.sokolov"), "Что такое витрина и где лежит витрина объявлений?")
	if len(hits) == 0 || hits[0].PageID != "kb-03" || hits[0].Author != "Михаил Орлов" || hits[0].UpdatedAt == "" {
		t.Fatalf("неожиданный ответ: %+v", hits)
	}
}

// BR-05: тестовая инструкция помечена.
func TestTestInstructionIsLabelled(t *testing.T) {
	for _, h := range SearchKB(ctxFor(t, newStore(t), "intern.sokolov"), "отгул отпуск больничный оформить") {
		if h.PageID == "kb-02" && h.Label != TestInstructionLabel {
			t.Fatalf("нет пометки: %+v", h)
		}
	}
}

// BR-02, BR-40, SC-16: чужое пространство не ищется.
func TestOtherSpaceIsNeverReturned(t *testing.T) {
	for _, h := range SearchKB(ctxFor(t, newStore(t), "intern.sokolov"), "Витрина клиентов с контактными данными") {
		if h.PageID == "kb-R1" {
			t.Fatal("страница чужого пространства попала в выдачу")
		}
	}
}

// BR-04: слабое совпадение — «не нашёл».
func TestWeakMatchIsNotAnAnswer(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	for _, q := range []string{"Как посчитать retention по когортам в нашем дашборде?", "Почему конверсия в дашборде отличается от выгрузки из DWH?", ""} {
		if r := data(t, Run(c, "search_kb", map[string]any{"query": q})); r["found"] != false {
			t.Fatalf("%q: ожидалось «не нашёл»", q)
		}
	}
}

// BR-10: лидер подбора по сценарным вопросам.
func TestFindExpertLeader(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	cases := map[string]string{
		"Почему конверсия в дашборде отличается от выгрузки из DWH?":        "u-orlov",
		"Как выбрать длительность A/B-теста для низкого трафика?":           "u-smirnova",
		"Как посчитать retention по когортам в нашем дашборде?":             "u-lebedeva",
		"В ads_mart нет поля платного продвижения, без него не сдам задачу": "u-orlov",
	}
	for q, want := range cases {
		if got, _, reason := FindExpert(c, q); got != want || reason == "" {
			t.Errorf("%q: лидер %s, ожидался %s", q, got, want)
		}
	}
	if id, _, _ := FindExpert(c, "zzz qqq"); id != "" {
		t.Error("без сигналов кандидата быть не должно")
	}
}

// BR-12, BR-13: уверенный эксперт / иначе бадди.
func TestFindExpertToolThresholdAndBuddy(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	r := data(t, Run(c, "find_expert", map[string]any{"question": "Как выбрать длительность A/B-теста для низкого трафика?"}))
	if r["addressee_id"] != "u-smirnova" || r["is_buddy"] != false || r["reason"] == "" {
		t.Fatalf("ожидалась Ольга: %v", r)
	}
	r = data(t, Run(c, "find_expert", map[string]any{"question": "Где взять шаблон презентации для экватора?"}))
	if r["addressee_id"] != "u-belova" || r["is_buddy"] != true {
		t.Fatalf("SC-07: ожидался бадди: %v", r)
	}
	// SC-04, SC-08: уверенный эксперт, а не бадди (questions.json: Q2 → Орлов, Q5 → Лебедева)
	for q, want := range map[string]string{
		"Почему конверсия в дашборде отличается от выгрузки из DWH?": "u-orlov",
		"Как посчитать retention по когортам в нашем дашборде?":      "u-lebedeva",
	} {
		r = data(t, Run(c, "find_expert", map[string]any{"question": q}))
		if r["addressee_id"] != want || r["is_buddy"] != false {
			t.Errorf("%q: %v, ожидался %s", q, r, want)
		}
	}
}

// SC-01, SC-18: шаги на сегодня — просроченный первым, не больше 3, только шаги стажёра; событие дня.
func TestGetToday(t *testing.T) {
	r := data(t, Run(ctxFor(t, newStore(t), "intern.sokolov"), "get_today", nil))
	steps := r["steps"].([]stepView)
	if len(steps) == 0 || len(steps) > 3 || steps[0].Status != "Overdue" || steps[0].ID != "s-04" {
		t.Fatalf("шаги: %+v", steps)
	}
	for _, s := range steps {
		if s.Owner != "intern" {
			t.Fatalf("шаг не стажёра: %+v", s)
		}
	}
	if ev := r["events"].([]map[string]string); len(ev) != 1 || ev[0]["start"] != "15:00" {
		t.Fatalf("событие дня: %v", ev)
	}
}

// 02-ROLES: план видят стажёр, его бадди и руководитель.
func TestPlanVisibility(t *testing.T) {
	s := newStore(t)
	r := data(t, Run(ctxFor(t, s, "intern.sokolov"), "get_plan", nil))
	if r["progress_pct"].(int) <= 0 || r["next_step"].(*stepView).ID != "s-04" {
		t.Fatalf("план: %v", r)
	}
	for _, login := range []string{"buddy.belova", "lead.kovalev"} {
		data(t, Run(ctxFor(t, s, login), "get_plan", map[string]any{"intern_id": "u-sokolov"}))
	}
	if r := Run(ctxFor(t, s, "expert.orlov"), "get_plan", map[string]any{"intern_id": "u-sokolov"}); r.Error.Code != "forbidden" {
		t.Fatal("эксперт не должен видеть план стажёра")
	}
	if r := Run(ctxFor(t, s, "intern.egorova"), "get_plan", map[string]any{"intern_id": "u-sokolov"}); r.Error.Code != "forbidden" {
		t.Fatal("стажёр не должен видеть чужой план")
	}
	if r := Run(ctxFor(t, s, "lead.kovalev"), "get_plan", nil); r.Error.Code != "validation" {
		t.Fatal("руководителю нужен intern_id")
	}
	if r := Run(ctxFor(t, s, "intern.egorova"), "get_plan", nil); r.Error.Code != "not_found" {
		t.Fatal("SC-20: у Полины план ещё черновик")
	}
}

func TestMarkStepDone(t *testing.T) {
	s := newStore(t)
	if r := Run(ctxFor(t, s, "buddy.belova"), "mark_step_done", map[string]any{"step_id": "s-03", "intern_id": "u-sokolov"}); !r.OK {
		t.Fatalf("бадди должен отмечать шаг: %+v", r.Error)
	}
	if r := Run(ctxFor(t, s, "intern.egorova"), "mark_step_done", map[string]any{"step_id": "s-03", "intern_id": "u-sokolov"}); r.Error.Code != "forbidden" {
		t.Fatal("чужой стажёр не может отмечать")
	}
	if r := Run(ctxFor(t, s, "intern.sokolov"), "mark_step_done", map[string]any{"step_id": "s-99"}); r.Error.Code != "not_found" {
		t.Fatal("несуществующий шаг")
	}
	if r := Run(ctxFor(t, s, "lead.kovalev"), "mark_step_done", map[string]any{"step_id": "s-03"}); r.Error.Code != "forbidden" {
		t.Fatal("руководителю инструмент не выдаётся")
	}
}

func TestTeamDirectoryExcludesInternsAndSelf(t *testing.T) {
	r := data(t, Run(ctxFor(t, newStore(t), "intern.sokolov"), "team_directory", nil))
	people := r["people"].([]map[string]string)
	if len(people) != 5 {
		t.Fatalf("ожидалось 5 сотрудников, получено %d", len(people))
	}
}

// SC-14, BR-42: порядок с пометкой и путь «сделай сам».
func TestAbsenceRules(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	r := data(t, Run(c, "get_absence_rules", map[string]any{"type": "day_off"}))
	if r["self_service"] == nil || r["rules"] == "" || r["approver"] != "Андрей Ковалёв" {
		t.Fatalf("правила: %v", r)
	}
	if Run(c, "get_absence_rules", map[string]any{"type": "x"}).Error.Code != "validation" {
		t.Fatal("неизвестный тип")
	}
}

func TestPrecheckAbsence(t *testing.T) {
	c := ctxFor(t, newStore(t), "intern.sokolov")
	codes := func(date string) map[string]bool {
		out := map[string]bool{}
		for _, w := range data(t, Run(c, "precheck_absence", map[string]any{"type": "day_off", "date": date}))["warnings"].([]warning) {
			out[w.Code] = true
		}
		return out
	}
	if w := codes("2026-11-12"); !w["SHORT_NOTICE"] || !w["MEETING_CONFLICT"] || !w["STEP_DEADLINE"] {
		t.Fatalf("12.11: %v", w)
	}
	if w := codes("2026-11-10"); !w["PAST_DATE"] {
		t.Fatalf("10.11: %v", w)
	}
	if w := codes("2026-11-17"); len(w) != 0 {
		t.Fatalf("17.11 без предупреждений: %v", w)
	}
	if Run(c, "precheck_absence", map[string]any{"type": "day_off", "date": "пятница"}).Error.Code != "validation" {
		t.Fatal("кривая дата")
	}
}

// SC-14, SC-15, SC-29: квитанция, ошибка сервиса, валидация.
func TestCreateAbsenceRequest(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	r := Run(c, "create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-13"})
	if !r.OK || r.Receipt.(map[string]string)["id"] != "ABS-1042" || r.Receipt.(map[string]string)["approver"] != "Андрей Ковалёв" {
		t.Fatalf("квитанция: %+v %+v", r.Receipt, r.Error)
	}
	if got := data(t, Run(c, "list_my_requests", nil)); got != nil {
		t.Fatal("list_my_requests возвращает список, а не объект")
	}
	if len(s.Requests("u-sokolov")) != 1 {
		t.Fatal("заявка не сохранена")
	}
	if Run(c, "create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-01"}).Error.Code != "validation" {
		t.Fatal("прошедшая дата")
	}
	if Run(c, "create_absence_request", map[string]any{"type": "x", "date": "2026-11-20"}).Error.Code != "validation" {
		t.Fatal("неизвестный тип")
	}
	t.Setenv("MOCK_FAIL", "absence")
	if Run(c, "create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-20"}).Error.Code != "unavailable" {
		t.Fatal("SC-15: ожидалась недоступность сервиса")
	}
}

// SC-27, SC-16: доступы по неделе, отказ на чужую систему.
func TestAccesses(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	systems := data(t, Run(c, "get_required_accesses", nil))["systems"].([]map[string]string)
	ids := map[string]string{}
	for _, sy := range systems {
		ids[sy["system"]] = sy["status"]
	}
	if ids["sys-kb-analytics"] != "granted" || ids["sys-redash"] != "not_requested" || ids["sys-dwh-pii"] != "" || ids["sys-dwh-marts-ro"] != "" {
		t.Fatalf("доступы недели 1: %v", ids)
	}
	r := Run(c, "request_access", map[string]any{"system": "sys-redash", "reason": "s-04"})
	if r.Receipt.(map[string]string)["id"] != "ACC-2002" {
		t.Fatalf("квитанция: %+v", r.Receipt)
	}
	systems = data(t, Run(c, "get_required_accesses", nil))["systems"].([]map[string]string)
	for _, sy := range systems {
		if sy["system"] == "sys-redash" && sy["status"] != "pending" {
			t.Fatal("статус заявки не отражён")
		}
	}
	if e := Run(c, "request_access", map[string]any{"system": "sys-dwh-pii", "reason": "x"}).Error; e.Code != "forbidden" || e.WhoCan != "Андрей Ковалёв" {
		t.Fatalf("SC-16: %+v", e)
	}
	if Run(c, "request_access", map[string]any{"system": "sys-nope"}).Error.Code != "not_found" {
		t.Fatal("нет такой системы")
	}
	t.Setenv("MOCK_FAIL", "access")
	if Run(c, "request_access", map[string]any{"system": "sys-redash"}).Error.Code != "unavailable" {
		t.Fatal("ожидалась недоступность сервиса")
	}
}

// BR-11, BR-21: вопрос человеку и срочность.
func TestSendQuestion(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	r := Run(c, "send_question", map[string]any{"text": "В ads_mart нет поля", "addressee_id": "u-orlov", "urgent": true})
	if r.Receipt.(map[string]string)["delivery"] != "сразу (срочно)" || len(s.Questions()) != 1 {
		t.Fatalf("срочный вопрос: %+v", r)
	}
	if Run(c, "send_question", map[string]any{"text": "x", "addressee_id": "u-nobody"}).Error.Code != "not_found" {
		t.Fatal("нет адресата")
	}
	if Run(c, "send_question", map[string]any{"addressee_id": "u-orlov"}).Error.Code != "validation" {
		t.Fatal("пустой вопрос")
	}
}

// BR-40: инструмент не той роли и неизвестный инструмент.
func TestRunForbidden(t *testing.T) {
	s := newStore(t)
	if Run(ctxFor(t, s, "buddy.belova"), "create_absence_request", nil).Error.Code != "forbidden" {
		t.Fatal("бадди не оформляет заявки за стажёра")
	}
	if Run(ctxFor(t, s, "intern.sokolov"), "drop_database", nil).Error.Code != "forbidden" {
		t.Fatal("неизвестный инструмент")
	}
	names := map[string]bool{}
	for _, tl := range For(mustUser(t, s, "lead.kovalev")) {
		names[tl.Name] = true
	}
	if names["send_question"] || !names["get_plan"] {
		t.Fatalf("инструменты руководителя: %v", names)
	}
}

func mustUser(t *testing.T, s *store.Store, login string) *mockdata.User {
	u, ok := s.UserByLogin(login)
	if !ok {
		t.Fatal(login)
	}
	return u
}

func TestCheck(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	cases := []struct {
		tool string
		args map[string]any
		code string
	}{
		{"request_access", map[string]any{"system": "sys-dwh-pii"}, "forbidden"},
		{"request_access", map[string]any{"system": "sys-redash"}, ""},
		{"send_question", map[string]any{"addressee_id": "u-x"}, "not_found"},
		{"send_question", map[string]any{"addressee_id": "u-orlov"}, ""},
		{"create_absence_request", map[string]any{"type": "x"}, "validation"},
		{"create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-01"}, "validation"},
		{"create_absence_request", map[string]any{"type": "day_off", "date": "2026-11-20"}, ""},
		{"mark_step_done", map[string]any{"step_id": "s-01"}, ""},
		{"decide_anything", nil, "forbidden"},
	}
	for _, tc := range cases {
		e := Check(c, tc.tool, tc.args)
		if (e == nil && tc.code != "") || (e != nil && e.Code != tc.code) {
			t.Errorf("%s %v: %+v", tc.tool, tc.args, e)
		}
	}
	if e := Check(ctxFor(t, s, "buddy.belova"), "mark_step_done", map[string]any{"intern_id": "u-x"}); e == nil || e.Code != "forbidden" {
		t.Error("бадди не своего стажёра")
	}
}

func TestWorkdaysBetween(t *testing.T) {
	from, _ := time.Parse(time.DateOnly, "2026-11-13") // пятница
	to, _ := time.Parse(time.DateOnly, "2026-11-16")   // понедельник
	if n := workdaysBetween(from, to); n != 1 {
		t.Fatalf("выходные не считаются: %d", n)
	}
}
