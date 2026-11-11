package tools

import (
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// P2 «Ответ текстом → страница в базе» (docs/03-requirements/03-PROCESSES.md).
// Это действия людей, а не инструменты модели: отвечает и публикует только человек.

const defaultFolder = "f-start"

// AnswerQuestion — адресат отвечает текстом (US-10). Ответ сразу уходит стажёру,
// а Напарник готовит черновик страницы «Вопрос-ответ» (BR-31): папка — по похожим страницам,
// похожая страница — предложение дополнить её (BR-32). Черновик не виден в поиске до «Ок» автора (BR-02, BR-30).
type answerResult struct {
	question store.Question
	page     mockdata.Page
}

func AnswerQuestion(c Ctx, questionID, text string) (store.Question, mockdata.Page, *Error) {
	r, e := atomic(c, func(c Ctx) (answerResult, *Error) {
		q, p, e := answerQuestion(c, questionID, text)
		return answerResult{q, p}, e
	})
	return r.question, r.page, e
}

func answerQuestion(c Ctx, questionID, text string) (store.Question, mockdata.Page, *Error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return store.Question{}, mockdata.Page{}, &Error{Code: "validation", Message: "Пустой ответ."}
	}
	var q store.Question
	for _, x := range c.Store.Questions() {
		if x.ID == questionID {
			q = x
		}
	}
	if q.ID == "" {
		return store.Question{}, mockdata.Page{}, &Error{Code: "not_found", Message: "Вопрос не найден."}
	}
	if q.AddresseeID != c.User.ID {
		return store.Question{}, mockdata.Page{}, &Error{Code: "forbidden", Message: "Ответить может только адресат вопроса."}
	}
	if q.Status == "Answered" && q.Answer == text {
		for _, p := range c.Store.Pages() {
			if p.ID == q.PageID {
				return q, p, nil
			}
		}
	}
	if q.Status != "Sent" {
		return store.Question{}, mockdata.Page{}, &Error{Code: "validation", Message: "На вопрос уже ответили."}
	}
	title := q.Title
	if title == "" {
		title = QuestionTitle("", q.Text)
	}
	folder, similar := placePage(c, title)
	page := mockdata.Page{
		ID: nextPageID(c), Space: c.Store.Data.Team.Department.KBSpaceID, Folder: folder, Type: "qa", Status: "Draft",
		Title: title, AuthorID: c.User.ID,
		UpdatedAt: c.Store.Today.Format(time.DateOnly), Body: text, SourceQuestion: q.ID, SimilarPageID: similar,
	}
	c.Store.AddPage(page)
	q, _ = c.Store.UpdateQuestion(q.ID, func(x *store.Question) {
		x.Status, x.Answer, x.AnsweredBy, x.AnsweredAt, x.PageID = "Answered", text, c.User.ID, time.Now().Format(time.RFC3339), page.ID
	})
	c.Store.Emit(store.EvQuestionAnswered, c.User.ID, q.ID, map[string]string{"asker": q.AskedBy, "page": page.ID})
	return q, page, nil
}

// nextPageID — следующий свободный номер страницы: в мок-данных есть страницы и с номерами выше kb-20.
func nextPageID(c Ctx) string {
	taken := map[string]bool{}
	for _, p := range c.Store.Pages() {
		taken[p.ID] = true
	}
	for {
		if id := c.Store.NextID("kb"); !taken[id] {
			return id
		}
	}
}

// placePage — папка по самой похожей опубликованной странице и похожая страница для BR-32.
func placePage(c Ctx, question string) (folder, similar string) {
	q := Stems(question)
	best := 0.0
	folder = defaultFolder
	for _, p := range readablePages(c) {
		if len(q) == 0 {
			break
		}
		cov := float64(overlap(q, Stems(p.Title+" "+p.Body))) / float64(len(q))
		if cov > best {
			best, folder = cov, p.Folder
			if cov >= MinCoverage {
				similar = p.ID
			}
		}
	}
	return folder, similar
}

// DraftsFor — черновики страниц, ожидающие проверки автора («Проверь, я занёс твой ответ в базу»).
func DraftsFor(c Ctx) []mockdata.Page {
	var out []mockdata.Page
	for _, p := range c.Store.Pages() {
		if p.Status == "Draft" && p.AuthorID == c.User.ID {
			out = append(out, p)
		}
	}
	return out
}

// ApprovePage — автор подтверждает черновик (можно с правками) → страница опубликована под его именем (BR-30).
func ApprovePage(c Ctx, pageID, title, body string) (mockdata.Page, *Error) {
	return atomic(c, func(c Ctx) (mockdata.Page, *Error) { return approvePage(c, pageID, title, body) })
}

func approvePage(c Ctx, pageID, title, body string) (mockdata.Page, *Error) {
	var found *mockdata.Page
	for _, p := range c.Store.Pages() {
		if p.ID == pageID {
			found = &p
		}
	}
	switch {
	case found == nil:
		return mockdata.Page{}, &Error{Code: "not_found", Message: "Черновик не найден."}
	case found.AuthorID != c.User.ID:
		return mockdata.Page{}, &Error{Code: "forbidden", Message: "Опубликовать может только автор ответа."}
	case found.Status == "Published" && (strings.TrimSpace(title) == "" || strings.TrimSpace(title) == found.Title) && (strings.TrimSpace(body) == "" || strings.TrimSpace(body) == found.Body):
		return *found, nil
	case found.Status != "Draft":
		return mockdata.Page{}, &Error{Code: "validation", Message: "Страница уже опубликована."}
	}
	page, _ := c.Store.UpdatePage(pageID, func(p *mockdata.Page) {
		if t := strings.TrimSpace(title); t != "" {
			p.Title = t
		}
		if b := strings.TrimSpace(body); b != "" {
			p.Body = b
		}
		p.Status, p.UpdatedAt = "Published", c.Store.Today.Format(time.DateOnly)
	})
	c.Store.Emit(store.EvPagePublished, c.User.ID, page.ID, map[string]string{"source_question": page.SourceQuestion})
	return page, nil
}
