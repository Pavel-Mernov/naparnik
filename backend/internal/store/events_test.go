package store

import (
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	return New(d, today)
}

// BR-73: событие пишется в журнал; BR-51: тексты уведомлений без содержания вопроса.
func TestEmitRules(t *testing.T) {
	s := newTestStore(t)
	Now = func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, MSK) }
	defer func() { Now = time.Now }()

	s.Emit(EvQuestionSent, "u-sokolov", "Q-1", map[string]string{"to": "u-orlov", "urgent": "true"})
	s.Emit(EvQuestionSent, "u-sokolov", "Q-2", map[string]string{"to": "u-orlov", "urgent": "false"})
	s.Emit(EvQuestionAnswered, "u-orlov", "Q-1", map[string]string{"asker": "u-sokolov", "page": "kb-20"})
	s.Emit(EvRequestCreated, "u-sokolov", "ABS-1042", map[string]string{"approver": "u-kovalev", "employee": "u-sokolov"})
	s.Emit(EvRequestDecided, "u-kovalev", "ABS-1042", map[string]string{"employee": "u-sokolov", "status_ru": "согласовано"})
	s.Emit(EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "stale", "author": "u-orlov"})
	s.Emit(EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "helped", "author": "u-orlov"})
	s.Emit(EvQuestionSent, "u-orlov", "Q-3", map[string]string{"to": "u-orlov"}) // сам себе — без уведомления

	if len(s.Events()) != 8 {
		t.Fatalf("журнал: %d", len(s.Events()))
	}
	orlov := s.NotificationsFor("u-orlov")
	types := map[string]Notification{}
	for _, n := range orlov {
		types[n.Type] = n
	}
	if !types["urgent_question"].Urgent || types["urgent_question"].DeliverAt != "" {
		t.Fatalf("срочный — сразу: %+v", types["urgent_question"])
	}
	if types["question_new"].DeliverAt != "2026-11-11T16:00:00+03:00" {
		t.Fatalf("обычный — в сводку 16:00: %+v", types["question_new"])
	}
	if types["draft_review"].Type == "" || types["page_stale"].Type == "" || len(orlov) != 4 {
		t.Fatalf("эксперту: %+v", orlov)
	}
	sokolov := s.NotificationsFor("u-sokolov")
	// стажёру: решение по заявке, квитанция «отправлена на согласование», ответ, две квитанции «вопрос отправлен»
	want := []string{"request_status", "request_sent", "answer_ready", "question_sent", "question_sent"}
	if len(sokolov) != len(want) {
		t.Fatalf("стажёру: %+v", sokolov)
	}
	for i, typ := range want {
		if sokolov[i].Type != typ {
			t.Fatalf("стажёру %d: %s, ожидалось %s", i, sokolov[i].Type, typ)
		}
	}
	if s.NotificationsFor("u-kovalev")[0].Type != "approval_needed" {
		t.Fatal("руководителю — заявка ждёт решения")
	}
	for _, n := range append(orlov, sokolov...) {
		if n.TG != "pending" || n.Text == "" {
			t.Fatalf("в Telegram: %+v", n)
		}
	}
	// повтор с тем же ID не дублирует (напоминания)
	if !s.Notify(Notification{ID: "rem:1", UserID: "u-sokolov"}) || s.Notify(Notification{ID: "rem:1", UserID: "u-sokolov"}) {
		t.Fatal("дубль напоминания")
	}
	s.MarkRead("u-sokolov")
	for _, n := range s.NotificationsFor("u-sokolov") {
		if !n.Read {
			t.Fatal("прочитано")
		}
	}
	// очередь Telegram: срочное — сейчас, сводка — не раньше 16:00
	if p := s.PendingTelegram(Now()); len(p) != 9 {
		t.Fatalf("сейчас к отправке: %d", len(p))
	}
	if p := s.PendingTelegram(time.Date(2026, 11, 11, 16, 0, 1, 0, MSK)); len(p) != 10 {
		t.Fatalf("после 16:00: %d", len(p))
	}
	s.SetTelegramStatus([]string{orlov[0].ID}, "sent", "")
	if s.NotificationsFor("u-orlov")[0].TG != "sent" {
		t.Fatal("статус отправки")
	}
}

func TestTelegramLinks(t *testing.T) {
	s := newTestStore(t)
	s.LinkTelegram("u-sokolov", 42)
	if id, ok := s.TelegramChat("u-sokolov"); !ok || id != 42 {
		t.Fatal("привязка")
	}
	if u, ok := s.UserByTelegram(42); !ok || u != "u-sokolov" {
		t.Fatal("поиск по чату")
	}
	s.UnlinkTelegram("u-sokolov")
	if _, ok := s.TelegramChat("u-sokolov"); ok {
		t.Fatal("отвязка")
	}
	if _, ok := s.UserByTelegram(42); ok {
		t.Fatal("чат отвязан")
	}
}

// BR-20, BR-52: сводки по будням 11:00/16:00, рабочие часы пользователя.
func TestDigestAndWorkingHours(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, MSK) }
	cases := map[time.Time]time.Time{
		at(11, 9, 0): at(11, 11, 0), at(11, 12, 0): at(11, 16, 0), at(11, 17, 0): at(12, 11, 0), at(13, 17, 0): at(16, 11, 0), // пятница → понедельник
	}
	for now, want := range cases {
		if got := NextDigest(now); !got.Equal(want) {
			t.Errorf("сводка после %v: %v, ожидалось %v", now, got, want)
		}
	}
	work := map[time.Time]time.Time{
		at(11, 12, 0): at(11, 12, 0), at(11, 8, 0): at(11, 10, 0), at(11, 20, 0): at(12, 10, 0), at(14, 12, 0): at(16, 10, 0), // суббота
	}
	for now, want := range work {
		if got := NextWorkingTime("10:00-19:00", now); !got.Equal(want) {
			t.Errorf("рабочее время после %v: %v, ожидалось %v", now, got, want)
		}
	}
	if got := NextWorkingTime("кривые часы", at(11, 3, 0)); !got.Equal(at(11, 3, 0)) {
		t.Error("без часов — не задерживаем")
	}
	if _, _, ok := WorkingWindow("10:00-xx", at(11, 0, 0)); ok {
		t.Error("кривое время")
	}
}

