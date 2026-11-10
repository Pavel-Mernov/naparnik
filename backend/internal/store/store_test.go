package store

import (
	"os"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

func load(t *testing.T) *Store {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	if d.DemoToday != "2026-11-11" || len(d.Team.Users) != 7 || len(d.Template.Steps) != 20 {
		t.Fatalf("мок-данные: %s, %d людей, %d шагов", d.DemoToday, len(d.Team.Users), len(d.Template.Steps))
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	return New(d, today)
}

func TestLoadErrors(t *testing.T) {
	if _, err := mockdata.Load("/nonexistent"); err == nil {
		t.Fatal("нет каталога")
	}
	dir := t.TempDir()
	_ = writeFile(dir+"/team.json", "{broken")
	if _, err := mockdata.Load(dir); err == nil {
		t.Fatal("битый JSON")
	}
}

func TestUsersAndRelations(t *testing.T) {
	s := load(t)
	u, _ := s.UserByLogin("intern.sokolov")
	b, _ := s.UserByLogin("buddy.belova")
	e, _ := s.UserByLogin("expert.orlov")
	if !HasRole(b, RoleBuddy) || HasRole(u, RoleBuddy) {
		t.Fatal("роли")
	}
	if !s.CanSeeIntern(b, u.ID) || s.CanSeeIntern(e, u.ID) || !s.CanSeeIntern(u, u.ID) || s.CanSeeIntern(b, "u-nobody") {
		t.Fatal("видимость стажёра")
	}
	if s.Name("u-nobody") != "u-nobody" || s.Name(u.ID) != "Даниил Соколов" {
		t.Fatal("имя")
	}
	if _, ok := s.UserByLogin("nobody"); ok {
		t.Fatal("нет учётки")
	}
}

func TestWeek(t *testing.T) {
	s := load(t)
	u, _ := s.UserByLogin("intern.sokolov")
	if s.Week(u) != 1 {
		t.Fatal("неделя 1")
	}
	s.Today = s.Today.AddDate(0, 0, 14)
	if s.Week(u) != 3 {
		t.Fatal("неделя 3")
	}
	p, _ := s.UserByLogin("intern.egorova")
	if s.Week(p) != 1 {
		t.Fatal("до начала стажировки — неделя 1")
	}
}

func TestStateChanges(t *testing.T) {
	s := load(t)
	if !s.SetStepStatus("u-sokolov", "s-01", "Done") || s.SetStepStatus("u-sokolov", "s-99", "Done") {
		t.Fatal("статус шага")
	}
	for _, st := range s.PlanSteps("u-sokolov") {
		if st.StepID == "s-01" && st.Status != "Done" {
			t.Fatal("статус не сохранён")
		}
	}
	if _, ok := s.TemplateStep("s-99"); ok {
		t.Fatal("нет шага в шаблоне")
	}
	s.AddRequest(&Request{ID: "ABS-1", EmployeeID: "u-1"})
	s.AddRequest(&Request{ID: "ABS-2", EmployeeID: "u-2"})
	if len(s.Requests("u-1")) != 1 || len(s.Requests("")) != 2 {
		t.Fatal("заявки")
	}
	if s.NextID("ABS") != "ABS-1042" || s.NextID("ABS") != "ABS-1043" {
		t.Fatal("нумерация квитанций как в мок-данных")
	}
	if !s.ReadableSpaces()["space-analytics"] || s.ReadableSpaces()["space-dwh-pii"] {
		t.Fatal("пространства отдела")
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }
