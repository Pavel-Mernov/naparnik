package tools

import (
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Поддержка базы (SC-22, F8, P5, SCR-L3): владельцу базы — открытые вопросы, черновики без «Ок»,
// страницы, отмеченные устаревшими, и пары похожих страниц от плановой проверки (BR-38) с решением
// «объединить» (старые ведут на общую, авторы уведомлены, BR-39) или «не дубль».

func isKBOwner(c Ctx) bool { return c.User.ID == c.Store.Data.Team.Department.KBOwnerID }

type OpenItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	AskedBy string `json:"asked_by"`
	Reason  string `json:"reason"`
	Seed    bool   `json:"seed,omitempty"` // из стартовых данных: назначение создаст вопрос
}

type DraftItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	AuthorID string `json:"author_id"`
	Since    string `json:"since"`
}

type StaleItem struct {
	PageID   string `json:"page_id"`
	Title    string `json:"title"`
	AuthorID string `json:"author_id"`
	Marks    int    `json:"marks"`
}

type DuplicateItem struct {
	ID            string   `json:"id"`
	Pages         []string `json:"pages"`
	Titles        []string `json:"titles"`
	Authors       []string `json:"authors"`
	ProposedTitle string   `json:"proposed_title"`
	Note          string   `json:"note,omitempty"` // почему пара может быть не дублем
	Decision      string   `json:"decision,omitempty"`
}

type Maintenance struct {
	Open       []OpenItem      `json:"open_questions"`
	Drafts     []DraftItem     `json:"drafts"`
	Stale      []StaleItem     `json:"stale"`
	Duplicates []DuplicateItem `json:"duplicates"`
	NextCheck  string          `json:"next_check"`
}

func pageByID(c Ctx, id string) (mockdata.Page, bool) {
	for _, p := range c.Store.Pages() {
		if p.ID == id {
			return p, true
		}
	}
	return mockdata.Page{}, false
}

// KBMaintenance — всё, что требует внимания владельца базы.
func KBMaintenance(c Ctx) (Maintenance, *Error) {
	if !isKBOwner(c) {
		return Maintenance{}, &Error{Code: "forbidden", Message: "Поддержка базы — у владельца базы."}
	}
	seed := c.Store.Data.KB.Maintenance
	out := Maintenance{Open: []OpenItem{}, Drafts: []DraftItem{}, Stale: []StaleItem{}, Duplicates: []DuplicateItem{}, NextCheck: seed.NextScheduledCheck}
	for _, q := range c.Store.Questions() {
		if q.Status == "Open" {
			out.Open = append(out.Open, OpenItem{ID: q.ID, Title: q.Title, AskedBy: q.AskedBy, Reason: "Встреча без записи, ответ текстом не дан"})
		}
	}
	for _, s := range seed.OpenQuestions {
		if c.Store.KBDecision("seed:"+s.ID) == "" {
			out.Open = append(out.Open, OpenItem{ID: s.ID, Title: s.Text, AskedBy: s.AskedBy, Reason: s.Reason, Seed: true})
		}
	}
	for _, p := range c.Store.Pages() {
		if p.Status == "Draft" {
			out.Drafts = append(out.Drafts, DraftItem{ID: p.ID, Title: p.Title, AuthorID: p.AuthorID, Since: p.UpdatedAt})
		}
	}
	for _, d := range seed.UnconfirmedDrafts {
		out.Drafts = append(out.Drafts, DraftItem{ID: d.ID, Title: d.Title, AuthorID: d.AuthorID, Since: d.CreatedAt})
	}
	marks := map[string]int{}
	for _, e := range c.Store.Events() {
		if e.Type == store.EvAnswerFeedback && e.Meta["value"] == "stale" {
			marks[e.Object]++
		}
	}
	for id, n := range marks {
		if p, ok := pageByID(c, id); ok && p.Status == "Published" {
			out.Stale = append(out.Stale, StaleItem{PageID: id, Title: p.Title, AuthorID: p.AuthorID, Marks: n})
		}
	}
	for _, d := range seed.DuplicatePairs {
		item := DuplicateItem{ID: d.ID, Pages: d.Pages, ProposedTitle: d.ProposedTitle, Note: d.Reason, Decision: c.Store.KBDecision("dup:" + d.ID)}
		for _, id := range d.Pages {
			p, _ := pageByID(c, id)
			item.Titles, item.Authors = append(item.Titles, p.Title), append(item.Authors, p.AuthorID)
		}
		out.Duplicates = append(out.Duplicates, item)
	}
	return out, nil
}

