// Package telegram — канал уведомлений Напарника (SC-24, BR-50…53, docs/04-mock-data/notifications.json).
//
// Бот только сообщает о событии и даёт ссылку в веб: текстов вопросов, ответов и страниц в Telegram нет (BR-51),
// поэтому и чата с ИИ в Telegram нет — разговор с Напарником остаётся в вебе, где проверяются права.
// Подключение — по одноразовому коду из веба (BR-50). Рабочие часы и сводки 11:00/16:00 соблюдаются (BR-20, BR-52).
// Недоступный Telegram не ломает веб: уведомление остаётся в центре уведомлений (BR-53).
// Без TG_BOT_TOKEN бот работает в режиме log — пишет сообщения в журнал сервера.
package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/worker4556/naparnik/backend/internal/flags"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/cache"
	"github.com/worker4556/naparnik/backend/internal/store"
)

type Config struct {
	Token       string
	Mode        string // live | log
	WebURL      string // куда ведёт «Открыть →»
	IgnoreHours bool   // демо: не ждать рабочих часов и сводок
	APIBase     string // https://api.telegram.org (в тестах — httptest)
}

// ConfigFromEnv: TG_BOT_TOKEN (или token_tg / TELEGRAM_BOT_TOKEN), TG_MODE, WEB_URL, NOTIFY_IGNORE_HOURS.
func ConfigFromEnv() Config {
	token := firstEnv("TG_BOT_TOKEN", "TELEGRAM_BOT_TOKEN", "token_tg", "TOKEN_TG")
	mode := os.Getenv("TG_MODE")
	if mode == "" {
		mode = "log"
		if token != "" {
			mode = "live"
		}
	}
	web := os.Getenv("WEB_URL")
	if web == "" {
		web = "http://localhost:3000"
	}
	return Config{Token: token, Mode: mode, WebURL: strings.TrimRight(web, "/"), IgnoreHours: os.Getenv("NOTIFY_IGNORE_HOURS") == "true",
		APIBase: "https://api.telegram.org"}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

type Service struct {
	cfg   Config
	store *store.Store
	kv    cache.KV
	http  *http.Client
	bot   string // имя бота для ссылки t.me/<bot>?start=<код>
	now   func() time.Time
}

func New(cfg Config, s *store.Store, kv cache.KV) *Service {
	return &Service{cfg: cfg, store: s, kv: kv, http: &http.Client{Timeout: 40 * time.Second}, now: time.Now}
}

func (t *Service) Mode() string { return t.cfg.Mode }
func (t *Service) Bot() string  { return t.bot }

// ---------- Bot API ----------

func (t *Service) call(ctx context.Context, method string, body any, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/%s", t.cfg.APIBase, t.cfg.Token, method), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: недоступен", method) // без URL: в нём токен
	}
	defer resp.Body.Close()
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || !env.OK {
		return fmt.Errorf("telegram %s: %s", method, env.Description)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func (t *Service) send(ctx context.Context, chatID int64, text string) error {
	if t.cfg.Mode != "live" {
		log.Printf("telegram (log) → чат %d: %s", chatID, text)
		return nil
	}
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text, "disable_web_page_preview": true}, nil)
}

// ---------- привязка (BR-50) ----------

const codeTTL = 10 * time.Minute

// LinkCode — одноразовый код на 10 минут и ссылка на бота.
func (t *Service) LinkCode(ctx context.Context, userID string) (code, link string, err error) {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	code = fmt.Sprintf("LINK-%X", b)
	if err := t.kv.SetTTL(ctx, "tg:code:"+code, userID, codeTTL); err != nil {
		return "", "", err
	}
	if t.bot != "" {
		link = "https://t.me/" + t.bot + "?start=" + code
	}
	return code, link, nil
}

// Unlink — отключить Telegram из веба.
func (t *Service) Unlink(userID string) error {
	_, _, _ = t.kv.Take(context.Background(), linkKey(userID))
	return t.store.Transaction(func(s *store.Store) error { s.UnlinkTelegram(userID); return nil })
}

