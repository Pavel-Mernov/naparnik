package llm

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAICompatible — модели с API в формате OpenAI: DeepSeek (по умолчанию), а также vLLM, llama.cpp и др.
// Адрес — LLM_BASE_URL, ключ — LLM_API_KEY или DEEPSEEK_API_KEY.
type OpenAICompatible struct {
	client *openai.Client
	cfg    Config
}

func NewOpenAICompatible(c Config, defaultBaseURL string, opts ...option.RequestOption) *OpenAICompatible {
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	opts = append([]option.RequestOption{option.WithBaseURL(base), option.WithAPIKey(c.APIKey)}, opts...)
	client := openai.NewClient(opts...)
	return &OpenAICompatible{client: &client, cfg: c}
}

func (o *OpenAICompatible) Start(system, context string, tools []ToolSpec) Session {
	var ts []openai.ChatCompletionToolUnionParam
	for _, t := range tools {
		ts = append(ts, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Description), Parameters: shared.FunctionParameters(t.Schema),
		}))
	}
	return &openaiSession{o: o, tools: ts, messages: []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(system + "\n\n" + context),
	}}
}

type openaiSession struct {
	o        *OpenAICompatible
	tools    []openai.ChatCompletionToolUnionParam
	messages []openai.ChatCompletionMessageParamUnion
}

func (s *openaiSession) call(ctx context.Context) (Step, error) {
	resp, err := s.o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: s.o.cfg.Model, Messages: s.messages, Tools: s.tools, MaxTokens: openai.Int(8000),
	})
	if err != nil {
		return Step{}, err
	}
	if len(resp.Choices) == 0 {
		return Step{Text: ""}, nil
	}
	msg := resp.Choices[0].Message
	s.messages = append(s.messages, msg.ToParam())
	step := Step{Text: msg.Content}
	for _, tc := range msg.ToolCalls {
		input := map[string]any{}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input) // битый JSON → пустые аргументы, инструмент вернёт ошибку
		step.ToolCalls = append(step.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: input})
	}
	return step, nil
}

func (s *openaiSession) SendUser(ctx context.Context, text string) (Step, error) {
	s.messages = append(s.messages, openai.UserMessage(text))
	return s.call(ctx)
}

func (s *openaiSession) SendToolResults(ctx context.Context, results []ToolResult) (Step, error) {
	for _, r := range results {
		s.messages = append(s.messages, openai.ToolMessage(r.Content, r.Call.ID))
	}
	return s.call(ctx)
}
