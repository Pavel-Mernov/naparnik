package agent

import (
	"regexp"
	"strings"
)

// Проверки ДО модели: такие сообщения вообще не уходят во внешний LLM.

// BR-61, BR-72: чувствительные ПДн и финансы не обрабатываются и не передаются в LLM.
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`), // номер карты
	regexp.MustCompile(`\b\d{2}\s?\d{2}\s?\d{6}\b`),                  // серия и номер паспорта
	regexp.MustCompile(`\b\d{3}-\d{3}-\d{3}\s?\d{2}\b`),              // СНИЛС
	regexp.MustCompile(`(?i)паспорт|номер карты|реквизит|снилс|диагноз|банковск`),
}

// BR-60: оценка, выход в штат, зарплата, конфликты — решают люди.
var boundaryPattern = regexp.MustCompile(`(?i)в штат|оценк[аиуе]|меня оценива|оценивает меня|зарплат|оклад|сколько (получа|плат)|уволят|конфликт|повышени`)

const (
	piiAnswer = "Такие данные я не обрабатываю и никуда не сохраняю. Паспортные и банковские данные вноси только в HR-сервисе — " +
		"там они защищены. Если нужна помощь с заявкой, напиши, что именно оформить, без самих данных."
	boundaryAnswer = "Это решают люди, а не я. Лучше обсудить с бадди или руководителем — могу подготовить вопрос, " +
		"а отправишь его ты."
)

func containsPII(text string) bool {
	for _, re := range piiPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// MaskPII — для логов и журнала событий (BR-72).
func MaskPII(text string) string {
	for _, re := range piiPatterns[:3] {
		text = re.ReplaceAllString(text, "***")
	}
	return text
}

// guard возвращает готовый ответ, если сообщение нельзя отправлять в модель. people — к кому идти
// с вопросом о границах (BR-60): стажёру называем бадди и руководителя по имени, а не «кого-нибудь».
func guard(text, people string) (string, bool) {
	switch {
	case containsPII(text):
		return piiAnswer, true
	case boundaryPattern.MatchString(strings.ToLower(text)):
		if people != "" {
			return "Это решают люди, а не я. Лучше обсудить лично: " + people + ". Могу подготовить вопрос, а отправишь его ты.", true
		}
		return boundaryAnswer, true
	}
	return "", false
}
