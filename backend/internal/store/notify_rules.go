package store

import (
	"fmt"
	"strings"
	"time"
)

// Правила «событие → уведомления». Тексты — из шаблонов docs/04-mock-data/notifications.json:
// только тип события и имена, без текста вопросов, ответов и страниц (BR-51), — их можно отправлять в Telegram.

// MSK — часовой пояс отдела в мок-данных (+03:00).
var MSK = time.FixedZone("MSK", 3*3600)

// DigestSlots — время сводок обычных вопросов (BR-20).
var DigestSlots = []int{11, 16}

func (s *Store) notificationsFor(e LogEvent) []Notification {
	m := e.Meta
	name := func(id string) string { return s.Name(id) }
	first := func(id string) string { return strings.Split(s.Name(id), " ")[0] }
	// Тексты дружелюбные и конкретные: что произошло и что сделать дальше. Имена — в именительном падеже
	// (склонять имена кодом ненадёжно). Без текстов вопросов, ответов и причины отгула (BR-51).
	switch e.Type {
	case EvQuestionSent:
		to := m["to"]
		if to == "" || to == e.Actor {
			return nil
		}
		// Квитанция автору: вопрос ушёл человеку (H3: стажёр видит маршрут)
		receipt := Notification{UserID: e.Actor, Type: "question_sent", View: "questions", TG: "pending",
			Text: "✉️ Отправил твой вопрос. Отвечает: " + name(to) + ".\nКак только придёт ответ — сразу напишу. А пока можно заняться другими шагами плана."}
		if m["urgent"] == "true" { // BR-21: срочный — сразу
			receipt.Text = "🔥 Отправил твой срочный вопрос. Отвечает: " + name(to) + ".\nПометил, что без ответа работа стоит, — он придёт адресату сразу."
			return []Notification{receipt, {UserID: to, Type: "urgent_question", View: "questions", Urgent: true, TG: "pending",
				Text: "🔥 Срочный вопрос от стажёра · " + name(e.Actor) + ".\nБез ответа у него стоит работа. Ответить можно прямо в карточке вопроса — пары строк хватит."}}
		}
		// BR-20: обычный — в веб сразу, в Telegram — сводкой в 11:00 или 16:00
		return []Notification{receipt, {UserID: to, Type: "question_new", View: "questions", TG: "pending",
			DeliverAt: NextDigest(Now()).Format(time.RFC3339),
			Text:      "❓ Новый вопрос от стажёра · " + name(e.Actor) + "."}}
	case EvQuestionAnswered:
		var out []Notification
		if asker := m["asker"]; asker != "" {
			out = append(out, Notification{UserID: asker, Type: "answer_ready", View: "questions", TG: "pending",
				Text: "💬 Пришёл ответ на твой вопрос · " + name(e.Actor) + ".\nОткрой раздел «Вопросы» в Напарнике. Если что-то непонятно — уточни там же."})
		}
		if m["page"] != "" {
			out = append(out, Notification{UserID: e.Actor, Type: "draft_review", View: "questions", TG: "pending",
				Text: "📄 Спасибо за ответ! Я оформил его страницей для базы знаний.\nПроверь текст и нажми «Ок, опубликовать» — тогда следующий стажёр получит ответ сразу, без тебя."})
		}
		return out
	case EvRequestCreated:
		what, out := m["title"], []Notification{}
		if what == "" {
			what = "заявка " + e.Object
		}
		if emp := m["employee"]; emp != "" { // квитанция автору (BR-43): что, номер и кто согласует
			out = append(out, Notification{UserID: emp, Type: "request_sent", View: "requests", TG: "pending",
				Text: "📝 Заявка отправлена: " + what + " (" + e.Object + ").\nСогласует: " + name(m["approver"]) + ". Как только будет решение — напишу."})
		}
		if m["approver"] != "" && m["approver"] != m["employee"] {
			out = append(out, Notification{UserID: m["approver"], Type: "approval_needed", View: "requests", TG: "pending",
				Text: "🗂 Нужно ваше решение: " + name(m["employee"]) + " — " + what + " (" + e.Object + ").\nСогласовать или отклонить можно в разделе «Согласование»."})
		}
		return out
	case EvPagePublished:
		// H5: ответ стал знанием команды — автору вопроса
		if asker := m["asker"]; asker != "" && asker != e.Actor {
			return []Notification{{UserID: asker, Type: "answer_published", View: "knowledge", TG: "pending",
				Text: "📚 Ответ на твой вопрос теперь в базе знаний (автор — " + name(e.Actor) + ").\nСледующий стажёр получит его сразу, без ожидания. Твой вопрос помог команде!"}}
		}
	case EvRequestDecided:
		return []Notification{{UserID: m["employee"], Type: "request_status", View: "requests", TG: "pending", Text: decisionText(m, first(m["employee"]))}}
	case EvMeetingScheduled:
		start, _ := time.Parse(time.RFC3339, m["start"])
		when := RuDay(start.In(MSK).Format(time.DateOnly)) + ", " + start.In(MSK).Format("15:04")
		return []Notification{{UserID: m["asker"], Type: "meeting_scheduled", View: "day", TG: "pending",
			Text: "📅 Встреча по твоему вопросу: " + when + " · " + name(e.Actor) + ".\nЧто сделать: запиши, что уже пробовал и где застрял, — так встреча займёт меньше времени."}}
	case EvMeetingNoRecording:
		// BR-37: один раз предлагаем ответить текстом
		return []Notification{{UserID: e.Actor, Type: "meeting_no_recording", View: "questions", TG: "pending",
			Text: "🎙 Записи встречи нет — ответ не попадёт в базу знаний.\nОтветишь текстом? Пары строк хватит, остальное я оформлю."}}
	case EvQuestionOpen:
		var out []Notification
		if asker := m["asker"]; asker != "" {
			out = append(out, Notification{UserID: asker, Type: "question_open", View: "questions", TG: "pending",
				Text: "📂 Ответ текстом по твоему вопросу не получится — я передал его владельцу базы знаний.\nОн найдёт, кто ответит. Если вопрос блокирует работу — напиши бадди."})
		}
		if owner := m["owner"]; owner != "" && owner != e.Actor {
			out = append(out, Notification{UserID: owner, Type: "open_question", View: "day", TG: "pending",
				Text: "📂 Новый открытый вопрос в базе знаний · " + name(m["asker"]) + ".\nВстреча прошла без записи, ответа текстом не будет. Назначь, кто ответит."})
		}
		return out
	case EvQuestionRerouted:
		// BR-16, BR-17: стажёр и новый адресат узнают о смене; прежний — что отвечать не нужно
		var out []Notification
		newTo, asker := m["to"], m["asker"]
		why := map[string]string{"not_mine": " подсказал(а), кто ответит лучше", "redirect": " (бадди) передал(а) его коллеге", "assign": " назначил(а), кто ответит"}[m["kind"]]
		if asker != "" {
			out = append(out, Notification{UserID: asker, Type: "question_rerouted", View: "questions", TG: "pending",
				Text: "↪️ Твой вопрос теперь у " + "коллеги · " + name(newTo) + ".\n" + name(e.Actor) + why + ". Ничего делать не нужно — ответ придёт сюда же."})
		}
		urgent := m["urgent"] == "true"
		out = append(out, Notification{UserID: newTo, Type: "question_new", View: "questions", TG: "pending", Urgent: urgent,
			Text: "↪️ Тебе передали вопрос стажёра · " + name(asker) + ".\nКонтекст уже собран в карточке — ответить можно парой строк или назначить встречу."})
		if m["kind"] != "not_mine" && m["from"] != "" && m["from"] != e.Actor && m["from"] != newTo {
			out = append(out, Notification{UserID: m["from"], Type: "question_taken", View: "questions", TG: "pending",
				Text: "👌 Вопрос стажёра · " + name(asker) + " передан коллеге — отвечать не нужно. Спасибо!"})
		}
		return out
	case EvPagesMerged:
		var out []Notification
		for _, a := range strings.Split(m["authors"], ",") {
			if a != "" && a != e.Actor {
				out = append(out, Notification{UserID: a, Type: "pages_merged", View: "knowledge", TG: "pending",
					Text: "🧩 Две похожие страницы в базе объединены в одну — твоя теперь ведёт на неё (BR-39).\nЗагляни, всё ли верно в общем тексте."})
			}
		}
		return out
	case EvAnswerFeedback:
		// H11: «устарело» → владельцу страницы
		if m["value"] == "stale" && m["author"] != "" && m["author"] != e.Actor {
			title := m["page_title"]
			if title == "" {
				title = "одна из твоих страниц"
			} else {
				title = "«" + title + "»"
			}
			return []Notification{{UserID: m["author"], Type: "page_stale", View: "knowledge", TG: "pending",
				Text: "🕰 Стажёр отметил, что страница " + title + " устарела.\nЗагляни, когда будет минутка: может, стоит обновить пару строк."}}
		}
	}
	return nil
}

