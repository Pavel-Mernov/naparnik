package tools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Жаргон и набор знаний роли (SC-34, US-36, BR-87, BR-88).
//
// Термин команды отвечается из глоссария отдела с пометкой «у нас так говорят» и ссылкой на страницу.
// Общий смысл ИИ может дать и без базы — с пометкой «общее определение». Нового термина в глоссарии нет →
// ИИ предлагает добавить его; страница-черновик уходит бадди, и в глоссарий термин попадает только после его «Ок».
// Набор знаний (глоссарий, код-стайл, карта данных или сервисов, практика) — данные роли, логика одна.

const (
	LabelTeamTerm    = "у нас так говорят"
	LabelGeneralTerm = "общее определение"
	GlossaryFolder   = "f-glossary"
)

// RoleSetFor — набор знаний роли пользователя: из профиля, иначе первый рабочий (не пример).
func RoleSetFor(c Ctx) (mockdata.RoleSet, bool) {
	sets := c.Store.Data.Roles.Sets
	for _, rs := range sets {
		if rs.ID == c.User.RoleSet {
			return rs, true
		}
	}
	for _, rs := range sets {
		if !rs.IsExample {
			return rs, true
		}
	}
	return mockdata.RoleSet{}, false
}

type glossaryEntry struct {
	Term, Definition string
	Page             *mockdata.Page // nil — термин прямо в наборе роли
}

// entryRe — «Термин — определение.» внутри страницы глоссария.
var entryRe = regexp.MustCompile(`([А-ЯЁA-Z][^.—\n]{0,40}?)\s+—\s+([^.\n]+)\.?`)

// glossary — термины команды: страницы глоссария из набора роли и опубликованные страницы папки глоссария
// (так туда попадают термины, добавленные бадди).
func glossary(c Ctx) []glossaryEntry {
	rs, _ := RoleSetFor(c)
	var out []glossaryEntry
	for _, p := range readablePages(c) {
		if (!containsStr(rs.Glossary, p.ID) && p.Folder != GlossaryFolder) || p.Confidential() { // BR-49
			continue
		}
		page := p
		for _, m := range entryRe.FindAllStringSubmatch(p.Body, -1) {
			out = append(out, glossaryEntry{strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), &page})
		}
	}
	for _, t := range rs.GlossaryTerms {
		out = append(out, glossaryEntry{Term: t.Term, Definition: t.Definition})
	}
	return out
}

