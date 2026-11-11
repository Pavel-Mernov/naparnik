package store

import (
	"encoding/json"

	"github.com/worker4556/naparnik/backend/internal/practice"
)

func (s *Store) Practice(intern, template string) (practice.State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.practices[practice.Key(intern, template)]
	raw, _ := json.Marshal(v)
	var out practice.State
	_ = json.Unmarshal(raw, &out)
	return out, ok
}

// Запись вызывается только внутри транзакции; JSON-копия не оставляет изменяемых ссылок у вызывающего.
func (s *Store) PutPractice(v practice.State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(v)
	var out practice.State
	_ = json.Unmarshal(raw, &out)
	s.practices[practice.Key(v.InternID, v.TemplateID)] = out
}

// Только недостающие источники практики из опубликованной конфигурации отдела; не перезаписываем страницы.
func (s *Store) EnsurePracticeSources(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, p := range s.pages {
		seen[p.ID] = true
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	spaces := s.ReadableSpaces()
	for _, p := range s.Data.KB.Pages {
		if wanted[p.ID] && !seen[p.ID] && p.Status == "Published" && spaces[p.Space] {
			s.pages = append(s.pages, p)
			seen[p.ID] = true
		}
	}
}
