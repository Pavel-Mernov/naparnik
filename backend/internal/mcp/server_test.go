package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/store"
)

func setup(t *testing.T, login string) (*Server, *store.Store) {
	t.Helper()
	d, err := mockdata.Load("../../../docs/04-mock-data")
	if err != nil {
		t.Fatal(err)
	}
	today, _ := time.Parse(time.DateOnly, d.DemoToday)
	s := store.New(d, today)
	u, _ := s.UserByLogin(login)
	return &Server{Store: s, User: u, WebURL: "http://localhost:3000"}, s
}

func rpc(t *testing.T, srv *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := srv.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		_ = json.Unmarshal([]byte(l), &m)
		res = append(res, m)
	}
	return res
}

func text(r map[string]any) (string, bool) {
	res := r["result"].(map[string]any)
	return res["content"].([]any)[0].(map[string]any)["text"].(string), res["isError"].(bool)
}

// Рекомендация кейса: действия агента через MCP — те же инструменты и права, действия только черновиком.
func TestMCPToolsAndRights(t *testing.T) {
	srv, s := setup(t, "intern.sokolov")
	r := rpc(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_kb","arguments":{"query":"отгул"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"request_access","arguments":{"system":"sys-redash","reason":"дашборды"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"request_access","arguments":{"system":"sys-dwh-pii","reason":"x"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"nope"}`,
		`not json`,
	)
	if len(r) != 7 { // уведомление без ответа
		t.Fatal(len(r), r)
	}
	if r[0]["result"].(map[string]any)["protocolVersion"] != ProtocolVersion {
		t.Fatal(r[0])
	}
	names := map[string]bool{}
	for _, tl := range r[1]["result"].(map[string]any)["tools"].([]any) {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["search_kb"] || !names["request_access"] || names["mark_step_done"] == false {
		t.Fatal(names)
	}
	if txt, isErr := text(r[2]); isErr || !strings.Contains(txt, "kb-02") {
		t.Fatal(txt)
	}
	txt, isErr := text(r[3])
	if isErr || !strings.Contains(txt, "needs_confirmation") || len(s.Requests("")) != 0 {
		t.Fatal("действие — только черновик", txt)
	}
	if _, isErr := text(r[4]); !isErr {
		t.Fatal("система с ПДн стажёру не положена")
	}
	if r[5]["error"] == nil || r[6]["error"] == nil {
		t.Fatal(r[5], r[6])
	}
	// Черновик из MCP подтверждает человек обычным путём Напарника — появляется квитанция.
	var draft struct {
		DraftID string `json:"draft_id"`
	}
	_ = json.Unmarshal([]byte(txt), &draft)
	a := agent.New(s, llm.NewStub("stub_ok", s.Today), time.Second)
	if res := a.Confirm("intern.sokolov", draft.DraftID); !res.OK || len(s.Requests("")) != 1 {
		t.Fatal(res)
	}
}

// Руководитель через MCP не получает инструменты стажёра (BR-40).
func TestMCPToolsByRole(t *testing.T) {
	srv, _ := setup(t, "lead.kovalev")
	r := rpc(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_question","arguments":{"text":"x","addressee_id":"u-orlov"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"ping"}`)
	for _, tl := range r[0]["result"].(map[string]any)["tools"].([]any) {
		if tl.(map[string]any)["name"] == "send_question" {
			t.Fatal("send_question — только стажёру")
		}
	}
	if _, isErr := text(r[1]); !isErr {
		t.Fatal("чужой инструмент")
	}
	if r[2]["error"] == nil || r[3]["error"] != nil {
		t.Fatal(r[2], r[3])
	}
}
