package tools

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// Профили сотрудников (карточка стажёра): фото, ФИО, статус, отдел, рабочие телефон и почта.
//
// Видимость (02-ROLES, BR-40): карточку видят сотрудники своего отдела — это рабочие контакты, чтобы стажёру
// и стажёра могли найти. Чужой отдел — не видит. Правит сотрудник только свой телефон и фото; ФИО, должность,
// отдел и почта — из оргструктуры. Контакты не передаются модели (BR-72): инструменты агента их не возвращают.

const MaxPhotoBytes = 512 << 10 // 512 КБ: фото хранится в состоянии; фронт уменьшает картинку перед загрузкой

var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

type PhotoView struct {
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

type ProfileView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Initials   string     `json:"initials"` // для заглушки вместо фото
	Photo      *PhotoView `json:"photo"`    // null — фото нет, показывается заглушка с инициалами
	Status     string     `json:"status"`   // «Стажёр», «Бадди», «Эксперт», «Руководитель»
	Position   string     `json:"position"`
	Department string     `json:"department"`
	Phone      string     `json:"phone"`
	Email      string     `json:"email"`
	// Для стажёра — где он в стажировке.
	StartDate string `json:"start_date,omitempty"`
	Week      int    `json:"week,omitempty"`
	Buddy     string `json:"buddy,omitempty"`
	Lead      string `json:"lead,omitempty"`
	CanEdit   bool   `json:"can_edit"` // своя карточка: можно поправить телефон и фото
}

// profileStatus — статус по главной роли: стажёр важнее остальных ролей.
func profileStatus(u *mockdata.User) string {
	for _, r := range []struct{ role, title string }{{store.RoleIntern, "Стажёр"}, {store.RoleLead, "Руководитель"}, {store.RoleBuddy, "Бадди"}, {store.RoleExpert, "Эксперт"}} {
		if store.HasRole(u, r.role) {
			return r.title
		}
	}
	return "Сотрудник"
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		if r := []rune(w); len(r) > 0 && len(out) < 2 {
			out = append(out, []rune(strings.ToUpper(string(r[0])))...)
		}
	}
	return string(out)
}

// sameDepartment — в прототипе один отдел; у пользователя без отдела карточек нет.
func sameDepartment(c Ctx, other *mockdata.User) bool {
	return c.Store.Data.Team.Department.ID != "" && other != nil
}

func profileView(c Ctx, u *mockdata.User) ProfileView {
	v := ProfileView{ID: u.ID, Name: u.Name, Initials: initials(u.Name), Status: profileStatus(u), Position: u.Position,
		Department: c.Store.Data.Team.Department.Name, Phone: u.Phone, Email: u.Email, CanEdit: u.ID == c.User.ID}
	if edit, ok := c.Store.Profile(u.ID); ok {
		if edit.Phone != "" {
			v.Phone = edit.Phone
		}
		if edit.Photo != nil {
			v.Photo = &PhotoView{URL: "/api/profiles/" + u.ID + "/photo?v=" + edit.Photo.UpdatedAt, UpdatedAt: edit.Photo.UpdatedAt}
		}
	}
	if store.HasRole(u, store.RoleIntern) {
		v.StartDate, v.Week, v.Buddy, v.Lead = u.StartDate, c.Store.Week(u), c.Store.Name(u.BuddyID), c.Store.Name(u.LeadID)
	}
	return v
}

// GetProfile — карточка сотрудника своего отдела.
func GetProfile(c Ctx, id string) (ProfileView, *Error) {
	u, ok := c.Store.User(id)
	if !ok || !sameDepartment(c, u) {
		return ProfileView{}, &Error{Code: "not_found", Message: "Профиль не найден."}
	}
	return profileView(c, u), nil
}

// ListProfiles — карточки отдела; role=intern — только стажёры.
func ListProfiles(c Ctx, role string) []ProfileView {
	out := []ProfileView{}
	for i := range c.Store.Data.Team.Users {
		u := &c.Store.Data.Team.Users[i]
		if role != "" && !store.HasRole(u, role) {
			continue
		}
		out = append(out, profileView(c, u))
	}
	return out
}

