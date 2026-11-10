// Package llm — общий интерфейс провайдера модели. Агент не знает, что внутри: Claude, DeepSeek или заглушка.
//
// Сессия хранит историю в «родном» формате своего API и наружу отдаёт только текст и вызовы инструментов.
package llm

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any // JSON Schema: type, properties, required
}

type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

type ToolResult struct {
	Call    ToolCall
	Content string // JSON-результат инструмента
}

type Step struct {
	Text      string
	ToolCalls []ToolCall
	Refused   bool
}

type Session interface {
	SendUser(ctx context.Context, text string) (Step, error)
	SendToolResults(ctx context.Context, results []ToolResult) (Step, error)
}

type Provider interface {
	Start(system, context string, tools []ToolSpec) Session
}

// Config — переменные окружения из docs/04-mock-data/llm-stub.json.
type Config struct {
	Provider string        // anthropic | deepseek | stub
	Model    string        // пусто — по умолчанию для провайдера
	APIKey   string        // LLM_API_KEY (или ANTHROPIC_API_KEY / DEEPSEEK_API_KEY)
	BaseURL  string        // для OpenAI-совместимых API
	Mode     string        // live | stub_ok | stub_slow | stub_timeout | stub_error
	Timeout  time.Duration // LLM_TIMEOUT_SEC, по умолчанию 30 с (BR-70)
	Effort   string        // глубина размышлений Claude: low | medium | high
}

var defaultModels = map[string]string{
	"anthropic": "claude-opus-5-5", // дешевле: LLM_MODEL=claude-sonnet-5-5
	"deepseek":  "deepseek-chat",   // проверьте актуальное имя в кабинете DeepSeek
	"stub":      "stub",
}

func ConfigFromEnv() (Config, error) {
	c := Config{
		Provider: strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER"))),
		Model:    os.Getenv("LLM_MODEL"),
		APIKey:   os.Getenv("LLM_API_KEY"),
		BaseURL:  os.Getenv("LLM_BASE_URL"),
		Mode:     os.Getenv("LLM_MODE"),
		Effort:   os.Getenv("LLM_EFFORT"),
		Timeout:  30 * time.Second,
	}
	if c.Provider == "claude" {
		c.Provider = "anthropic"
	}
	// «Вставил ключ — заработало»: провайдер не указан, но есть ключ → включаем соответствующую модель.
	if c.Provider == "" && !strings.HasPrefix(c.Mode, "stub") {
		switch {
		case c.APIKey != "" || os.Getenv("ANTHROPIC_API_KEY") != "":
			c.Provider = "anthropic"
		case os.Getenv("DEEPSEEK_API_KEY") != "":
			c.Provider = "deepseek"
		}
	}
	if c.Mode == "" {
		c.Mode = "live"
	}
	if c.Provider == "" || strings.HasPrefix(c.Mode, "stub") {
		c.Provider = "stub"
		if c.Mode == "live" {
			c.Mode = "stub_ok"
		}
	}
	if _, known := defaultModels[c.Provider]; !known {
		return c, fmt.Errorf("LLM_PROVIDER=%q: допустимо anthropic, deepseek или stub", c.Provider)
	}
	if c.Model == "" {
		c.Model = defaultModels[c.Provider]
	}
	if c.Effort == "" {
		c.Effort = "medium"
	}
	if v, err := strconv.Atoi(os.Getenv("LLM_TIMEOUT_SEC")); err == nil && v > 0 {
		c.Timeout = time.Duration(v) * time.Second
	}
	if c.APIKey == "" {
		c.APIKey = map[string]string{"anthropic": os.Getenv("ANTHROPIC_API_KEY"), "deepseek": os.Getenv("DEEPSEEK_API_KEY")}[c.Provider]
	}
	return c, nil
}

// New создаёт провайдера по конфигу. today нужен заглушке для разбора дат («в пятницу»).
func New(c Config, today time.Time) Provider {
	switch c.Provider {
	case "anthropic":
		return NewAnthropic(c)
	case "deepseek":
		return NewOpenAICompatible(c, "https://api.deepseek.com")
	default:
		return NewStub(c.Mode, today)
	}
}
