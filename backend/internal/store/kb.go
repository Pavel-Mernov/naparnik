package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// База знаний: владелец (BR-34), черновики без «Ок» (BR-33), плановый поиск дублей (BR-38).

const (
	DraftRemindDays = 3 // BR-33: через 3 рабочих дня — одно напоминание автору
	DraftEscalate   = 6 // ещё через 3 — черновик уходит в «Поддержку базы» владельцу
	CheckEveryMonth = 4 // BR-38: плановая проверка раз в 4 месяца
	DupThreshold    = 0.25
)

// KBOwner — владелец базы: по умолчанию руководитель отдела, он может назначить другого (BR-34).
func (s *Store) KBOwner() string {
	if id := s.KBDecision("owner"); id != "" {
		return id
	}
	return s.Data.Team.Department.KBOwnerID
}

func (s *Store) SetKBOwner(id string) { s.SetKBDecision("owner", id) }

// NextKBCheck — дата следующей плановой проверки; LastKBCheck — последней (пусто, если не было).
func (s *Store) NextKBCheck() string {
	if d := s.KBDecision("next_check"); d != "" {
		return d
	}
	return s.Data.KB.Maintenance.NextScheduledCheck
}

func (s *Store) LastKBCheck() string { return s.KBDecision("last_check") }

// KBDraft — черновик страницы, который ждёт «Ок» автора.
type KBDraft struct {
	ID, Title, AuthorID string
	Since               time.Time
}

// KBDrafts — черновики из базы и из стартовых данных.
func (s *Store) KBDrafts() []KBDraft {
	var out []KBDraft
	for _, p := range s.Pages() {
		if p.Status == "Draft" {
			out = append(out, KBDraft{p.ID, p.Title, p.AuthorID, parseDay(p.UpdatedAt)})
		}
	}
	for _, d := range s.Data.KB.Maintenance.UnconfirmedDrafts {
		out = append(out, KBDraft{d.ID, d.Title, d.AuthorID, parseDay(d.CreatedAt)})
	}
	return out
}

func parseDay(v string) time.Time {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	t, _ := time.ParseInLocation(time.DateOnly, v, MSK)
	return t
}

// DraftOverdue — черновик без «Ок» дольше DraftEscalate рабочих дней: место ему в «Поддержке базы».
func (s *Store) DraftOverdue(d KBDraft, now time.Time) bool {
	return !d.Since.IsZero() && !now.Before(s.draftDeadline(d, DraftEscalate))
}

func (s *Store) draftDeadline(d KBDraft, days int) time.Time {
	hours := ""
	if u, ok := s.User(d.AuthorID); ok {
		hours = u.WorkingHours
	}
	return AddWorking(hours, d.Since, time.Duration(days)*WorkingDay(hours))
}

// watchKBDrafts — BR-33: автору одно напоминание, затем владельцу базы.
func (s *Store) watchKBDrafts(now time.Time) {
	for _, d := range s.KBDrafts() {
		if d.Since.IsZero() || now.Before(s.draftDeadline(d, DraftRemindDays)) {
			continue
		}
		s.Notify(Notification{ID: "kbdraft1:" + d.ID, UserID: d.AuthorID, Type: "draft_review", View: "questions", TG: "pending",
			Text: "📄 Черновик страницы для базы знаний ждёт твоего «Ок» уже 3 рабочих дня.\nЭто минута: проверь текст и опубликуй — следующий стажёр получит ответ сразу."})
		if s.DraftOverdue(d, now) && d.AuthorID != s.KBOwner() {
			s.Notify(Notification{ID: "kbdraft2:" + d.ID, UserID: s.KBOwner(), Type: "draft_review", View: "knowledge", TG: "pending",
				Text: fmt.Sprintf("📄 Черновик автора %s без «Ок» уже 6 рабочих дней.\nЧто сделать: открой «Поддержку базы» в разделе «База знаний» — напомни автору или реши, нужна ли страница.", s.Name(d.AuthorID))})
		}
	}
}

