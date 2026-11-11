package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	aopt "github.com/anthropics/anthropic-sdk-go/v2/option"
	oopt "github.com/openai/openai-go/v3/option"
)

var today = time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) // среда

var spec = []ToolSpec{{Name: "get_plan", Description: "План", Schema: map[string]any{
	"type": "object", "properties": map[string]any{"intern_id": map[string]any{"type": "string"}}, "required": []string{},
}}}

func TestConfigFromEnv(t *testing.T) {
	for _, k := range []string{"LLM_PROVIDER", "LLM_MODEL", "LLM_API_KEY", "LLM_MODE", "LLM_TIMEOUT_SEC", "ANTHROPIC_API_KEY", "LLM_EFFORT"} {
		t.Setenv(k, "")
	}
	c, err := ConfigFromEnv()
	if err != nil || c.Provider != "stub" || c.Mode != "stub_ok" || c.Timeout != 30*time.Second || c.Effort != "medium" {
		t.Fatalf("по умолчанию заглушка: %+v %v", c, err)
	}
	t.Setenv("LLM_PROVIDER", "claude")
	t.Setenv("ANTHROPIC_API_KEY", "k")
	t.Setenv("LLM_TIMEOUT_SEC", "5")
	c, _ = ConfigFromEnv()
	if c.Provider != "anthropic" || c.Model != "claude-opus-5-5" || c.APIKey != "k" || c.Timeout != 5*time.Second {
		t.Fatalf("claude: %+v", c)
	}
	t.Setenv("LLM_MODE", "stub_error")
	if c, _ = ConfigFromEnv(); c.Provider != "stub" || c.Mode != "stub_error" {
		t.Fatalf("режим заглушки важнее провайдера: %+v", c)
	}
	t.Setenv("LLM_MODE", "")
	t.Setenv("LLM_PROVIDER", "gpt")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("неизвестный провайдер")
	}
}

func TestNewPicksProvider(t *testing.T) {
	if _, ok := New(Config{Provider: "anthropic", APIKey: "k"}, today).(*Anthropic); !ok {
		t.Error("anthropic")
	}
	if _, ok := New(Config{Provider: "deepseek", APIKey: "k"}, today).(*OpenAICompatible); !ok {
		t.Error("deepseek")
	}
	if _, ok := New(Config{Provider: "stub", Mode: "stub_ok"}, today).(*Stub); !ok {
		t.Error("stub")
	}
}

func TestParseDay(t *testing.T) {
	cases := map[string]string{"отгул в пятницу": "2026-11-13", "завтра": "2026-11-12", "на 2026-12-01": "2026-12-01",
		"в среду": "2026-11-18", "когда-нибудь": ""}
	for in, want := range cases {
		if got := ParseDay(in, today); got != want {
			t.Errorf("%q: %q, ожидалось %q", in, got, want)
		}
	}
}

// ---------- заглушка ----------