// sameTerm — тот же термин по основам слов: «гардрейлы» ≈ «Гардрейл», но «конверсия в контакт» ≠ «конверсия»
// (у составного термина свой смысл, общий определением подменять нельзя).
func sameTerm(a, b string) bool {
	sa, sb := Stems(a), Stems(b)
	if len(sa) == 0 || len(sb) == 0 {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	if len(sa) != len(sb) {
		return false
	}
	for st := range sb {
		if !sa[st] {
			return false
		}
	}
	return true
}

// termPages — страницы базы по словам стажёра: вопрос часто шире значения слова («какие гардрейлы у нас»).
func termPages(c Ctx, term string) []Hit {
	q := c.Question
	if q == "" {
		q = term
	}
	return cached(c, "search", q, func() []Hit { return SearchKB(c, q) })
}

func lookupTerm(c Ctx, term string) (glossaryEntry, bool) {
	for _, e := range glossary(c) {
		if sameTerm(term, e.Term) {
			return e, true
		}
	}
	return glossaryEntry{}, false
}

func generalTerm(c Ctx, term string) (string, bool) {
	for _, t := range c.Store.Data.Practice.Templates {
		for _, g := range t.General {
			if sameTerm(term, g.Term) {
				return g.Definition, true
			}
		}
	}
	return "", false
}

func explainTerm(c Ctx, args map[string]any) Result {
	term := strings.Trim(strings.TrimSpace(str(args, "term")), "«»\"'")
	if term == "" {
		return fail("validation", "Укажи термин.")
	}
	res := explainTermOnly(c, term)
	if pages := termPages(c, term); len(pages) > 0 && res.OK {
		m := res.Data.(map[string]any)
		m["kb_pages"] = pages
		m["kb_note"] = "Если вопрос шире значения слова (какие, как, чем отличается) — отвечай по этим страницам базы и назови источник."
	}
	return res
}

func explainTermOnly(c Ctx, term string) Result {
	if e, known := lookupTerm(c, term); known {
		out := map[string]any{"found": true, "term": e.Term, "definition": e.Definition, "label": LabelTeamTerm}
		if e.Page != nil {
			out["source"] = map[string]any{"title": e.Page.Title, "author": c.Store.Name(e.Page.AuthorID), "updated_at": e.Page.UpdatedAt}
		}
		return ok(out)
	}
	buddy := c.User.BuddyID
	out := map[string]any{"found": false, "term": term, "label": LabelGeneralTerm,
		"note": "В глоссарии отдела этого термина нет. Можно дать общий смысл — только с пометкой «общее определение», без выдуманных правил команды. " +
			"Предложи добавить термин в глоссарий: добавление подтвердит " + c.Store.Name(buddy) + " (suggest_term, после согласия стажёра)."}
	if d, known := generalTerm(c, term); known {
		out["definition"] = d
	}
	return ok(out)
}

// suggestTerm — черновик страницы глоссария от имени бадди: в базу попадёт только после его «Ок» (BR-88, BR-30).
func suggestTerm(c Ctx, args map[string]any) Result {
	term := strings.Trim(strings.TrimSpace(str(args, "term")), "«»\"'")
	def := strings.TrimSpace(str(args, "definition"))
	buddy := c.User.BuddyID
	switch {
	case term == "" || def == "":
		return fail("validation", "Нужны термин и черновик определения.")
	case buddy == "":
		return fail("validation", "Некому подтвердить термин: у тебя не указан бадди.")
	}
	if _, found := lookupTerm(c, term); found {
		return fail("validation", "Этот термин уже есть в глоссарии.")
	}
	for _, p := range c.Store.Pages() {
		if p.Folder == GlossaryFolder && p.Status == "Draft" && sameTerm(term, strings.TrimPrefix(p.Title, "Термин: ")) {
			return fail("validation", "Этот термин уже ждёт подтверждения у "+c.Store.Name(p.AuthorID)+".")
		}
	}
	space := "space-analytics"
	for _, p := range c.Store.Pages() {
		if p.Folder == GlossaryFolder || p.ID == "kb-05" {
			space = p.Space
		}
	}
	page := mockdata.Page{ID: c.Store.NextID("KB"), Space: space, Folder: GlossaryFolder, Type: "glossary", Status: "Draft",
		Title: "Термин: " + capitalizeFirst(term), AuthorID: buddy, UpdatedAt: c.Store.Today.Format("2006-01-02"),
		Body: capitalizeFirst(term) + " — " + strings.TrimSuffix(def, ".") + "."}
	c.Store.AddPage(page)
	c.Store.Notify(store.Notification{UserID: buddy, Type: "draft_review", View: "questions", TG: "pending",
		Text: fmt.Sprintf("📖 %s предлагает добавить в глоссарий «%s».\nЧто сделать: поправь определение, если нужно, и нажми «Ок» — термин появится в базе для всех.",
			c.User.Name, term)})
	return ok(map[string]any{"page_id": page.ID, "term": term, "approver": c.Store.Name(buddy),
		"receipt": "Черновик термина отправлен на подтверждение " + c.Store.Name(buddy) + "."})
}

func capitalizeFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

// roleKnowledge — что важно знать в своей роли: глоссарий, код-стайл, ревью, карта данных или сервисов, практика.
func roleKnowledge(c Ctx, _ map[string]any) Result {
	rs, found := RoleSetFor(c)
	if !found {
		return fail("not_found", "Набор знаний для твоей роли пока не заполнен.")
	}
	pages := map[string]mockdata.Page{}
	for _, p := range readablePages(c) {
		pages[p.ID] = p
	}
	ref := func(ids []string) []map[string]string {
		out := []map[string]string{}
		for _, id := range ids {
			if p, ok := pages[id]; ok {
				out = append(out, map[string]string{"page_id": p.ID, "title": p.Title, "author": c.Store.Name(p.AuthorID), "updated_at": p.UpdatedAt})
			} else if !strings.HasPrefix(id, "kb-") {
				out = append(out, map[string]string{"rule": id}) // правило текстом прямо в наборе
			}
		}
		return out
	}
	var data []map[string]any
	for _, d := range rs.DataMap {
		data = append(data, map[string]any{"name": d.Name, "what": d.What, "owner": c.Store.Name(d.OwnerID), "sources": ref(d.Sources)})
	}
	var services []map[string]any
	for _, sv := range rs.ServiceMap {
		services = append(services, map[string]any{"service": sv.Service, "what": sv.What, "depends_on": sv.DependsOn, "owner_team": sv.OwnerTeam})
	}
	return ok(map[string]any{"role": rs.Title, "glossary": ref(rs.Glossary), "code_style": ref(rs.CodeStyle), "review": ref(rs.Review),
		"data_map": data, "service_map": services, "practices": rs.Practices, "practice_example": rs.PracticeExample})
}

// roleSources — страницы набора роли как источники ответа (BR-01).
func roleSources(c Ctx) []string {
	rs, _ := RoleSetFor(c)
	ids := append(append(append([]string{}, rs.Glossary...), rs.CodeStyle...), rs.Review...)
	for _, d := range rs.DataMap {
		ids = append(ids, d.Sources...)
	}
	return ids
}