// Привязка чата дублируется в Redis без срока: если состояние Напарника сбросили (новые демо-данные,
// пересоздали базу), бот не «забывает» людей и при старте возвращает привязки (BR-50).
func linkKey(userID string) string { return "tg:link:" + userID }

func (t *Service) remember(ctx context.Context, userID string, chatID int64) {
	if err := t.kv.SetTTL(ctx, linkKey(userID), fmt.Sprint(chatID), 0); err != nil {
		log.Printf("telegram: копия привязки не сохранена: %v", err)
	}
}

// Restore возвращает привязки из Redis, которых нет в состоянии, и копирует в Redis те, что есть только в состоянии.
func (t *Service) Restore(ctx context.Context) (restored int) {
	_ = t.store.Transaction(func(s *store.Store) error {
		for _, u := range s.Data.Team.Users {
			if chat, ok := s.TelegramChat(u.ID); ok {
				t.remember(ctx, u.ID, chat)
				continue
			}
			v, ok, err := t.kv.Get(ctx, linkKey(u.ID))
			var chat int64
			if err == nil && ok {
				if _, err := fmt.Sscan(v, &chat); err == nil && chat != 0 {
					s.LinkTelegram(u.ID, chat)
					restored++
				}
			}
		}
		return nil
	})
	if restored > 0 {
		log.Printf("telegram: восстановлено привязок из Redis: %d", restored)
	}
	return restored
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// Handle — ответ бота на сообщение. Бот не разговаривает о работе: только привязка и подсказка, где Напарник.
func (t *Service) Handle(ctx context.Context, chatID int64, text string) string {
	text = strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(text, "/start"):
		code := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(text, "/start")))
		if code == "" {
			return "Привет! Я присылаю уведомления Напарника. Чтобы подключиться, открой Напарник → меню учётки → «Подключить Telegram» и пришли мне код."
		}
		userID, ok, err := t.kv.Take(ctx, "tg:code:"+code)
		if err != nil || !ok {
			return "Код не подошёл или устарел — он действует 10 минут. Возьми новый в Напарнике."
		}
		var missed int
		var last string
		if err := t.store.Transaction(func(s *store.Store) error { missed, last = s.LinkTelegram(userID, chatID); return nil }); err != nil {
			return "Не получилось сохранить подключение. Попробуй ещё раз чуть позже."
		}
		t.remember(ctx, userID, chatID)
		reply := fmt.Sprintf("Готово, %s! Буду присылать уведомления: ответы на вопросы, статусы заявок, напоминания — только в твои рабочие часы. Отключить — /stop.", firstName(t.store.Name(userID)))
		if missed > 0 {
			reply += fmt.Sprintf("\n\nПока Telegram не был подключён, в Напарнике накопилось уведомлений: %d — они ждут там. Последнее:\n%s", missed, firstLine(last))
		}
		return reply
	case text == "/stop":
		if userID, ok := t.store.UserByTelegram(chatID); ok {
			_ = t.Unlink(userID)
			return "Отключил уведомления. Всё по-прежнему видно в Напарнике."
		}
		return "Этот чат и так не подключён."
	case strings.HasPrefix(strings.ToUpper(text), "LINK-"):
		return t.Handle(ctx, chatID, "/start "+text)
	default:
		return "Я только присылаю уведомления — вопросы и заявки живут в Напарнике, там мы проверяем права и не отправляем ничего без подтверждения: " + t.cfg.WebURL
	}
}

// bundle — несколько уведомлений одним сообщением: первые строки, свежие сверху, не больше пяти.
func bundle(list []store.Notification) string {
	sorted := append([]store.Notification(nil), list...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At > sorted[j].At })
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 Новых уведомлений: %d", len(sorted))
	for i, n := range sorted {
		if i == 5 {
			fmt.Fprintf(&b, "\n…и ещё %d — в Напарнике.", len(sorted)-5)
			break
		}
		b.WriteString("\n• " + firstLine(n.Text))
	}
	return b.String()
}