func send(t *testing.T, s Session, text string) Step {
	t.Helper()
	st, err := s.SendUser(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStubRoutesToTools(t *testing.T) {
	all := []ToolSpec{}
	for _, n := range []string{"get_today", "get_plan", "search_kb", "team_directory", "find_expert", "send_question",
		"get_absence_rules", "precheck_absence", "create_absence_request", "get_required_accesses", "get_access_rules", "request_access"} {
		all = append(all, ToolSpec{Name: n})
	}
	stub := NewStub("stub_ok", today)
	cases := map[string]string{
		"мне нужен отгул в пятницу": "get_absence_rules", "нужен доступ к Redash": "get_access_rules",
		"Хочу сам посмотреть данные в витринах DWH, как туда попасть?": "get_access_rules",
		"Как оформить доступ к DWH?":                                   "get_access_rules",
		"какие доступы нужны":                                          "get_required_accesses", "что мне сегодня делать": "get_today",
		"покажи мой план": "get_plan", "кто есть кто": "team_directory", "что такое витрина": "search_kb",
		"Не помогло, нужен человек: что такое витрина": "find_expert",
	}
	for in, want := range cases {
		if st := send(t, stub.Start("", "", all), in); len(st.ToolCalls) == 0 || st.ToolCalls[0].Name != want {
			t.Errorf("%q → %+v, ожидался %s", in, st.ToolCalls, want)
		}
	}
	if st := send(t, stub.Start("", "", all), "нужен отгул"); !strings.Contains(st.Text, "дату") {
		t.Error("без даты — уточнить")
	}
	if st := send(t, stub.Start("", "", nil), "что мне сегодня делать"); st.Text == "" {
		t.Error("инструмент недоступен роли — текстовый отказ")
	}
}

func result(call ToolCall, v any) ToolResult {
	raw, _ := json.Marshal(v)
	return ToolResult{Call: call, Content: string(raw)}
}

func TestStubComposesAnswers(t *testing.T) {
	all := []ToolSpec{{Name: "search_kb"}, {Name: "find_expert"}, {Name: "send_question"}}
	s := NewStub("stub_ok", today).Start("", "", all)
	st := send(t, s, "Почему конверсия отличается?")
	// поиск не нашёл → ищем человека → готовим вопрос
	st, _ = s.SendToolResults(context.Background(), []ToolResult{result(st.ToolCalls[0], map[string]any{"ok": true, "data": map[string]any{"found": false}})})
	if st.ToolCalls[0].Name != "find_expert" {
		t.Fatalf("ожидался find_expert: %+v", st)
	}
	st, _ = s.SendToolResults(context.Background(), []ToolResult{result(st.ToolCalls[0], map[string]any{"ok": true,
		"data": map[string]any{"addressee_id": "u-orlov", "name": "Михаил", "reason": "автор"}})})
	if st.ToolCalls[0].Name != "send_question" || st.ToolCalls[0].Input["addressee_id"] != "u-orlov" {
		t.Fatalf("ожидался send_question: %+v", st)
	}
	st, _ = s.SendToolResults(context.Background(), []ToolResult{result(st.ToolCalls[0], map[string]any{"ok": true, "status": "needs_confirmation"})})
	if !strings.Contains(st.Text, "черновик") {
		t.Fatalf("черновик: %q", st.Text)
	}

	texts := []struct {
		tool string
		data any
		want string
	}{
		{"search_kb", map[string]any{"found": true, "hits": []map[string]any{{"title": "T", "body": "B", "author": "A", "updated_at": "D", "label": "L"}}}, "Источник: «T», A, D · L"},
		{"get_today", map[string]any{"week": 1, "steps": []map[string]any{{"title": "S", "status": "Overdue"}}, "events": []map[string]any{{"title": "E", "start": "15:00"}}}, "просрочен"},
		{"get_plan", map[string]any{"progress_pct": 40, "next_step": map[string]any{"title": "N"}}, "40%"},
		{"get_plan", map[string]any{"progress_pct": 100}, "всё пройдено"},
		{"precheck_absence", map[string]any{"warnings": []map[string]any{{"message": "W"}}}, "Обрати внимание: W"},
		{"get_absence_rules", map[string]any{"rules": "R", "source": map[string]any{"title": "T", "label": "L"}}, "сделаю за тебя"},
		{"team_directory", map[string]any{"people": []map[string]any{{"name": "N", "responsibility": "R"}}}, "N — R"},
		{"get_required_accesses", map[string]any{"systems": []map[string]any{{"title": "Redash", "status": "pending"}}}, "Redash: pending"},
		{"precheck_absence", map[string]any{"warnings": []any{}}, "Готово."},
	}
	for _, tc := range texts {
		s := NewStub("stub_ok", today).Start("", "", nil)
		st, _ := s.SendToolResults(context.Background(), []ToolResult{result(ToolCall{Name: tc.tool}, map[string]any{"ok": true, "data": tc.data})})
		if !strings.Contains(st.Text, tc.want) {
			t.Errorf("%s: %q не содержит %q", tc.tool, st.Text, tc.want)
		}
	}
	st, _ = NewStub("stub_ok", today).Start("", "", nil).SendToolResults(context.Background(), []ToolResult{result(ToolCall{Name: "request_access"},
		map[string]any{"ok": false, "error": map[string]any{"message": "Вне зоны.", "who_can": "Андрей"}})})
	if !strings.Contains(st.Text, "Андрей") {
		t.Fatalf("ошибка с адресатом: %q", st.Text)
	}
}

func TestStubModes(t *testing.T) {
	if _, err := NewStub("stub_error", today).Start("", "", nil).SendUser(context.Background(), "x"); !errors.Is(err, ErrStub) {
		t.Fatal("stub_error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := NewStub("stub_timeout", today).Start("", "", nil).SendUser(ctx, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("stub_timeout")
	}
	t.Setenv("LLM_STUB_SLOW_SEC", "0")
	if _, err := NewStub("stub_slow", today).Start("", "", []ToolSpec{{Name: "search_kb"}}).SendUser(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_STUB_SLOW_SEC", "60")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if _, err := NewStub("stub_slow", today).Start("", "", nil).SendToolResults(ctx2, nil); err == nil {
		t.Fatal("stub_slow должен уважать таймаут")
	}
}

// ---------- Claude и DeepSeek через локальный тестовый сервер ----------

func fakeAPI(t *testing.T, responses ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var bodies []string
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		if i >= len(responses) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
			return
		}
		_, _ = w.Write([]byte(responses[i]))
		i++
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func TestAnthropicToolLoop(t *testing.T) {
	srv, bodies := fakeAPI(t,
		`{"id":"m1","type":"message","role":"assistant","model":"m","content":[{"type":"tool_use","id":"t1","name":"get_plan","input":{"intern_id":"u-1"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
		`{"id":"m2","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"Прогресс 40%"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
		`{"id":"m3","type":"message","role":"assistant","model":"m","content":[],"stop_reason":"refusal","usage":{"input_tokens":1,"output_tokens":1}}`,
	)
	p := NewAnthropic(Config{Model: "claude-opus-5-5", APIKey: "k", Effort: "medium"}, aopt.WithBaseURL(srv.URL), aopt.WithMaxRetries(0))
	s := p.Start("SYS", "CTX", spec)
	st := send(t, s, "где я?")
	if len(st.ToolCalls) != 1 || st.ToolCalls[0].Input["intern_id"] != "u-1" {
		t.Fatalf("вызов инструмента: %+v", st)
	}
	st, err := s.SendToolResults(context.Background(), []ToolResult{{Call: st.ToolCalls[0], Content: `{"ok":true}`}})
	if err != nil || st.Text != "Прогресс 40%" {
		t.Fatalf("текст: %+v %v", st, err)
	}
	if st = send(t, s, "?"); !st.Refused {
		t.Fatal("отказ модели")
	}
	first := (*bodies)[0]
	for _, want := range []string{`"cache_control"`, `"effort":"medium"`, `"name":"get_plan"`, `"CTX"`} {
		if !strings.Contains(first, want) {
			t.Errorf("в запросе нет %s: %s", want, first)
		}
	}
	if !strings.Contains((*bodies)[1], `"tool_use_id":"t1"`) {
		t.Error("результат инструмента не передан")
	}
	if _, err := s.SendUser(context.Background(), "x"); err == nil {
		t.Fatal("ошибка API должна вернуться")
	}
}

func TestOpenAICompatibleToolLoop(t *testing.T) {
	srv, bodies := fakeAPI(t,
		`{"id":"c1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"get_plan","arguments":"{\"intern_id\":\"u-1\"}"}},{"id":"c2","type":"function","function":{"name":"get_plan","arguments":"{broken"}}]}}]}`,
		`{"id":"c2","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Готово"}}]}`,
		`{"id":"c3","object":"chat.completion","created":1,"model":"m","choices":[]}`,
	)
	p := NewOpenAICompatible(Config{Model: "deepseek-chat", APIKey: "k"}, srv.URL, oopt.WithMaxRetries(0))
	s := p.Start("SYS", "CTX", spec)
	st := send(t, s, "где я?")
	if len(st.ToolCalls) != 2 || st.ToolCalls[0].Input["intern_id"] != "u-1" || len(st.ToolCalls[1].Input) != 0 {
		t.Fatalf("вызовы: %+v", st)
	}
	st, err := s.SendToolResults(context.Background(), []ToolResult{{Call: st.ToolCalls[0], Content: "{}"}})
	if err != nil || st.Text != "Готово" {
		t.Fatalf("%+v %v", st, err)
	}
	if st = send(t, s, "?"); st.Text != "" {
		t.Fatal("пустой ответ без choices")
	}
	if !strings.Contains((*bodies)[0], `"SYS\n\nCTX"`) || !strings.Contains((*bodies)[1], `"tool_call_id":"c1"`) {
		t.Errorf("формат запросов: %s", (*bodies)[0])
	}
	if _, err := s.SendUser(context.Background(), "x"); err == nil {
		t.Fatal("ошибка API должна вернуться")
	}
}

func TestToStrings(t *testing.T) {
	if got := toStrings([]any{"a", 1, "b"}); len(got) != 2 {
		t.Fatal(got)
	}
	if toStrings(42) != nil {
		t.Fatal("не список")
	}
}

func TestKeyOnlyEnablesModel(t *testing.T) {
	for _, k := range []string{"LLM_PROVIDER", "LLM_MODEL", "LLM_API_KEY", "LLM_MODE", "ANTHROPIC_API_KEY", "DEEPSEEK_API_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("LLM_API_KEY", "k")
	if c, _ := ConfigFromEnv(); c.Provider != "anthropic" || c.Mode != "live" {
		t.Fatalf("только ключ → Claude: %+v", c)
	}
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "d")
	if c, _ := ConfigFromEnv(); c.Provider != "deepseek" || c.APIKey != "d" {
		t.Fatalf("ключ DeepSeek → DeepSeek: %+v", c)
	}
	t.Setenv("LLM_MODE", "stub_ok")
	if c, _ := ConfigFromEnv(); c.Provider != "stub" {
		t.Fatalf("явная заглушка важнее ключа: %+v", c)
	}
}
