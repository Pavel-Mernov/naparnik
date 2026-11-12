package flags

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlags(t *testing.T) {
	var none *Provider
	if !none.Enabled(Practice, nil) || !On(Practice, nil) {
		t.Fatal("без настроек — всё включено")
	}
	t.Setenv("FEATURE_FLAGS", "practice=off, glossary , telegram=false")
	dir := t.TempDir()
	file := filepath.Join(dir, "flags.json")
	t.Setenv("FLAGS_FILE", file)
	p := FromEnv()
	if p.Enabled(Practice, nil) || !p.Enabled(Glossary, nil) || p.Enabled(Telegram, nil) || !p.Enabled(Meetings, nil) {
		t.Fatal(p.Snapshot())
	}
	// Файл — во время работы, важнее переменной; роли — постепенное включение.
	_ = os.WriteFile(file, []byte(`{"practice": {"on": true, "roles": ["intern"]}, "meetings": {"on": false}}`), 0o644)
	future := time.Now().Add(time.Second)
	_ = os.Chtimes(file, future, future)
	p.checked = time.Time{}
	if !p.Enabled(Practice, []string{"intern"}) || p.Enabled(Practice, []string{"lead"}) || p.Enabled(Meetings, nil) {
		t.Fatal("файл не перечитан")
	}
	if s := p.Snapshot(); !s[Practice] || s[Meetings] {
		t.Fatal(s)
	}
	_ = os.WriteFile(file, []byte(`не json`), 0o644)
	later := future.Add(time.Second)
	_ = os.Chtimes(file, later, later)
	p.checked = time.Time{}
	if p.Enabled(Meetings, nil) {
		t.Fatal("битый файл — остаются прежние значения")
	}
}
