package tools

import "github.com/worker4556/naparnik/backend/internal/store"

// Feedback — BR-02, BR-40: оценить можно только страницу, доступную в базе и поиске.
// Проверка и событие выполняются в одной транзакции; отказ не создаёт уведомлений.
func Feedback(c Ctx, pageID, value string) Result {
	if pageID == "" || (value != "helped" && value != "not_helped" && value != "stale") {
		return fail("validation", "Нужны page_id и оценка: helped, not_helped или stale.")
	}
	return atomicResult(c, func(c Ctx) Result {
		for _, p := range readablePages(c) {
			if p.ID == pageID {
				c.Store.Emit(store.EvAnswerFeedback, c.User.ID, p.ID, map[string]string{
					"value": value, "author": p.AuthorID, "page_title": p.Title,
				})
				return ok(nil)
			}
		}
		// Одинаковый ответ для отсутствующей, закрытой и неопубликованной страницы.
		return fail("not_found", "Страница не найдена.")
	})
}
