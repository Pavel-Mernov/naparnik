package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

// Карточка стажёра: фото (место), ФИО, статус, отдел, телефон, почта, где в стажировке.
func TestInternProfile(t *testing.T) {
	s := newStore(t)
	lead := ctxFor(t, s, "lead.kovalev")
	v, e := GetProfile(lead, "u-sokolov")
	if e != nil || v.Name != "Даниил Соколов" || v.Status != "Стажёр" || v.Department == "" || v.Email != "intern.sokolov@naparnik.example" ||
		v.Phone == "" || v.Photo != nil || v.Initials != "ДС" || v.Buddy != "Екатерина Белова" || v.Week < 1 || v.CanEdit {
		t.Fatalf("%+v %v", v, e)
	}
	if p, _ := GetProfile(lead, "u-kovalev"); p.Status != "Руководитель" || !p.CanEdit || p.Buddy != "" {
		t.Fatalf("%+v", p)
	}
	if _, e := GetProfile(lead, "u-nobody"); e == nil || e.Code != "not_found" {
		t.Fatal(e)
	}
	interns := ListProfiles(lead, "intern")
	if len(interns) != 2 || interns[0].Status != "Стажёр" {
		t.Fatal(interns)
	}
	if len(ListProfiles(lead, "")) != len(s.Data.Team.Users) {
		t.Fatal("весь отдел")
	}
}

func TestProfilePhone(t *testing.T) {
	for in, want := range map[string]string{"+7 (912) 345-67-89": "+7 912 345-67-89", "89123456789": "+7 912 345-67-89", "9123456789": "+7 912 345-67-89"} {
		if got, ok := NormalizePhone(in); !ok || got != want {
			t.Error(in, got)
		}
	}
	for _, bad := range []string{"", "123", "+1 202 555 0100 1", "0123456789", "телефон"} {
		if _, ok := NormalizePhone(bad); ok {
			t.Error("принят неверный номер", bad)
		}
	}
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	v, e := UpdatePhone(c, "8 (912) 345 67 89")
	if e != nil || v.Phone != "+7 912 345-67-89" {
		t.Fatal(v, e)
	}
	if other, _ := GetProfile(ctxFor(t, s, "buddy.belova"), "u-sokolov"); other.Phone != "+7 912 345-67-89" {
		t.Fatal("бадди видит новый номер", other.Phone)
	}
	if _, e := UpdatePhone(c, "abc"); e == nil || e.Code != "validation" {
		t.Fatal(e)
	}
	for _, ev := range s.Events() {
		if ev.Type == "profile_updated" && strings.Contains(strings.Join([]string{ev.Meta["field"], ev.Object}, " "), "912") {
			t.Fatal("номер попал в журнал событий", ev)
		}
	}
}

func TestProfilePhoto(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	if _, e := PhotoOf(c, "u-sokolov"); e == nil || e.Code != "not_found" {
		t.Fatal("фото ещё нет")
	}
	v, e := SetPhoto(c, pngBytes)
	if e != nil || v.Photo == nil || !strings.HasPrefix(v.Photo.URL, "/api/profiles/u-sokolov/photo") {
		t.Fatal(v, e)
	}
	p, e := PhotoOf(ctxFor(t, s, "lead.kovalev"), "u-sokolov")
	if e != nil || p.ContentType != "image/png" || len(p.Data) != len(pngBytes) {
		t.Fatal(p, e)
	}
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	if v, e := SetPhoto(c, webp); e != nil || v.Photo == nil {
		t.Fatal("webp", e)
	}
	for name, bad := range map[string][]byte{"пусто": nil, "не картинка": []byte("<script>alert(1)</script>"), "большое": append(append([]byte{}, pngBytes...), make([]byte, MaxPhotoBytes)...)} {
		if _, e := SetPhoto(c, bad); e == nil || e.Code != "validation" {
			t.Error(name, e)
		}
	}
	if v, e := DeletePhoto(c); e != nil || v.Photo != nil {
		t.Fatal(v, e)
	}
	if _, e := PhotoOf(c, "u-sokolov"); e == nil {
		t.Fatal("фото удалено")
	}
}

// BR-72: контакты не уходят в модель — инструменты агента их не возвращают.
func TestContactsNotSentToModel(t *testing.T) {
	s := newStore(t)
	c := ctxFor(t, s, "intern.sokolov")
	for _, tool := range []string{"team_directory", "get_plan", "get_today", "find_expert"} {
		raw, _ := json.Marshal(Run(c, tool, map[string]any{"question": "конверсия"}))
		if strings.Contains(string(raw), "@naparnik.example") || strings.Contains(string(raw), "+7 900") {
			t.Fatal("контакты в ответе инструмента", tool)
		}
	}
}
