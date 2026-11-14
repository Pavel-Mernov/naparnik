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

func isKBOwner(c Ctx) bool { return c.User.ID == c.Store.KBOwner() }

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
	LastCheck  string          `json:"last_check,omitempty"`
	OwnerID    string          `json:"owner_id"`
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
	out := Maintenance{Open: []OpenItem{}, Drafts: []DraftItem{}, Stale: []StaleItem{}, Duplicates: []DuplicateItem{},
		NextCheck: c.Store.NextKBCheck(), LastCheck: c.Store.LastKBCheck(), OwnerID: c.Store.KBOwner()}
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
	// BR-33: черновик попадает сюда, когда автор не дал «Ок» 6 рабочих дней (после напоминания на 3-й).
	now := time.Now()
	for _, d := range c.Store.KBDrafts() {
		if c.Store.DraftOverdue(d, now) {
			out.Drafts = append(out.Drafts, DraftItem{ID: d.ID, Title: d.Title, AuthorID: d.AuthorID, Since: d.Since.In(store.MSK).Format(time.DateOnly)})
		}
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
	// Пары, найденные плановой проверкой (BR-38): общий заголовок — у страницы с более полным текстом.
	for _, d := range c.Store.AutoPairs() {
		item := DuplicateItem{ID: d.ID, Pages: d.Pages, Decision: c.Store.KBDecision("dup:" + d.ID),
			Note: "Похожи заголовком и текстом — проверь, одна ли это тема"}
		var longest mockdata.Page
		for _, id := range d.Pages {
			p, _ := pageByID(c, id)
			item.Titles, item.Authors = append(item.Titles, p.Title), append(item.Authors, p.AuthorID)
			if len(p.Body) > len(longest.Body) {
				longest = p
			}
		}
		if item.Decision == "" && len(item.Titles) == 2 && (item.Titles[0] == "" || item.Titles[1] == "") {
			continue // одна из страниц уже объединена или снята
		}
		item.ProposedTitle = longest.Title
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
				p.Classification = mockdata.StricterClass(a.Classification, b.Classification) // BR-49
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

// CheckKBNow — владелец запускает плановую проверку не дожидаясь даты (BR-38).
func CheckKBNow(c Ctx) (Maintenance, *Error) {
	if !isKBOwner(c) {
		return Maintenance{}, &Error{Code: "forbidden", Message: "Проверку запускает владелец базы."}
	}
	if e := asErr(c.Store.Transaction(func(s *store.Store) error { s.CheckKB(time.Now()); return nil })); e != nil {
		return Maintenance{}, e
	}
	return KBMaintenance(c)
}

// KBOwnerInfo — кто владелец базы и может ли этот человек его сменить.
type KBOwnerInfo struct {
	OwnerID   string `json:"owner_id"`
	CanChange bool   `json:"can_change"`
	IsOwner   bool   `json:"is_owner"`
	DefaultID string `json:"default_id"`
}

func canChangeOwner(c Ctx) bool { return c.User.ID == c.Store.Data.Team.Department.LeadID }

func KBOwner(c Ctx) KBOwnerInfo {
	return KBOwnerInfo{OwnerID: c.Store.KBOwner(), CanChange: canChangeOwner(c), IsOwner: isKBOwner(c), DefaultID: c.Store.Data.Team.Department.KBOwnerID}
}

// SetKBOwner — BR-34: владелец по умолчанию — руководитель отдела; он может назначить другого сотрудника.
// Стажёр владельцем быть не может: ему разбирать чужие вопросы и дубли рано.
func SetKBOwner(c Ctx, userID string) (KBOwnerInfo, *Error) {
	return atomic(c, func(c Ctx) (KBOwnerInfo, *Error) {
		if !canChangeOwner(c) {
			return KBOwnerInfo{}, &Error{Code: "forbidden", Message: "Владельца базы назначает руководитель отдела.", WhoCan: c.Store.Name(c.Store.Data.Team.Department.LeadID)}
		}
		u, ok := c.Store.User(userID)
		if !ok {
			return KBOwnerInfo{}, &Error{Code: "not_found", Message: "Сотрудник не найден."}
		}
		if store.HasRole(u, store.RoleIntern) {
			return KBOwnerInfo{}, &Error{Code: "validation", Message: "Стажёр не может быть владельцем базы."}
		}
		prev := c.Store.KBOwner()
		if prev == userID {
			return KBOwner(c), nil
		}
		c.Store.SetKBOwner(userID)
		if userID != c.User.ID {
			c.Store.Notify(store.Notification{ID: "kbowner:" + userID + ":" + time.Now().Format(time.RFC3339), UserID: userID, Type: "kb_owner", View: "knowledge", TG: "pending",
				Text: "📚 " + c.Store.Name(c.User.ID) + " назначил(а) тебя владельцем базы знаний отдела.\nЧто это: открытые вопросы, черновики без «Ок» и дубли теперь приходят тебе — смотри «Поддержку базы» в разделе «База знаний»."})
		}
		return KBOwner(c), nil
	})
}