// command — что пришло, без кода привязки: он одноразовый, но в журнал не пишем.
func command(text string) string {
	f := strings.Fields(text)
	if len(f) == 0 {
		return ""
	}
	if strings.HasPrefix(strings.ToUpper(f[0]), "LINK-") {
		return "LINK-…"
	}
	if len(f) > 1 && f[0] == "/start" {
		return "/start <код>"
	}
	return f[0]
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if r := []rune(line); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return line
}

func firstName(name string) string { return strings.Split(name, " ")[0] }

// ---------- доставка ----------

// Flush отправляет накопившиеся уведомления: рабочие часы (BR-52), сводка обычных вопросов (BR-20),
// неподключённым — пропуск (уведомление остаётся в вебе, BR-53). Отправляет один экземпляр backend.
func (t *Service) Flush(ctx context.Context) error {
	if !flags.On(flags.Telegram, nil) {
		return nil // выключено флагом: уведомления копятся в вебе и уйдут, когда флаг включат
	}
	if ok, err := t.kv.Lock(ctx, "tg:flush", 20*time.Second); err != nil || !ok {
		return err
	}
	now := t.now()
	due := now
	if t.cfg.IgnoreHours { // демо: всё сразу — без ожидания рабочих часов и сводок 11:00/16:00
		due = now.AddDate(1, 0, 0)
	}
	byUser := map[string][]store.Notification{}
	for _, n := range t.store.PendingTelegram(due) {
		byUser[n.UserID] = append(byUser[n.UserID], n)
	}
	for userID, list := range byUser {
		ids := func(xs []store.Notification) []string {
			out := make([]string, len(xs))
			for i, x := range xs {
				out[i] = x.ID
			}
			return out
		}
		chatID, linked := t.store.TelegramChat(userID)
		if !linked {
			t.mark(ids(list), "skipped", "")
			continue
		}
		if u, ok := t.store.User(userID); ok && !t.cfg.IgnoreHours {
			if next := store.NextWorkingTime(u.WorkingHours, now); next.After(now) {
				t.mark(ids(list), "pending", next.Format(time.RFC3339))
				continue
			}
		}
		// «Не беспокоить» (SCR-I8, BR-52): несрочное ждёт конца паузы, срочное приходит сразу.
		if quiet := t.store.QuietUntil(userID, now); !quiet.IsZero() {
			var later []string
			kept := list[:0:0]
			for _, n := range list {
				if n.Urgent {
					kept = append(kept, n)
				} else {
					later = append(later, n.ID)
				}
			}
			t.mark(later, "pending", quiet.Format(time.RFC3339))
			list = kept
		}
		var digest, single []store.Notification
		for _, n := range list {
			if n.Type == "question_new" {
				digest = append(digest, n)
			} else {
				single = append(single, n)
			}
		}
		// Срочное — всегда отдельным сообщением; остальное, что накопилось к одному моменту, — одним (не засыпаем чат).
		var urgent, rest []store.Notification
		for _, n := range single {
			if n.Urgent {
				urgent = append(urgent, n)
			} else {
				rest = append(rest, n)
			}
		}
		for _, n := range urgent {
			t.deliver(ctx, chatID, []store.Notification{n}, n.Text)
		}
		switch {
		case len(rest) == 1:
			t.deliver(ctx, chatID, rest, rest[0].Text)
		case len(rest) > 1:
			t.deliver(ctx, chatID, rest, bundle(rest))
		}
		if len(digest) > 0 {
			t.deliver(ctx, chatID, digest, fmt.Sprintf("📬 Сводка: новых вопросов от стажёров — %d.\nОтветить можно прямо в карточке вопроса, пары строк хватит.", len(digest)))
		}
	}
	return nil
}

func (t *Service) deliver(ctx context.Context, chatID int64, list []store.Notification, text string) {
	ids := make([]string, len(list))
	for i, n := range list {
		ids[i] = n.ID
	}
	if err := t.send(ctx, chatID, text+"\n\n👉 Открыть Напарник: "+t.cfg.WebURL); err != nil {
		log.Printf("telegram: %v — уведомление осталось в вебе", err)
		t.mark(ids, "failed", "")
		return
	}
	t.mark(ids, "sent", "")
}

