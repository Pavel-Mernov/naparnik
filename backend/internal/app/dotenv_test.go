package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	_ = os.WriteFile(p, []byte("# комментарий\nBUDDY_A=1\nBUDDY_B=\"два\"\n\nBUDDY_C=из файла\nбез знака\n"), 0o600)
	t.Setenv("BUDDY_C", "из окружения")
	_ = os.Unsetenv("BUDDY_A")
	_ = os.Unsetenv("BUDDY_B")
	t.Cleanup(func() { _ = os.Unsetenv("BUDDY_A"); _ = os.Unsetenv("BUDDY_B") })
	if got := LoadDotEnv(filepath.Join(dir, "нет"), p); got != p {
		t.Fatalf("файл: %q", got)
	}
	if os.Getenv("BUDDY_A") != "1" || os.Getenv("BUDDY_B") != "два" || os.Getenv("BUDDY_C") != "из окружения" {
		t.Fatal("значения")
	}
	if LoadDotEnv(filepath.Join(dir, "нет")) != "" {
		t.Fatal("нет файлов")
	}
}
