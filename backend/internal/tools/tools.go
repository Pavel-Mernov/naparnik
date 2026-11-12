// Package tools — инструменты агента AI-buddy.
//
// Правила из docs/03-requirements, которые проверяет код (а не промпт):
//   - BR-40: права проверяются до вызова; недоступное не показывается и не упоминается;
//   - пользователь берётся из сессии, аргументам модели «кто я» не доверяем;
//   - BR-11, BR-43: инструменты с Confirm=true не выполняются по решению модели — агент делает черновик,
//     выполнение только после подтверждения человеком (лестница доверия);
//   - BR-01, BR-02, BR-04, BR-05: ответы из базы — только опубликованные страницы своего пространства,
//     с источником, датой, автором и пометкой тестовой инструкции; слабое совпадение = «не нашёл».
package tools

import (
	"github.com/worker4556/naparnik/backend/internal/flags"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

const TestInstructionLabel = "тестовая инструкция, не регламент Авито"

type Error struct {
	Code    string `json:"code"` // forbidden | not_found | validation | unavailable | conflict
	Message string `json:"message"`
	WhoCan  string `json:"who_can,omitempty"`
}

type Result struct {
	OK      bool   `json:"ok"`
	Data    any    `json:"data,omitempty"`
	Receipt any    `json:"receipt,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

func fail(code, msg string) Result { return Result{Error: &Error{Code: code, Message: msg}} }
func ok(data any) Result           { return Result{OK: true, Data: data} }

type Ctx struct {
	Store *store.Store
	User  *mockdata.User
	// Question — исходная реплика пользователя в этом ходе. Модель часто «обогащает» запрос к базе
	// общими словами, и верная страница не проходит порог покрытия; поиск страхуется исходной фразой.
	Question string
}

type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Roles       []string // кому инструмент вообще передаётся модели
	Confirm     bool
	Run         func(Ctx, map[string]any) Result
}

func str(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func boolean(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func schema(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func prop(typ, descr string) map[string]any { return map[string]any{"type": typ, "description": descr} }

var allRoles = []string{store.RoleIntern, store.RoleBuddy, store.RoleExpert, store.RoleLead}

// All — полный список инструментов.
func All() []Tool {
	return []Tool{
		{"get_practice", "Практика перед шагом своего плана: термины, этап, одобрение задания, результат. Покажи стажёру практику в «Мой день» или «Мой план».", schema(map[string]any{"practice_id": prop("string", "необязательно: id практики")}), []string{store.RoleIntern}, false, getPractice},
		{"advance_practice", "Выбор стажёра: начать, следующий этап или пропустить практику. Реальная задача не отмечается выполненной.", schema(map[string]any{"practice_id": prop("string", "необязательно: id практики"), "action": map[string]any{"type": "string", "enum": []string{"start", "next", "skip_stage", "skip_all"}}, "expected_stage": prop("string", "текущий этап из get_practice, нужен для next и skip_stage")}, "action"), []string{store.RoleIntern}, false, advancePracticeTool},
		{"get_today", "Шаги на сегодня (до 3, просроченные первыми) и события дня стажёра.",
			schema(map[string]any{}), []string{store.RoleIntern}, false, getToday},
		{"get_plan", "План адаптации: этапы, шаги со статусами, прогресс, следующий шаг. intern_id — только для бадди и руководителя.",
			schema(map[string]any{"intern_id": prop("string", "id стажёра")}), allRoles, false, getPlan},
		{"mark_step_done", "Отметить шаг плана выполненным (после подтверждения).",
			schema(map[string]any{"step_id": prop("string", "id шага, например s-04"), "intern_id": prop("string", "для бадди")}, "step_id"),
			[]string{store.RoleIntern, store.RoleBuddy}, true, markStepDone},
		{"search_kb", "Поиск в базе знаний своего отдела. Возвращает страницы с источником, автором и датой.",
			schema(map[string]any{"query": prop("string", "вопрос или ключевые слова")}, "query"), allRoles, false, searchKB},
		{"team_directory", "Кто есть кто в отделе: роль и зона ответственности.",
			schema(map[string]any{}), allRoles, false, teamDirectory},
		{"find_expert", "Подобрать человека для вопроса, которого нет в базе: по авторству страниц и зоне ответственности. Если уверенности нет — бадди.",
			schema(map[string]any{"question": prop("string", "текст вопроса")}, "question"), []string{store.RoleIntern}, false, findExpert},
		{"send_question", "Отправить сформулированный вопрос человеку (только после подтверждения стажёра).",
			schema(map[string]any{"title": prop("string", "суть вопроса одной строкой, без приветствия — станет заголовком страницы базы"),
				"text": prop("string", "полный вопрос: суть, контекст, что уже пробовал"), "addressee_id": prop("string", "id адресата"),
				"reason": prop("string", "почему выбран адресат"), "urgent": prop("boolean", "блокирует работу")}, "text", "addressee_id"),
			[]string{store.RoleIntern}, true, sendQuestion},
		{"explain_term", "Только «что значит слово»: значение термина или жаргона команды — из глоссария отдела с пометкой «у нас так говорят» и источником; если в глоссарии нет — found=false, можно дать общий смысл с пометкой «общее определение». Вызывай для каждого термина отдельно.",
			schema(map[string]any{"term": prop("string", "один термин, например «сплит»")}, "term"), allRoles, false, explainTerm},
		{"suggest_term", "Предложить новый термин в глоссарий отдела (после подтверждения стажёра): черновик страницы уйдёт бадди, в базе термин появится после его «Ок».",
			schema(map[string]any{"term": prop("string", "термин"), "definition": prop("string", "черновик определения простыми словами")}, "term", "definition"),
			[]string{store.RoleIntern}, true, suggestTerm},
		{"get_role_knowledge", "Набор знаний своей роли: глоссарий, код-стайл, ревью, карта данных или сервисов, учебная практика — со страницами базы.",
			schema(map[string]any{}), allRoles, false, roleKnowledge},
		{"get_absence_rules", "Порядок оформления отсутствия и путь «сделай сам».",
			schema(map[string]any{"type": map[string]any{"type": "string", "enum": []string{"day_off", "vacation", "sick_leave"}}}, "type"),
			allRoles, false, getAbsenceRules},
		{"precheck_absence", "Проверить дату отсутствия: срок подачи, встречи, дедлайны шагов.",
			schema(map[string]any{"type": map[string]any{"type": "string", "enum": []string{"day_off", "vacation", "sick_leave"}},
				"date": prop("string", "ГГГГ-ММ-ДД")}, "type", "date"), []string{store.RoleIntern}, false, precheckAbsence},
		{"create_absence_request", "Создать заявку в сервисе отсутствий (после подтверждения).",
			schema(map[string]any{"type": map[string]any{"type": "string", "enum": []string{"day_off", "vacation", "sick_leave"}},
				"date": prop("string", "ГГГГ-ММ-ДД"), "comment": prop("string", "необязательно")}, "type", "date"),
			[]string{store.RoleIntern}, true, createAbsenceRequest},
		{"list_my_requests", "Мои заявки и их статусы.", schema(map[string]any{}), []string{store.RoleIntern}, false, listMyRequests},
		{"get_required_accesses", "Какие доступы нужны на текущей неделе и их статусы.",
			schema(map[string]any{}), []string{store.RoleIntern}, false, getRequiredAccesses},
		{"get_access_rules", "Как попасть в систему: каталог разрешённых доступов, статус, с какой недели положен, свой шаг плана и кто согласует. Включает будущие недели. Без system — весь разрешённый каталог. Не создаёт заявку.",
			schema(map[string]any{"system": prop("string", "необязательно: id системы из каталога")}), []string{store.RoleIntern}, false, getAccessRules},
		{"request_access", "Создать заявку на доступ к системе (после подтверждения).",
			schema(map[string]any{"system": prop("string", "id системы, например sys-redash"), "reason": prop("string", "зачем")}, "system", "reason"),
			[]string{store.RoleIntern}, true, requestAccess},
	}
}

func ByName(name string) (Tool, bool) {
	for _, t := range All() {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// toolFlags — какой feature flag выключает инструмент (FEATURE_FLAGS, FLAGS_FILE).
var toolFlags = map[string]string{
	"get_practice": flags.Practice, "advance_practice": flags.Practice,
	"explain_term": flags.Glossary, "suggest_term": flags.Glossary, "get_role_knowledge": flags.Glossary,
}

// Allowed — роль пользователя и флаг функции (выключенный инструмент не видит ни модель, ни MCP).
func Allowed(t Tool, u *mockdata.User) bool {
	if f := toolFlags[t.Name]; f != "" && !flags.On(f, u.Roles) {
		return false
	}
	for _, r := range t.Roles {
		if store.HasRole(u, r) {
			return true
		}
	}
	return false
}

// For — инструменты, которые можно передать модели для этого пользователя.
func For(u *mockdata.User) []Tool {
	var out []Tool
	for _, t := range All() {
		if Allowed(t, u) {
			out = append(out, t)
		}
	}
	return out
}

// Run выполняет инструмент с проверкой прав (BR-40).
func Run(c Ctx, name string, args map[string]any) Result {
	t, found := ByName(name)
	if !found || !Allowed(t, c.User) {
		return fail("forbidden", "Это действие тебе недоступно.")
	}
	if args == nil {
		args = map[string]any{}
	}
	if t.Confirm {
		return atomicResult(c, func(c Ctx) Result { return t.Run(c, args) })
	}
	return t.Run(c, args)
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func workdaysBetween(from, to time.Time) int {
	n := 0
	for d := from.AddDate(0, 0, 1); !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n++
		}
	}
	return n
}

// Check — проверки прав и данных для инструментов с подтверждением, БЕЗ побочных эффектов.
// Агент вызывает их до создания черновика: недоступное не предлагается даже черновиком (BR-40, BR-44).
func Check(c Ctx, name string, args map[string]any) *Error {
	t, found := ByName(name)
	if !found || !Allowed(t, c.User) {
		return &Error{Code: "forbidden", Message: "Это действие тебе недоступно."}
	}
	switch name {
	case "request_access":
		return checkAccess(c, str(args, "system"))
	case "send_question":
		if _, ok := c.Store.User(str(args, "addressee_id")); !ok {
			return &Error{Code: "not_found", Message: "Адресат не найден в команде."}
		}
		if e := seniorBlock(c, str(args, "addressee_id")); e != nil { // до черновика: имя директора стажёр не увидит
			return e
		}
	case "create_absence_request":
		if _, ok := requestType(c, str(args, "type")); !ok {
			return &Error{Code: "validation", Message: "Неизвестный тип отсутствия."}
		}
		if day, ok := parseDate(str(args, "date")); !ok || !day.After(c.Store.Today) {
			return &Error{Code: "validation", Message: "Укажи будущую дату"}
		}
	case "suggest_term":
		if _, found := lookupTerm(c, str(args, "term")); found {
			return &Error{Code: "validation", Message: "Этот термин уже есть в глоссарии — объясни его через explain_term."}
		}
	case "mark_step_done":
		if internID := str(args, "intern_id"); internID != "" && internID != c.User.ID {
			if intern, ok := c.Store.User(internID); !ok || intern.BuddyID != c.User.ID {
				return &Error{Code: "forbidden", Message: "Отметить шаг может стажёр или его бадди."}
			}
		}
	}
	return nil
}
