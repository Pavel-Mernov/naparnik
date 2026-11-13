package httpapi

import "testing"

// История чата: вопрос не пропадает после закрытия чата и перезагрузки, черновик показывается
// в текущем состоянии, «Новый разговор» очищает окно, но не «Мои вопросы», чужой истории не видно.
func TestChatHistory(t *testing.T) {
	_, h := newServer(t)
	_, reply := call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Мне нужен отгул в пятницу"}`)
	id := reply["drafts"].([]any)[0].(map[string]any)["id"].(string)
	call(t, h, "POST", "/api/drafts/"+id+"/confirm", "intern.sokolov", "")
	call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Что такое витрина?"}`)

	code, hist := call(t, h, "GET", "/api/chat/history", "intern.sokolov", "")
	turns, _ := hist["turns"].([]any)
	if code != 200 || len(turns) != 2 || hist["conversation"].(float64) != 0 {
		t.Fatalf("история: %d %v", code, hist)
	}
	first := turns[0].(map[string]any)
	if first["question"] != "Мне нужен отгул в пятницу" {
		t.Fatalf("порядок: %v", first)
	}
	d := first["reply"].(map[string]any)["drafts"].([]any)[0].(map[string]any)
	if d["status"] != "confirmed" || d["result"].(map[string]any)["receipt"].(map[string]any)["id"] != "ABS-1042" {
		t.Fatalf("черновик в истории не подтверждён: %v", d)
	}

	if code, _ := call(t, h, "POST", "/api/chat/reset", "intern.sokolov", ""); code != 200 {
		t.Fatal(code)
	}
	call(t, h, "POST", "/api/chat/messages", "intern.sokolov", `{"text":"Что мне сегодня делать?"}`)
	_, hist = call(t, h, "GET", "/api/chat/history", "intern.sokolov", "")
	turns = hist["turns"].([]any)
	last := turns[len(turns)-1].(map[string]any)
	if len(turns) != 3 || hist["conversation"].(float64) != 1 || last["conversation"].(float64) != 1 || turns[0].(map[string]any)["conversation"].(float64) != 0 {
		t.Fatalf("после «Новый разговор»: %v", hist)
	}

	if _, other := call(t, h, "GET", "/api/chat/history", "intern.egorova", ""); len(other["turns"].([]any)) != 0 {
		t.Fatalf("чужая история: %v", other)
	}
}