// DupPair — пара похожих страниц от плановой проверки.
type DupPair struct {
	ID    string   `json:"id"`
	Pages []string `json:"pages"`
	Score float64  `json:"score"`
}

var wordRe = regexp.MustCompile(`[\p{L}\d]+`)

var stopWords = map[string]bool{"как": true, "для": true, "что": true, "где": true, "это": true, "или": true, "при": true, "через": true, "если": true}

// stems — слова от трёх букв, обрезанные до пяти: «доступ», «доступа», «доступы» — одно слово.
func stems(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
		r := []rune(w)
		if len(r) < 3 || stopWords[w] {
			continue
		}
		out[string(r[:min(5, len(r))])] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return float64(n) / float64(len(a)+len(b)-n)
}

// FindDuplicates — пары опубликованных страниц, похожих заголовком и текстом. Заголовок весит больше:
// у страниц про разное часто общие слова отдела, а одинаковая тема видна по заголовку.
func (s *Store) FindDuplicates() []DupPair {
	var pages []struct {
		id           string
		title, whole map[string]bool
	}
	for _, p := range s.Pages() {
		if p.Status == "Published" {
			pages = append(pages, struct {
				id           string
				title, whole map[string]bool
			}{p.ID, stems(p.Title), stems(p.Title + " " + p.Body)})
		}
	}
	var out []DupPair
	for i := range pages {
		for j := i + 1; j < len(pages); j++ {
			score := 0.6*jaccard(pages[i].title, pages[j].title) + 0.4*jaccard(pages[i].whole, pages[j].whole)
			if score >= DupThreshold {
				a, b := pages[i].id, pages[j].id
				out = append(out, DupPair{ID: "auto-" + a + "-" + b, Pages: []string{a, b}, Score: score})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// CheckKB — плановая проверка (BR-38): находит пары похожих страниц, ставит следующую дату и присылает
// владельцу сводку. Пары, по которым уже есть решение, повторно не предлагаются.
func (s *Store) CheckKB(now time.Time) []DupPair {
	known := map[string]bool{}
	for _, d := range s.Data.KB.Maintenance.DuplicatePairs {
		known[strings.Join(d.Pages, "|")] = true
	}
	var fresh []DupPair
	for _, p := range s.FindDuplicates() {
		if known[strings.Join(p.Pages, "|")] || s.KBDecision("dup:"+p.ID) != "" {
			continue
		}
		s.SetKBDecision("pair:"+p.ID, strings.Join(p.Pages, "|"))
		fresh = append(fresh, p)
	}
	day := now.In(MSK).Format(time.DateOnly)
	s.SetKBDecision("last_check", day)
	s.SetKBDecision("next_check", now.In(MSK).AddDate(0, CheckEveryMonth, 0).Format(time.DateOnly))
	open := 0
	for _, q := range s.Questions() {
		if q.Status == "Open" {
			open++
		}
	}
	s.Notify(Notification{ID: "kbcheck:" + day, UserID: s.KBOwner(), Type: "kb_check", View: "knowledge", TG: "pending",
		Text: fmt.Sprintf("🔎 Плановая проверка базы знаний: новых пар похожих страниц — %d, открытых вопросов — %d.\nЧто сделать: открой «Поддержку базы» в разделе «База знаний» и реши по каждой паре: объединить или не дубль.", len(fresh), open)})
	return fresh
}

// AutoPairs — пары, найденные проверками (сохранены, чтобы решение «не дубль» не терялось).
func (s *Store) AutoPairs() []DupPair {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []DupPair
	for k, v := range s.kbDecisions {
		if id, ok := strings.CutPrefix(k, "pair:"); ok {
			out = append(out, DupPair{ID: id, Pages: strings.Split(v, "|")})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// watchKBCheck — пора плановой проверки (BR-38).
func (s *Store) watchKBCheck(now time.Time) {
	next, err := time.ParseInLocation(time.DateOnly, s.NextKBCheck(), MSK)
	if err == nil && !now.Before(next) {
		s.CheckKB(now)
	}
}
