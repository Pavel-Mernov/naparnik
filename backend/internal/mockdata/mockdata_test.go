package mockdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTeamData(t *testing.T) {
	d, err := Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	if d.Team.Department.ID != "dept-analytics" || len(d.KB.Pages) < 20 || len(d.Tracker.Plans) != 2 ||
		len(d.Calendar.Events) == 0 || len(d.Absence.RequestTypes) != 3 || len(d.Access.Systems) != 5 {
		t.Fatalf("неполные данные: %+v", d.Team.Department)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("пустой каталог")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "team.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("битый JSON")
	}
}