func (t *Service) mark(ids []string, status, deliverAt string) {
	_ = t.store.Transaction(func(s *store.Store) error { s.SetTelegramStatus(ids, status, deliverAt); return nil })
}

// Schedule — напоминания по времени: утренний план (BR-83, BR-84) и события календаря за день и за час.
func (t *Service) Schedule() error {
	now := t.now().In(store.MSK)
	date := now.Format(time.DateOnly)
	return t.store.Transaction(func(s *store.Store) error {
		for _, u := range s.Data.Team.Users {
			// Утренний план — стажёрам с режимом «план на день», в начале рабочего дня.
			settings := s.SettingsFor(&u)
			if settings.Proactivity == "daily_plan" {
				if start, end, ok := store.WorkingWindow(u.WorkingHours, now); ok && !now.Before(start) && now.Before(end) {
					open, overdue, firstStep := 0, 0, ""
					for _, st := range s.PlanSteps(u.ID) {
						if st.Status != "Today" && st.Status != "Overdue" {
							continue
						}
						open++
						title := st.StepID
						if ts, ok := s.TemplateStep(st.StepID); ok {
							title = ts.Title
						}
						if st.Status == "Overdue" {
							overdue++
							if overdue == 1 {
								firstStep = title // начинаем с просроченного
							}
						} else if firstStep == "" {
							firstStep = title
						}
					}
					if open > 0 {
						text := fmt.Sprintf("☀️ Доброе утро, %s! На сегодня по плану шагов: %d.", firstName(u.Name), open)
						if overdue > 0 {
							text += fmt.Sprintf("\nОдин уже ждёт со вчера — начни с него: «%s».", firstStep)
						} else {
							text += fmt.Sprintf("\nНачни с «%s». Застрянешь — спроси меня в Напарнике.", firstStep)
						}
						s.Notify(store.Notification{ID: "plan:" + u.ID + ":" + date, UserID: u.ID, Type: "morning_plan", Text: text, View: "day", TG: "pending"})
					}
				}
			}
			reminders := settings.Reminders // пустой список — человек выключил напоминания
			for _, e := range s.Data.Calendar.Events {
				if !contains(e.Participants, u.ID) {
					continue
				}
				start, err := time.Parse(time.RFC3339, e.Start)
				if err != nil {
					continue
				}
				for _, r := range reminders {
					lead, label := time.Hour, "⏰ Через час, в %s — %s."
					if r == "1d" {
						lead, label = 24*time.Hour, "📅 Завтра в %s — %s."
					}
					// «Завтра» — только если событие и правда завтра по календарю, а не через 24 часа.
					tomorrow := start.In(store.MSK).Format(time.DateOnly) == now.AddDate(0, 0, 1).Format(time.DateOnly)
					if !now.Before(start.Add(-lead)) && now.Before(start) && (r != "1d" || (tomorrow && now.Before(start.Add(-time.Hour)))) {
						s.Notify(store.Notification{ID: "rem:" + e.ID + ":" + u.ID + ":" + r, UserID: u.ID, Type: "event_reminder",
							Text: fmt.Sprintf(label, start.In(store.MSK).Format("15:04"), e.Title) + "\n" + eventTip(e.Type, store.HasRole(&u, store.RoleBuddy)), View: "day", TG: "pending"})
					}
				}
			}
		}
		// Встречи по вопросам (US-14, US-15): за час — участникам; организатору — включить запись за 5 минут и в начале (BR-35)
		for _, m := range s.Meetings() {
			if m.Status != "scheduled" {
				continue
			}
			start, err := time.Parse(time.RFC3339, m.Start)
			if err != nil {
				continue
			}
			hm := start.In(store.MSK).Format("15:04")
			for _, p := range m.Participants {
				if !now.Before(start.Add(-time.Hour)) && now.Before(start) {
					tip := "Что сделать: запиши, что уже пробовал и где застрял, — так встреча займёт меньше времени."
					if p == m.OrganizerID {
						tip = "Что сделать: пролистай вопрос стажёра в карточке — там суть и контекст."
					}
					s.Notify(store.Notification{ID: "rem:" + m.ID + ":" + p + ":1h", UserID: p, Type: "event_reminder", View: "questions", TG: "pending",
						Text: fmt.Sprintf("⏰ Через час, в %s — встреча по вопросу · %s.\n%s", hm, s.Name(m.OrganizerID), tip)})
				}
			}
			if !now.Before(start.Add(-5*time.Minute)) && now.Before(start) {
				s.Notify(store.Notification{ID: "rec5:" + m.ID, UserID: m.OrganizerID, Type: "enable_recording", View: "questions", TG: "pending", Urgent: true,
					Text: fmt.Sprintf("🔴 Через 5 минут, в %s, — встреча по вопросу стажёра.\nВключи запись: из неё я соберу ответ для базы знаний.", hm)})
			}
			if !now.Before(start) && now.Before(start.Add(10*time.Minute)) {
				s.Notify(store.Notification{ID: "rec0:" + m.ID, UserID: m.OrganizerID, Type: "enable_recording", View: "questions", TG: "pending", Urgent: true,
					Text: "▶️ Встреча началась — запись включена?\nПосле встречи отметь в карточке вопроса, была ли запись, а ответ я оформлю сам."})
			}
		}
		if flags.On(flags.Watchers, nil) {
			s.Watch(now) // SLA вопросов (BR-18), простой стажёра (BR-85), автоактивация плана (BR-81)
		}
		return nil
	})
}

