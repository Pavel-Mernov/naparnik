package app

import "testing"

func TestNew(t *testing.T) {
	t.Setenv("MOCK_DATA_DIR", "../../../docs/04-mock-data")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_MODE", "")
	t.Setenv("DEMO_TODAY", "")
	a, err := New()
	if err != nil || a.LLM.Provider != "stub" || a.Store.Today.Format("2006-01-02") != "2026-11-11" {
		t.Fatalf("%+v %v", a, err)
	}
	t.Setenv("DEMO_TODAY", "2026-11-13")
	if a, _ := New(); a.Store.Today.Day() != 13 {
		t.Fatal("DEMO_TODAY")
	}
	t.Setenv("DEMO_TODAY", "кривая")
	if _, err := New(); err != nil {
		t.Fatal("кривая дата — берём сегодня")
	}
	t.Setenv("LLM_PROVIDER", "gpt")
	if _, err := New(); err == nil {
		t.Fatal("неизвестный провайдер")
	}
	t.Setenv("MOCK_DATA_DIR", "/nonexistent")
	if _, err := New(); err == nil {
		t.Fatal("нет мок-данных")
	}
	t.Setenv("MOCK_DATA_DIR", "")
	if _, err := New(); err == nil {
		t.Fatal("путь по умолчанию относительно backend/, из пакета он не найдётся")
	}
}