// DecideDuplicate — «объединить»: первая страница получает общий заголовок и текст обеих, вторая ведёт на неё
// и не попадает в поиск; авторы уведомлены (BR-39). «не дубль» — пара больше не предлагается.
func DecideDuplicate(c Ctx, pairID, decision string) (DuplicateItem, *Error) {
	return atomic(c, func(c Ctx) (DuplicateItem, *Error) {
		if !isKBOwner(c) {
			return DuplicateItem{}, &Error{Code: "forbidden", Message: "Решение по дублям принимает владелец базы."}
		}
		if decision != "merge" && decision != "not_duplicate" {
			return DuplicateItem{}, &Error{Code: "validation", Message: "Решение: merge или not_duplicate."}
		}
		var pair *DuplicateItem
		m, _ := KBMaintenance(c)
		for i := range m.Duplicates {
			if m.Duplicates[i].ID == pairID {
				pair = &m.Duplicates[i]
			}
		}
		switch {
		case pair == nil:
			return DuplicateItem{}, &Error{Code: "not_found", Message: "Пара не найдена."}
		case pair.Decision != "":
			return *pair, &Error{Code: "validation", Message: "По этой паре уже есть решение."}
		}
		if decision == "merge" {
			a, _ := pageByID(c, pair.Pages[0])
			b, _ := pageByID(c, pair.Pages[1])
			c.Store.UpdatePage(a.ID, func(p *mockdata.Page) {
				if pair.ProposedTitle != "" {
					p.Title = pair.ProposedTitle
				}
				p.Body = strings.TrimSpace(a.Body) + "\n\n" + strings.TrimSpace(b.Body)
				p.UpdatedAt = c.Store.Today.Format(time.DateOnly)
			})
			c.Store.UpdatePage(b.ID, func(p *mockdata.Page) { p.Status, p.MergedInto = "Merged", a.ID })
			c.Store.Emit(store.EvPagesMerged, c.User.ID, a.ID, map[string]string{"merged": b.ID, "authors": strings.Join(pair.Authors, ",")})
		}
		c.Store.SetKBDecision("dup:"+pairID, decision)
		pair.Decision = decision
		return *pair, nil
	})
}

// AssignOpen — владелец базы назначает, кто ответит на открытый вопрос (BR-37). Вопрос из стартовых данных
// при первом назначении становится обычным вопросом стажёра.
func AssignOpen(c Ctx, questionID, to string) (store.Question, *Error) {
	if !isKBOwner(c) {
		return store.Question{}, &Error{Code: "forbidden", Message: "Назначает владелец базы."}
	}
	for _, s := range c.Store.Data.KB.Maintenance.OpenQuestions {
		if s.ID != questionID {
			continue
		}
		created, e := atomic(c, func(c Ctx) (store.Question, *Error) {
			if c.Store.KBDecision("seed:"+s.ID) != "" {
				return store.Question{}, &Error{Code: "validation", Message: "Этот вопрос уже назначен."}
			}
			q := &store.Question{ID: c.Store.NextID("Q"), AskedBy: s.AskedBy, AddresseeID: c.User.ID, Title: QuestionTitle("", s.Text), Text: s.Text,
				Reason: s.Reason, Status: "Open"}
			c.Store.AddQuestion(q)
			c.Store.SetKBDecision("seed:"+s.ID, q.ID)
			return *q, nil
		})
		if e != nil {
			return created, e
		}
		questionID = created.ID
	}
	return Redirect(c, questionID, to)
}

// RemindDraft — одно напоминание автору черновика: без «Ок» страница не появится в базе (BR-33).
func RemindDraft(c Ctx, draftID string) *Error {
	if !isKBOwner(c) {
		return &Error{Code: "forbidden", Message: "Напоминает владелец базы."}
	}
	m, _ := KBMaintenance(c)
	for _, d := range m.Drafts {
		if d.ID == draftID {
			return asErr(c.Store.Transaction(func(s *store.Store) error {
				s.Notify(store.Notification{ID: "remind:" + d.ID + ":" + c.Store.Today.Format(time.DateOnly), UserID: d.AuthorID, Type: "draft_review",
					View: "questions", TG: "pending",
					Text: "📄 Черновик страницы для базы знаний ждёт твоего «Ок».\nЭто минута: проверь текст и опубликуй — следующий стажёр получит ответ сразу."})
				return nil
			}))
		}
	}
	return &Error{Code: "not_found", Message: "Черновик не найден."}
}

func asErr(err error) *Error {
	if err != nil {
		return &Error{Code: "unavailable", Message: "Не удалось сохранить. Повтори запрос."}
	}
	return nil
}
