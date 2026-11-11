package tools

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// MinCoverage — доля слов вопроса, найденных на странице. Ниже порога — «не нашёл», а не слабый ответ (BR-04).
const MinCoverage = 0.6

// ExpertThreshold — BR-13: уверенность выбора эксперта ниже порога → вопрос бадди.
const ExpertThreshold = 0.6

// generalFolders — памятки для всех (онбординг, процессы отдела, справочник «кто есть кто»):
// они упоминают все темы сразу, и их авторство — не предметная экспертиза (BR-10). В поиске по базе участвуют.
var generalFolders = map[string]bool{"f-start": true, "f-process": true}

// titleBonus — вес совпадений в заголовке при сортировке выдачи (на порог MinCoverage не влияет).
const titleBonus = 0.5

// MinExpertEvidence — сколько «очков» сигнала нужно для полной уверенности (страница по теме ≈ 1–4, зона ответственности ≈ 1.5).
const MinExpertEvidence = 3.0

// secondWeight — насколько близкий второй кандидат снижает уверенность. 1 — слишком строго: при двух
// авторах по теме уверенность падала ниже порога даже у очевидного лидера (Q2 → бадди вместо Орлова).
const secondWeight = 0.5

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_]+`)

var stopWords = map[string]bool{
	"как": true, "что": true, "где": true, "это": true, "мне": true, "для": true, "через": true, "такое": true,
	"нужно": true, "нужен": true, "можно": true, "почему": true, "когда": true, "какой": true, "какие": true,
	"кто": true, "мой": true, "или": true, "если": true, "есть": true, "наш": true, "нашем": true, "the": true,
}

// synonyms — словарь синонимов отдела: сводит разные слова к одной основе.
// Данные, а не логика: в проде — из настроек отдела. Пополняется по эталонному набору evals.json.
var synonyms = map[string]string{
	"отлич": "расхо", // «отличается» ≈ «расхождение»
	"совпа": "расхо", // «не совпадает» ≈ «расхождение»
	"сходи": "расхо",
	"кода":  "код", // «стиль кода», «кодом», «code style» ≈ «код-стайл»
	"кодом": "код",
	"code":  "код",
	"стиль": "стайл",
	"style": "стайл",
}

// Stems — грубый стемминг префиксом: для русского работает прилично и не требует словарей.
func Stems(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(strings.ReplaceAll(text, "ё", "е")), -1) {
		r := []rune(w)
		if len(r) < 3 || stopWords[w] {
			continue
		}
		if len(r) > 5 {
			r = r[:5]
		}
		stem := string(r)
		if syn, found := synonyms[stem]; found {
			stem = syn
		}
		out[stem] = true
	}
	return out
}

func overlap(q, doc map[string]bool) int {
	n := 0
	for s := range q {
		if doc[s] {
			n++
		}
	}
	return n
}

type Hit struct {
	PageID          string  `json:"page_id"`
	Title           string  `json:"title"`
	Body            string  `json:"body"`
	Author          string  `json:"author"`
	UpdatedAt       string  `json:"updated_at"`
	TestInstruction bool    `json:"test_instruction"`
	Label           string  `json:"label,omitempty"`
	Score           float64 `json:"score"`
}

// readablePages — BR-02, BR-40: только опубликованные страницы пространств своего отдела.
func readablePages(c Ctx) []mockdata.Page {
	spaces := c.Store.ReadableSpaces()
	var out []mockdata.Page
	for _, p := range c.Store.Pages() {
		if spaces[p.Space] && p.Status == "Published" {
			out = append(out, p)
		}
	}
	return out
}

func SearchKB(c Ctx, query string) []Hit {
	q := Stems(query)
	if len(q) == 0 {
		return nil
	}
	var hits []Hit
	for _, p := range readablePages(c) {
		score := float64(overlap(q, Stems(p.Title+" "+p.Body))) / float64(len(q))
		if score < MinCoverage {
			continue
		}
		// Порог — по всему тексту, а порядок — с бонусом за слова запроса в заголовке:
		// страница «Код-стайл Python» важнее памятки, где код-стайл упомянут одной строкой.
		score += titleBonus * float64(overlap(q, Stems(p.Title))) / float64(len(q))
		h := Hit{p.ID, p.Title, p.Body, c.Store.Name(p.AuthorID), p.UpdatedAt, p.TestInstruction, "", score}
		if p.TestInstruction {
			h.Label = TestInstructionLabel
		}
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > 3 {
		hits = hits[:3]
	}
	return hits
}

func searchKB(c Ctx, args map[string]any) Result {
	query := str(args, "query")
	hits := cached(c, "search", query, func() []Hit { return SearchKB(c, query) })
	return ok(map[string]any{"found": len(hits) > 0, "hits": hits})
}

func teamDirectory(c Ctx, _ map[string]any) Result {
	var people []map[string]string
	for _, u := range c.Store.Data.Team.Users {
		if u.ID == c.User.ID || store.HasRole(&u, store.RoleIntern) {
			continue
		}
		people = append(people, map[string]string{"id": u.ID, "name": u.Name, "position": u.Position,
			"responsibility": u.Responsibility, "working_hours": u.WorkingHours})
	}
	return ok(map[string]any{"department": c.Store.Data.Team.Department.Name, "people": people,
		"note": "Черновик сообщения коллеге стажёр отправляет сам (BR-62)."})
}

// FindExpert — BR-10…BR-13: кандидат по авторству страниц по теме (свежие весят больше) и зоне ответственности.
//
// Базовый лексический подбор. Лидер на сценарных вопросах верный (Q2 → Орлов, Q3 → Смирнова, Q5 → Лебедева),
// но уверенность у Q2 и Q5 ниже порога BR-13 → они уйдут бадди. Настраивать по docs/04-mock-data/evals.json.
func FindExpert(c Ctx, question string) (id string, confidence float64, reason string) {
	q := Stems(question)
	type cand struct {
		pageScores []float64
		resp       int
	}
	cands := map[string]*cand{}
	get := func(uid string) *cand {
		if cands[uid] == nil {
			cands[uid] = &cand{}
		}
		return cands[uid]
	}
	kbPages := readablePages(c)
	docs := make([]map[string]bool, len(kbPages))
	df := map[string]int{}
	for i, p := range kbPages {
		docs[i] = Stems(p.Title + " " + p.Body)
		for st := range q {
			if docs[i][st] {
				df[st]++
			}
		}
	}
	// IDF: слово, которое есть во многих страницах («правила», «метод», «дашборд»), — слабый сигнал;
	// редкое предметное («конверсия», «выгрузка») — сильный. Так подбор не уплывает, когда модель
	// дописывает к вопросу общие слова. Слово из 3 страниц и реже весит 1, чаще — меньше
	// (сверху ограничено: случайное редкое слово не должно перевешивать тему).
	n := float64(len(kbPages))
	weight := func(st string) float64 { return math.Min(1, math.Log(1+n/float64(df[st]))/math.Log(1+n/3)) }
	for i, p := range kbPages {
		if p.AuthorID == "" || p.AuthorID == c.User.ID || generalFolders[p.Folder] {
			continue
		}
		topic := 0.0
		for st := range q {
			if docs[i][st] {
				topic += weight(st)
			}
		}
		if topic == 0 {
			continue
		}
		get(p.AuthorID).pageScores = append(get(p.AuthorID).pageScores, recencyWeight(c, p.UpdatedAt)*topic)
	}
	for _, u := range c.Store.Data.Team.Users {
		if u.ID == c.User.ID || store.HasRole(&u, store.RoleIntern) {
			continue
		}
		if n := overlap(q, Stems(u.Responsibility)); n > 0 {
			get(u.ID).resp = n
		}
	}
	score := func(cd *cand) float64 {
		ps := append([]float64(nil), cd.pageScores...)
		sort.Sort(sort.Reverse(sort.Float64Slice(ps)))
		total := 1.5 * float64(cd.resp)
		for i := 0; i < len(ps) && i < 2; i++ { // две лучшие страницы: число страниц само по себе не экспертиза
			total += ps[i]
		}
		return total
	}
	var best, second float64
	for uid, cd := range cands {
		if sc := score(cd); sc > best {
			second, best, id = best, sc, uid
		} else if sc > second {
			second = sc
		}
	}
	if id == "" {
		return "", 0, ""
	}
	// Уверенность: насколько лидер отрывается от второго, с поправкой на силу сигнала
	// (одно случайное совпадение слова — ещё не экспертиза).
	confidence = best / (best + secondWeight*second) * min(1, best/MinExpertEvidence)
	cd := cands[id]
	pages := len(cd.pageScores)
	switch {
	case pages > 0 && cd.resp > 0:
		reason = fmt.Sprintf("автор страниц по теме (%d) и это его зона ответственности", pages)
	case pages > 0:
		reason = fmt.Sprintf("автор страниц по теме (%d)", pages)
	default:
		reason = "это его зона ответственности"
	}
	return id, confidence, reason
}

func findExpert(c Ctx, args map[string]any) Result {
	question := str(args, "question")
	pick := cached(c, "expert", question, func() expertPick {
		id, conf, reason := FindExpert(c, question)
		return expertPick{id, conf, reason}
	})
	id, conf, reason := pick.ID, pick.Confidence, pick.Reason
	// BR-10a: топ-руководителя стажёру напрямую не предлагаем — вопрос уходит бадди. Имя руководителя
	// в ответ инструменту не попадает, чтобы модель не назвала его стажёру; подсказку бадди добавит send_question.
	if id != "" && isSenior(c, id) {
		buddy := c.User.BuddyID
		return ok(map[string]any{"addressee_id": buddy, "name": c.Store.Name(buddy), "is_buddy": true,
			"reason": "по этой теме лучше начать с бадди — он подскажет, к кому идти", "confidence": conf})
	}
	if id == "" || conf < ExpertThreshold {
		buddy := c.User.BuddyID
		return ok(map[string]any{"addressee_id": buddy, "name": c.Store.Name(buddy), "is_buddy": true,
			"reason": "уверенного эксперта нет — спросим бадди", "confidence": conf})
	}
	return ok(map[string]any{"addressee_id": id, "name": c.Store.Name(id), "is_buddy": false, "reason": reason, "confidence": conf})
}

// isSenior — руководитель уровня из конфига отдела (BR-10a: директора, руководители руководителей).
func isSenior(c Ctx, userID string) bool {
	u, ok := c.Store.User(userID)
	return ok && u.ManagementLevel != "" && containsStr(c.Store.Data.Team.Department.Routing.SeniorManagementLevels, u.ManagementLevel)
}

// seniorBlock — стажёр не пишет топ-руководителю напрямую, даже если модель выбрала его в обход подбора (BR-10a).
func seniorBlock(c Ctx, addressee string) *Error {
	if store.HasRole(c.User, store.RoleIntern) && isSenior(c, addressee) {
		return &Error{Code: "validation", Message: "Стажёр не отправляет вопрос этому руководителю напрямую (BR-10a). Отправь вопрос бадди: addressee_id=" +
			c.User.BuddyID + ", имя руководителя стажёру не называй."}
	}
	return nil
}

// seniorHint — подсказка бадди, если лучший кандидат по теме — топ-руководитель (BR-10a).
func seniorHint(c Ctx, text string) string {
	if id, _, _ := FindExpert(c, text); id != "" && isSenior(c, id) {
		u, _ := c.Store.User(id)
		return "По теме больше всех писал " + u.Name + ", " + strings.ToLower(strings.Split(u.Position, " (")[0]) + ". Ответь сам или переадресуй, если считаешь нужным."
	}
	return ""
}

func sendQuestion(c Ctx, args map[string]any) Result {
	to, found := c.Store.User(str(args, "addressee_id"))
	if !found {
		return fail("not_found", "Адресат не найден в команде.")
	}
	text := str(args, "text")
	if text == "" {
		return fail("validation", "Пустой вопрос.")
	}
	if e := seniorBlock(c, to.ID); e != nil {
		return Result{Error: e}
	}
	q := &store.Question{ID: c.Store.NextID("Q"), AskedBy: c.User.ID, AddresseeID: to.ID, Title: QuestionTitle(str(args, "title"), text), Text: text,
		Reason: str(args, "reason"), Urgent: boolean(args, "urgent"), Status: "Sent"}
	if to.ID == c.User.BuddyID { // BR-10a: подсказка видна только бадди
		q.Hint = seniorHint(c, q.Title+" "+text)
	}
	c.Store.AddQuestion(q)
	c.Store.Emit(store.EvQuestionSent, c.User.ID, q.ID, map[string]string{"to": to.ID, "urgent": fmt.Sprint(q.Urgent)})
	delivery := "в сводке"
	if q.Urgent {
		delivery = "сразу (срочно)"
	}
	return Result{OK: true, Data: q, Receipt: map[string]string{"id": q.ID, "status": "Sent", "to": to.Name, "delivery": delivery}}
}

// recencyWeight — BR-10: свежие страницы весят больше, давно не обновлённые — слабый сигнал.
func recencyWeight(c Ctx, updatedAt string) float64 {
	upd, err := time.Parse(time.DateOnly, updatedAt)
	if err != nil {
		return 1
	}
	age := c.Store.Today.Sub(upd)
	switch {
	case age <= 30*24*time.Hour:
		return 2
	case age <= 180*24*time.Hour:
		return 1
	case age <= 365*24*time.Hour:
		return 0.6
	default:
		return 0.3
	}
}

// PageCard — страница базы для интерфейса: начало текста для карточки и полный текст для просмотра.
type PageCard struct {
	ID              string `json:"id"`
	Folder          string `json:"folder"`
	Type            string `json:"type"`
	Title           string `json:"title"`
	Excerpt         string `json:"excerpt"`
	Body            string `json:"body"`
	Author          string `json:"author"`
	UpdatedAt       string `json:"updated_at"`
	TestInstruction bool   `json:"test_instruction"`
}

// KBPages — папки и опубликованные страницы своего пространства (BR-02, BR-40).
func KBPages(c Ctx) (folders []mockdata.Folder, pages []PageCard) {
	spaces := c.Store.ReadableSpaces()
	for _, f := range c.Store.Data.KB.Folders {
		if spaces[f.Space] {
			folders = append(folders, f)
		}
	}
	for _, p := range readablePages(c) {
		ex := []rune(p.Body)
		if len(ex) > 140 {
			ex = append(ex[:140], '…')
		}
		pages = append(pages, PageCard{p.ID, p.Folder, p.Type, p.Title, string(ex), p.Body, c.Store.Name(p.AuthorID), p.UpdatedAt, p.TestInstruction})
	}
	return folders, pages
}

// QuestionTitle — суть вопроса одной строкой. Если модель не передала title, берём строку «Суть: …»
// или первую содержательную строку без приветствия, не длиннее 120 символов.
func QuestionTitle(title, text string) string {
	if t := strings.TrimSpace(title); t != "" {
		return clip(t)
	}
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if rest, ok := strings.CutPrefix(l, "Суть:"); ok && strings.TrimSpace(rest) != "" {
			return clip(strings.TrimSpace(rest))
		}
	}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		low := strings.ToLower(l)
		if l == "" || strings.HasPrefix(low, "привет") || strings.HasPrefix(low, "здравствуй") || strings.HasPrefix(low, "добрый") {
			continue
		}
		return clip(l)
	}
	return clip(strings.TrimSpace(text))
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}
