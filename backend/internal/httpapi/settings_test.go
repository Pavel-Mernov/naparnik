package httpapi

import "testing"

// SCR-I8: стажёр меняет настройки; бадди сразу перестаёт видеть текст вопросов; проверка значений и прав.
func TestSettings(t *testing.T) {
	_, h := newServer(t)
	code, r := call(t, h, "GET", "/api/settings", "intern.sokolov", "")
	set := r["settings"].(map[string]any)
	if code != 200 || set["proactivity"] != "daily_plan" || set["question_text_visible_to_buddy"] != true || set["working_hours"] != "10:00-17:00" {
		t.Fatalf("настройки по умолчанию: %d %v", code, r)
	}
	// Даниил задаёт вопрос эксперту — бадди следит за маршрутом и видит текст.
	_, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Почему конверсия в дашборде отличается от выгрузки из DWH?"}`)
	id := reply["drafts"].([]any)[0].(map[string]any)["id"].(string)
	_, res := call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", "")
	qid := res["data"].(map[string]any)["id"].(string)
	visible := func() bool {
		_, q := call(t, h, "GET", "/api/questions", "buddy.belova", "")
		for _, x := range q["questions"].([]any) {
			if v := x.(map[string]any); v["id"] == qid {
				return v["text_hidden"] != true && v["title"] != nil
			}
		}
		t.Fatal("вопрос не найден у бадди")
		return false
	}
	if !visible() {
		t.Fatal("по умолчанию текст виден бадди")
	}
	code, r = call(t, h, "PATCH", "/api/settings", "intern.sokolov", `{"question_text_visible_to_buddy":false,"proactivity":"off","reminders":["1d"],"quiet_hours":3}`)
	set = r["settings"].(map[string]any)
	if code != 200 || set["proactivity"] != "off" || set["question_text_visible_to_buddy"] != false || set["quiet_until"] == nil || len(set["reminders"].([]any)) != 1 {
		t.Fatalf("сохранение: %d %v", code, r)
	}
	if visible() {
		t.Fatal("после выключения бадди не видит текст (BR-86)")
	}
	if _, r = call(t, h, "PATCH", "/api/settings", "intern.sokolov", `{"quiet_hours":0}`); r["settings"].(map[string]any)["quiet_until"] != nil {
		t.Fatal("«не беспокоить» выключается")
	}
	for body, want := range map[string]int{
		`{"proactivity":"always"}`: 422, `{"reminders":["5m"]}`: 422, `{"quiet_hours":48}`: 422, `{"phone":"1"}`: 422, `not json`: 422,
	} {
		if code, _ := call(t, h, "PATCH", "/api/settings", "intern.sokolov", body); code != want {
			t.Errorf("%s → %d, ждали %d", body, code, want)
		}
	}
	if code, _ := call(t, h, "PATCH", "/api/settings", "lead.kovalev", `{"question_text_visible_to_buddy":false}`); code != 403 {
		t.Fatal("видимость вопросов — настройка стажёра")
	}
	if code, _ := call(t, h, "PATCH", "/api/settings", "lead.kovalev", `{"quiet_hours":1}`); code != 200 {
		t.Fatal("«не беспокоить» доступно всем")
	}
}