var phoneDigits = regexp.MustCompile(`\D`)

// NormalizePhone — российский номер в вид «+7 900 123-45-67»: принимаются +7…, 8…, 7… и 10 цифр без кода.
func NormalizePhone(raw string) (string, bool) {
	d := phoneDigits.ReplaceAllString(raw, "")
	if len(d) == 11 && (d[0] == '7' || d[0] == '8') {
		d = d[1:]
	}
	if len(d) != 10 || d[0] == '0' {
		return "", false
	}
	return fmt.Sprintf("+7 %s %s-%s-%s", d[0:3], d[3:6], d[6:8], d[8:10]), true
}

// UpdatePhone — сотрудник правит свой рабочий телефон.
func UpdatePhone(c Ctx, phone string) (ProfileView, *Error) {
	return atomic(c, func(c Ctx) (ProfileView, *Error) {
		norm, ok := NormalizePhone(phone)
		if !ok {
			return ProfileView{}, &Error{Code: "validation", Message: "Телефон — российский номер, например +7 900 123-45-67."}
		}
		c.Store.UpdateProfile(c.User.ID, func(p *store.ProfileEdit) { p.Phone, p.UpdatedAt = norm, store.Now().Format(time.RFC3339) })
		c.Store.Emit(store.EvProfileUpdated, c.User.ID, c.User.ID, map[string]string{"field": "phone"})
		return profileView(c, c.User), nil
	})
}

// SetPhoto — своё фото: JPEG, PNG или WebP до 512 КБ; тип проверяется по содержимому, а не по заголовку.
func SetPhoto(c Ctx, data []byte) (ProfileView, *Error) {
	if len(data) == 0 {
		return ProfileView{}, &Error{Code: "validation", Message: "Пустой файл."}
	}
	if len(data) > MaxPhotoBytes {
		return ProfileView{}, &Error{Code: "validation", Message: "Фото больше 512 КБ — уменьши его перед загрузкой."}
	}
	ct := http.DetectContentType(data)
	if ct == "application/octet-stream" && len(data) > 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		ct = "image/webp"
	}
	if !photoTypes[ct] {
		return ProfileView{}, &Error{Code: "validation", Message: "Фото — JPEG, PNG или WebP."}
	}
	return atomic(c, func(c Ctx) (ProfileView, *Error) {
		now := store.Now().Format(time.RFC3339)
		c.Store.UpdateProfile(c.User.ID, func(p *store.ProfileEdit) {
			p.Photo, p.UpdatedAt = &store.Photo{ContentType: ct, Data: data, UpdatedAt: now}, now
		})
		c.Store.Emit(store.EvProfileUpdated, c.User.ID, c.User.ID, map[string]string{"field": "photo"})
		return profileView(c, c.User), nil
	})
}

// DeletePhoto — убрать своё фото: вместо него заглушка с инициалами.
func DeletePhoto(c Ctx) (ProfileView, *Error) {
	return atomic(c, func(c Ctx) (ProfileView, *Error) {
		c.Store.UpdateProfile(c.User.ID, func(p *store.ProfileEdit) { p.Photo, p.UpdatedAt = nil, store.Now().Format(time.RFC3339) })
		c.Store.Emit(store.EvProfileUpdated, c.User.ID, c.User.ID, map[string]string{"field": "photo_deleted"})
		return profileView(c, c.User), nil
	})
}

// PhotoOf — само изображение для <img>; видимость как у карточки.
func PhotoOf(c Ctx, id string) (*store.Photo, *Error) {
	if _, e := GetProfile(c, id); e != nil {
		return nil, e
	}
	if p, ok := c.Store.Profile(id); ok && p.Photo != nil {
		return p.Photo, nil
	}
	return nil, &Error{Code: "not_found", Message: "Фото нет."}
}
