package tools

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// TeamPage — страница базы, которую написал человек: ссылка на его тему в карточке.
type TeamPage struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

// TeamCard — человек на карте команды (SCR-I5, US-06, SC-02).
type TeamCard struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Initials       string     `json:"initials"`
	Photo          *PhotoView `json:"photo"`
	Status         string     `json:"status"`
	Position       string     `json:"position"`
	Responsibility string     `json:"responsibility"`
	WorkingHours   string     `json:"working_hours"`
	MyBuddy        bool       `json:"my_buddy,omitempty"`
	// ViaBuddy — топ-руководитель: стажёру напрямую не предлагается, вопросы — через бадди (BR-10a). Черновика нет.
	ViaBuddy bool       `json:"via_buddy,omitempty"`
	Pages    []TeamPage `json:"pages"`
	// Message — черновик первого сообщения. Его копирует и отправляет сам человек: Напарник ничего
	// не пишет коллегам от его имени (BR-62).
	Message string `json:"message"`
}

// maxTeamPages — сколько своих страниц показать в карточке.
const maxTeamPages = 3

// TeamMap — карта команды для стажёра: коллеги отдела с зоной ответственности, их страницами в базе,
// которые стажёр может читать (BR-40), и черновиком сообщения. Бадди — первым, остальные по алфавиту.
func TeamMap(c Ctx) []TeamCard {
	pages := readablePages(c)
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].UpdatedAt > pages[j].UpdatedAt })
	out := []TeamCard{}
	for i := range c.Store.Data.Team.Users {
		u := &c.Store.Data.Team.Users[i]
		if u.ID == c.User.ID || store.HasRole(u, store.RoleIntern) {
			continue
		}
		p := profileView(c, u)
		card := TeamCard{ID: u.ID, Name: u.Name, Initials: p.Initials, Photo: p.Photo, Status: p.Status, Position: u.Position,
			Responsibility: u.Responsibility, WorkingHours: u.WorkingHours, MyBuddy: u.ID == c.User.BuddyID, Pages: []TeamPage{}}
		for _, pg := range pages {
			if pg.AuthorID == u.ID && len(card.Pages) < maxTeamPages {
				card.Pages = append(card.Pages, TeamPage{pg.ID, pg.Title, pg.UpdatedAt})
			}
		}
		if store.HasRole(c.User, store.RoleIntern) && isSenior(c, u.ID) {
			card.ViaBuddy = true
		} else {
			card.Message = firstMessage(c, u.Name, u.Responsibility, card.MyBuddy)
		}
		out = append(out, card)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MyBuddy != out[j].MyBuddy {
			return out[i].MyBuddy
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// firstMessage — короткое знакомство: кто я, почему пишу именно тебе, о чём хочу договориться.
func firstMessage(c Ctx, name, responsibility string, buddy bool) string {
	first := strings.Fields(name)[0]
	me := c.User.Name
	if c.User.Position != "" {
		me += ", " + lowerFirst(c.User.Position)
	}
	if buddy {
		return fmt.Sprintf("Привет, %s! Я %s. Ты мой бадди — давай договоримся о регулярной встрече и о том, как мне удобнее задавать вопросы.", first, me)
	}
	if responsibility == "" {
		return fmt.Sprintf("Привет, %s! Я %s, недавно в отделе. Хочу познакомиться — найдёшь 15 минут на этой неделе?", first, me)
	}
	return fmt.Sprintf("Привет, %s! Я %s, недавно в отделе. Мне подсказали, что ты отвечаешь за: %s. Хочу познакомиться и понять, с какими вопросами к тебе можно приходить. Найдёшь 15 минут на этой неделе?",
		first, me, lowerFirst(strings.TrimSuffix(responsibility, ".")))
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 1 && unicode.IsUpper(r[0]) && unicode.IsLower(r[1]) { // «A/B», «DWH» — не трогаем
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}