// decisionText — решение по заявке: что именно и что делать дальше.
func decisionText(m map[string]string, firstName string) string {
	what := m["title"]
	if what == "" {
		what = "заявка"
	}
	switch m["status"] {
	case "approved":
		return "✅ " + capitalize(what) + " — согласовано!\nНичего больше делать не нужно. Хорошего отдыха, " + firstName + "!"
	case "granted":
		text := "✅ " + capitalize(what) + " — доступ выдан!"
		if step := m["step_title"]; step != "" {
			return text + "\nЧто дальше: отметь шаг «" + step + "» выполненным в плане — один клик в Напарнике."
		}
		return text + "\nМожно пользоваться."
	case "rejected":
		text := "❌ " + capitalize(what) + " — не согласовано."
		if r := m["reason_text"]; r != "" {
			text += "\nПричина: " + strings.TrimSuffix(r, ".") + "."
		}
		next := map[string]string{
			"DATE_CONFLICT":  "выбери другую дату и подай снова — я помогу оформить.",
			"TOO_LATE":       "в следующий раз подавай не позже чем за 2 рабочих дня. Можно выбрать другую дату.",
			"NEED_INFO":      "посмотри комментарий руководителя в Напарнике, дополни и подай снова.",
			"NOT_NEEDED_YET": "этот доступ понадобится позже по плану — я напомню, когда придёт время.",
		}[m["reason_code"]]
		if next == "" {
			next = "подробности и комментарий — в разделе «Заявки»."
		}
		return text + "\nЧто сделать: " + next
	}
	return "Статус заявки изменился — посмотри в разделе «Заявки»."
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// RuDay — «пятницу, 13 ноября» для «на …» (винительный падеж дня недели).
func RuDay(date string) string {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	days := []string{"воскресенье", "понедельник", "вторник", "среду", "четверг", "пятницу", "субботу"}
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	return fmt.Sprintf("%s, %d %s", days[d.Weekday()], d.Day(), months[d.Month()-1])
}

// NextDigest — ближайший слот сводки (11:00 / 16:00 МСК, по будням).
func NextDigest(now time.Time) time.Time {
	t := now.In(MSK)
	for d := 0; d < 8; d++ {
		day := time.Date(t.Year(), t.Month(), t.Day()+d, 0, 0, 0, 0, MSK)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for _, h := range DigestSlots {
			slot := day.Add(time.Duration(h) * time.Hour)
			if slot.After(t) {
				return slot
			}
		}
	}
	return t
}

// WorkingWindow — рабочие часы пользователя «10:00-19:00» в МСК на дату t.
func WorkingWindow(hours string, t time.Time) (time.Time, time.Time, bool) {
	from, to, ok := strings.Cut(hours, "-")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	parse := func(hm string) (time.Time, bool) {
		x, err := time.Parse("15:04", strings.TrimSpace(hm))
		if err != nil {
			return time.Time{}, false
		}
		d := t.In(MSK)
		return time.Date(d.Year(), d.Month(), d.Day(), x.Hour(), x.Minute(), 0, 0, MSK), true
	}
	a, ok1 := parse(from)
	b, ok2 := parse(to)
	return a, b, ok1 && ok2
}

// NextWorkingTime — когда можно беспокоить (BR-52): сейчас, если рабочее время, иначе начало следующего рабочего дня.
func NextWorkingTime(hours string, now time.Time) time.Time {
	t := now.In(MSK)
	for d := 0; d < 8; d++ {
		day := t.AddDate(0, 0, d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		start, end, ok := WorkingWindow(hours, day)
		if !ok {
			return t
		}
		if d == 0 && !t.Before(start) && t.Before(end) {
			return t
		}
		if start.After(t) {
			return start
		}
	}
	return t
}