// eventTip — что сделать перед событием: короткая подсказка по его типу.
func eventTip(kind string, isBuddy bool) string {
	switch kind {
	case "one_on_one":
		if isBuddy {
			return "Что сделать: посмотри, где стажёр застрял, — это видно в его плане."
		}
		return "Что сделать: запиши 2–3 вопроса, которые хочешь обсудить. Мелких вопросов не бывает."
	case "program":
		return "Что сделать: подготовь вопросы — на мастер-классах их всегда не хватает."
	case "team":
		return "Что сделать: продумай одну фразу — что сделал и где нужна помощь."
	case "deadline":
		return "Что сделать: если не успеваешь — напиши бадди заранее, это нормально."
	}
	return "Открой Напарник — там все детали."
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Run — фон: опрос бота (live), напоминания и отправка раз в 15 секунд. Ошибки Telegram не роняют сервер.
func (t *Service) Run(ctx context.Context) {
	t.Restore(ctx)
	if t.cfg.Mode == "live" {
		var me struct {
			Username string `json:"username"`
		}
		if err := t.call(ctx, "getMe", map[string]any{}, &me); err != nil {
			log.Printf("telegram: %v — проверь TG_BOT_TOKEN; уведомления остаются в вебе", err)
		} else {
			t.bot = me.Username
			log.Printf("telegram: бот @%s подключён", t.bot)
			go t.poll(ctx)
		}
	} else {
		log.Printf("telegram: режим log (нет TG_BOT_TOKEN) — уведомления пишутся в журнал")
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		if err := t.Schedule(); err != nil {
			log.Printf("напоминания: %v", err)
		}
		if err := t.Flush(ctx); err != nil {
			log.Printf("telegram: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (t *Service) poll(ctx context.Context) {
	var offset int64
	var lastErr string
	for ctx.Err() == nil {
		var ups []update
		if err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25, "allowed_updates": []string{"message"}}, &ups); err != nil {
			// Ошибку видно в журнале (раньше глоталась): например, Conflict — тот же бот опрашивает другой экземпляр.
			if msg := err.Error(); msg != lastErr && ctx.Err() == nil {
				log.Printf("telegram: %v", err)
				lastErr = msg
			}
			time.Sleep(5 * time.Second)
			continue
		}
		lastErr = ""
		for _, u := range ups {
			offset = u.UpdateID + 1
			if u.Message == nil {
				continue
			}
			reply := t.Handle(ctx, u.Message.Chat.ID, u.Message.Text)
			log.Printf("telegram: входящее %q → %s", command(u.Message.Text), firstLine(reply))
			if err := t.send(ctx, u.Message.Chat.ID, reply); err != nil {
				log.Printf("telegram: %v", err)
			}
		}
	}
}
