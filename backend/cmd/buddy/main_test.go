package main

import (
	"bytes"
	"strings"
	"testing"
)

func setup(t *testing.T) {
	t.Setenv("MOCK_DATA_DIR", "../../../docs/04-mock-data")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_MODE", "")
	t.Setenv("MOCK_FAIL", "")
}

func TestSingleQuestionWithConfirm(t *testing.T) {
	setup(t)
	var out bytes.Buffer
	if err := run([]string{"-user", "intern.sokolov", "Мне нужен отгул в пятницу"}, strings.NewReader("y\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ABS-1042") {
		t.Fatal(out.String())
	}
}

func TestDialogSourcesAndFailedConfirm(t *testing.T) {
	setup(t)
	t.Setenv("MOCK_FAIL", "access")
	var out bytes.Buffer
	in := "Что такое витрина и где лежит витрина объявлений?\nНужен доступ к Redash\ny\n\n"
	if err := run([]string{"-user", "intern.sokolov"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "📎") || !strings.Contains(out.String(), "❌") {
		t.Fatal(out.String())
	}
}

func TestErrors(t *testing.T) {
	setup(t)
	var out bytes.Buffer
	if err := run([]string{"-user", "nobody", "x"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("неизвестная учётка")
	}
	if err := run([]string{"-unknown-flag"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("неизвестный флаг")
	}
	t.Setenv("MOCK_DATA_DIR", "/nonexistent")
	if err := run(nil, strings.NewReader(""), &out); err == nil {
		t.Fatal("нет мок-данных")
	}
	t.Setenv("MOCK_DATA_DIR", "../../../docs/04-mock-data")
	if err := run([]string{"-user", "nobody"}, strings.NewReader("вопрос\n"), &out); err == nil {
		t.Fatal("ошибка в диалоге")
	}
}