// Сохранённое состояние переживает перезапуск: журнал, уведомления и привязки — в снимке.
func TestSnapshotKeepsEvents(t *testing.T) {
	s := newTestStore(t)
	s.Emit(EvRequestCreated, "u-sokolov", "ACC-1", map[string]string{"approver": "u-kovalev", "employee": "u-sokolov"})
	s.LinkTelegram("u-kovalev", 7)
	raw := s.snapshot()
	r := newTestStore(t)
	if err := r.restore(raw); err != nil {
		t.Fatal(err)
	}
	if len(r.Events()) != 1 || len(r.NotificationsFor("u-kovalev")) != 1 {
		t.Fatal("журнал и уведомления")
	}
	if _, ok := r.TelegramChat("u-kovalev"); !ok {
		t.Fatal("привязка")
	}
	old := raw
	old.TGLinks = nil
	if err := r.restore(old); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.TelegramChat("u-kovalev"); ok {
		t.Fatal("старое состояние без привязок")
	}
}

// Ответ попал в базу — автору вопроса (H5); при подключении бота приходит пропущенное за сутки.
func TestPublishedAndBacklog(t *testing.T) {
	s := newTestStore(t)
	Now = func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, MSK) }
	defer func() { Now = time.Now }()
	s.Emit(EvPagePublished, "u-orlov", "kb-20", map[string]string{"asker": "u-sokolov"})
	s.Emit(EvPagePublished, "u-orlov", "kb-21", map[string]string{"asker": "u-orlov"}) // сам себе — нет
	if n := s.NotificationsFor("u-sokolov"); len(n) != 1 || n[0].Type != "answer_published" {
		t.Fatalf("%+v", n)
	}
	s.Notify(Notification{ID: "old", UserID: "u-sokolov", TG: "skipped", At: "2026-11-09T12:00:00+03:00"})
	s.SetTelegramStatus([]string{s.NotificationsFor("u-sokolov")[1].ID}, "skipped", "")
	s.LinkTelegram("u-sokolov", 5)
	for _, n := range s.NotificationsFor("u-sokolov") {
		if n.ID == "old" && n.TG != "skipped" {
			t.Fatal("старше суток — не присылаем")
		}
		if n.Type == "answer_published" && n.TG != "pending" {
			t.Fatal("свежее пропущенное — снова в очередь")
		}
	}
}

// Тексты уведомлений: что именно и что сделать дальше; без причины отгула (BR-51).
func TestFriendlyTexts(t *testing.T) {
	if RuDay("2026-11-13") != "пятницу, 13 ноября" || RuDay("2026-11-11") != "среду, 11 ноября" || RuDay("bad") != "bad" {
		t.Fatal(RuDay("2026-11-13"))
	}
	cases := []struct {
		meta map[string]string
		want []string
	}{
		{map[string]string{"status": "approved", "title": "отгул на пятницу, 13 ноября"}, []string{"✅ Отгул на пятницу, 13 ноября — согласовано", "Хорошего отдыха, Даниил"}},
		{map[string]string{"status": "granted", "title": "доступ к Redash", "step_title": "Запроси доступ к Redash"}, []string{"доступ выдан", "отметь шаг «Запроси доступ к Redash»"}},
		{map[string]string{"status": "granted", "title": "доступ к Redash"}, []string{"Можно пользоваться"}},
		{map[string]string{"status": "rejected", "title": "отгул на пятницу, 13 ноября", "reason_code": "DATE_CONFLICT", "reason_text": "В эту дату важное событие"}, []string{"не согласовано", "Причина: В эту дату важное событие.", "выбери другую дату"}},
		{map[string]string{"status": "rejected", "reason_code": "OTHER"}, []string{"Заявка — не согласовано", "в разделе «Заявки»"}},
		{map[string]string{"status": "weird"}, []string{"Статус заявки изменился"}},
	}
	for _, c := range cases {
		got := decisionText(c.meta, "Даниил")
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%v: %q нет в %q", c.meta, w, got)
			}
		}
	}
	if capitalize("") != "" {
		t.Fatal("пустая строка")
	}
	s := newTestStore(t)
	s.Emit(EvRequestCreated, "u-sokolov", "ABS-1", map[string]string{"approver": "u-kovalev", "employee": "u-sokolov", "title": "отгул на пятницу, 13 ноября"})
	s.Emit(EvAnswerFeedback, "u-sokolov", "kb-03", map[string]string{"value": "stale", "author": "u-orlov", "page_title": "Витрины DWH"})
	if n := s.NotificationsFor("u-sokolov")[0].Text; !strings.Contains(n, "Заявка отправлена: отгул на пятницу, 13 ноября (ABS-1)") || !strings.Contains(n, "Согласует: Андрей Ковалёв") {
		t.Fatal(n)
	}
	if n := s.NotificationsFor("u-kovalev")[0].Text; !strings.Contains(n, "Даниил Соколов — отгул на пятницу, 13 ноября") {
		t.Fatal(n)
	}
	if n := s.NotificationsFor("u-orlov")[0].Text; !strings.Contains(n, "«Витрины DWH» устарела") {
		t.Fatal(n)
	}
}
