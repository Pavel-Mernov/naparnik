// Package flags — feature flags (в Авито — runtime feature flags). Функции включаются и выключаются без
// пересборки: значения из FEATURE_FLAGS и файла FLAGS_FILE, файл перечитывается при изменении.
//
//	FEATURE_FLAGS="practice=off,glossary=on"
//	FLAGS_FILE=flags.json   {"practice": {"on": true, "roles": ["intern"]}, "telegram": {"on": false}}
//
// Флаг без значения считается включённым: выключенным может быть только то, что явно выключили.
// Роли — постепенное включение: флаг действует только для перечисленных ролей.
package flags

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// Известные флаги.
const (
	Practice = "practice" // практика перед задачей (SC-30…32)
	Glossary = "glossary" // жаргон и набор знаний роли (SC-34)
	Meetings = "meetings" // встречи по вопросам (SC-10, SC-11)
	Telegram = "telegram" // доставка уведомлений в Telegram (SC-24)
	Watchers = "watchers" // сторожа сроков: SLA, простой, автоактивация плана
)

type Flag struct {
	On    bool     `json:"on"`
	Roles []string `json:"roles,omitempty"`
}

type Provider struct {
	mu      sync.Mutex
	env     map[string]Flag
	file    string
	fileMod time.Time
	fromF   map[string]Flag
	checked time.Time
}

// FromEnv — флаги из FEATURE_FLAGS и FLAGS_FILE.
func FromEnv() *Provider {
	p := &Provider{env: parse(os.Getenv("FEATURE_FLAGS")), file: os.Getenv("FLAGS_FILE")}
	p.reload()
	return p
}

func parse(s string) map[string]Flag {
	out := map[string]Flag{}
	for _, part := range strings.Split(s, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		if k == "" {
			continue
		}
		v = strings.ToLower(strings.TrimSpace(v))
		out[k] = Flag{On: v == "" || v == "on" || v == "true" || v == "1"}
	}
	return out
}

func (p *Provider) reload() {
	if p.file == "" {
		return
	}
	st, err := os.Stat(p.file)
	if err != nil || !st.ModTime().After(p.fileMod) {
		return
	}
	raw, err := os.ReadFile(p.file)
	if err != nil {
		return
	}
	m := map[string]Flag{}
	if json.Unmarshal(raw, &m) == nil {
		p.fromF, p.fileMod = m, st.ModTime()
	}
}

// Enabled — включён ли флаг для пользователя с такими ролями. Файл важнее переменной окружения.
func (p *Provider) Enabled(name string, roles []string) bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.checked) > 5*time.Second {
		p.reload()
		p.checked = time.Now()
	}
	f, ok := p.fromF[name]
	if !ok {
		f, ok = p.env[name]
	}
	if !ok {
		return true
	}
	if !f.On {
		return false
	}
	if len(f.Roles) == 0 {
		return true
	}
	for _, r := range roles {
		for _, want := range f.Roles {
			if r == want {
				return true
			}
		}
	}
	return false
}

// Snapshot — текущие значения для /api/health (без ролей пользователя).
func (p *Provider) Snapshot() map[string]bool {
	out := map[string]bool{}
	for _, n := range []string{Practice, Glossary, Meetings, Telegram, Watchers} {
		out[n] = p.Enabled(n, nil) || p.hasRoles(n)
	}
	return out
}

func (p *Provider) hasRoles(name string) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if f, ok := p.fromF[name]; ok {
		return f.On && len(f.Roles) > 0
	}
	return false
}

// Default — флаги процесса; задаёт app.New. nil — всё включено (тесты, CLI).
var Default *Provider

// On — включён ли флаг для этих ролей по флагам процесса.
func On(name string, roles []string) bool { return Default.Enabled(name, roles) }
