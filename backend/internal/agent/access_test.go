package agent

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// SC-33, BR-40, BR-45: модель получает каталог, а не закрытые страницы и их метаданные.
func TestAccessGuidanceDoesNotSearchPrivateKB(t *testing.T) {
	for _, question := range []string{
		"Хочу сам посмотреть данные в витринах DWH, как туда попасть?",
		"Как оформить доступ к DWH?",
		"Мне нужен доступ к DWH. Как его получить?",
		"Не оформляй заявку, только объясни как попасть в витрины DWH",
	} {
		a := newAgent(t, "stub_ok", time.Second)
		r := ask(t, a, "intern.sokolov", question)
		if len(r.Drafts) != 0 || len(a.Store.Requests("")) != 0 || !slices.Equal(r.ToolLog, []string{"get_access_rules"}) {
			t.Fatal(question, r)
		}
		// Источник — только открытая тестовая инструкция по доступам (M-16), не закрытые страницы.
		for _, src := range r.Sources {
			if src.PageID != "kb-11" {
				t.Fatal("лишний источник", src)
			}
		}
		for _, secret := range []string{"kb-R1", "space-dwh-pii", "sys-dwh-pii", "Витрина клиентов с контактными данными"} {
			if strings.Contains(r.Text, secret) {
				t.Fatal("закрытые метаданные в ответе", secret)
			}
		}
	}
}

// SC-33: уже выданный доступ / ожидающая заявка не создают новый черновик даже по прямой просьбе.
func TestAccessGuidanceExistingStatus(t *testing.T) {
	for status, message := range map[string]string{"granted": "уже выдан", "pending": "уже на согласовании"} {
		a := newAgent(t, "stub_ok", time.Second)
		a.Store.Data.Access.CurrentState["u-sokolov"] = []mockdata.AccessState{{System: "sys-dwh-marts-ro", Status: status}}
		r := ask(t, a, "intern.sokolov", "Оформи доступ к витринам DWH")
		if len(r.Drafts) != 0 || !strings.Contains(r.Text, message) || strings.Contains(r.Text, "сделаю за тебя") {
			t.Fatal(status, r)
		}
	}
}

// SC-16 / SC-33: закрытую систему не объясняем по её странице и не предлагаем заявку даже черновиком.
func TestAccessGuidanceForbiddenSystem(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	r := ask(t, a, "intern.sokolov", "Нужен доступ к витринам с ПДн")
	if len(r.Drafts) != 0 || len(r.Sources) != 0 || len(a.Store.Requests("")) != 0 ||
		!strings.Contains(r.Text, "бадди") || strings.Contains(r.Text, "сделаю за тебя") || strings.Contains(r.Text, "sys-dwh-pii") {
		t.Fatal(r)
	}
}

// SC-33 → SC-14: новая просьба после объяснения доступа не должна оформлять старую систему.
func TestAccessGuidanceDoesNotCaptureAnotherRequest(t *testing.T) {
	a := newAgent(t, "stub_ok", time.Second)
	ask(t, a, "intern.sokolov", "Как попасть в витрины DWH?")
	r := ask(t, a, "intern.sokolov", "Оформи отгул в пятницу")
	if len(r.Drafts) != 1 || r.Drafts[0].Tool != "create_absence_request" {
		t.Fatal(r)
	}
	// Вопрос о другой системе нельзя привязать к предыдущему доступу без выбора пользователя.
	a = newAgent(t, "stub_ok", time.Second)
	ask(t, a, "intern.sokolov", "Как попасть в витрины DWH?")
	r = ask(t, a, "intern.sokolov", "Как получить доступ к другой системе?")
	if len(r.Drafts) != 0 || slices.Contains(r.ToolLog, "request_access") {
		t.Fatal(r)
	}
}
