package llm

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Anthropic — Claude через официальный Go SDK. Ключ: LLM_API_KEY или ANTHROPIC_API_KEY.
//
// Стабильная часть (инструкции и инструменты) кэшируется — повторные запросы дешевле.
// Контекст пользователя — отдельным блоком после кэшируемой части.
type Anthropic struct {
	client *anthropic.Client
	cfg    Config
}

func NewAnthropic(c Config, opts ...option.RequestOption) *Anthropic {
	if c.APIKey != "" {
		opts = append(opts, option.WithAPIKey(c.APIKey))
	}
	client := anthropic.NewClient(opts...)
	return &Anthropic{client: &client, cfg: c}
}

func (a *Anthropic) Start(system, context string, tools []ToolSpec) Session {
	var ts []anthropic.ToolUnionParam
	for _, t := range tools {
		tp := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: t.Schema["properties"], Required: toStrings(t.Schema["required"])},
		}
		ts = append(ts, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return &anthropicSession{a: a, tools: ts, system: []anthropic.TextBlockParam{
		{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()},
		{Text: context},
	}}
}

type anthropicSession struct {
	a        *Anthropic
	tools    []anthropic.ToolUnionParam
	system   []anthropic.TextBlockParam
	messages []anthropic.MessageParam
}

func (s *anthropicSession) call(ctx context.Context) (Step, error) {
	resp, err := s.a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        s.a.cfg.Model,
		MaxTokens:    16000,
		System:       s.system,
		Tools:        s.tools,
		Messages:     s.messages,
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(s.a.cfg.Effort)},
	})
	if err != nil {
		return Step{}, err
	}
	// Ответ модели (включая блоки размышлений) возвращаем в историю без изменений
	s.messages = append(s.messages, resp.ToParam())
	step := Step{Refused: resp.StopReason == anthropic.StopReasonRefusal}
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			step.Text += b.Text
		case anthropic.ToolUseBlock:
			input := map[string]any{}
			_ = json.Unmarshal([]byte(b.JSON.Input.Raw()), &input)
			step.ToolCalls = append(step.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Input: input})
		}
	}
	return step, nil
}

func (s *anthropicSession) SendUser(ctx context.Context, text string) (Step, error) {
	s.messages = append(s.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
	return s.call(ctx)
}

func (s *anthropicSession) SendToolResults(ctx context.Context, results []ToolResult) (Step, error) {
	// Все результаты — одним сообщением, иначе модель перестаёт вызывать инструменты параллельно
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(results))
	for _, r := range results {
		blocks = append(blocks, anthropic.NewToolResultBlock(r.Call.ID, r.Content, false))
	}
	s.messages = append(s.messages, anthropic.NewUserMessage(blocks...))
	return s.call(ctx)
}

func toStrings(v any) []string {
	switch xs := v.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
