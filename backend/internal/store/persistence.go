package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/practice"
)

// Для небольшого прототипа состояние хранится одним версионированным JSONB-документом.
// Блокировка строки объединяет проверку прав/статуса и все изменения в одну транзакцию,
// в том числе при двух экземплярах backend. Сетевые вызовы LLM сюда не входят.
type snapshot struct {
	Version    int                                         `json:"version"`
	Steps      map[string]map[string]*mockdata.TrackerStep `json:"steps"`
	Requests   []*Request                                  `json:"requests"`
	Questions  []*Question                                 `json:"questions"`
	Pages      []mockdata.Page                             `json:"pages"`
	Seq        map[string]int                              `json:"seq"`
	Drafts     map[string]ActionDraft                      `json:"drafts"`
	Operations map[string]Operation                        `json:"operations"`
	Practices  map[string]practice.State                   `json:"practices"`
	// Журнал событий, уведомления и привязки Telegram. В старом сохранённом состоянии их нет — это пустые списки.
	Events        []LogEvent       `json:"events,omitempty"`
	Notifications []Notification   `json:"notifications,omitempty"`
	TGLinks       map[string]int64 `json:"tg_links,omitempty"`
}

type ActionDraft struct {
	ID     string          `json:"id"`
	UserID string          `json:"user_id"`
	Tool   string          `json:"tool"`
	Args   map[string]any  `json:"args"`
	Result json.RawMessage `json:"result,omitempty"`
}

// Operation — результат принятого запроса с ключом идемпотентности.
type Operation struct {
	Fingerprint string          `json:"fingerprint"`
	Result      json.RawMessage `json:"result"`
}

func (s *Store) snapshot() snapshot {
	return snapshot{2, s.steps, s.requests, s.questions, s.pages, s.seq, s.drafts, s.operations, s.practices, s.events, s.notifications, s.tgLinks}
}

func (s *Store) restore(v snapshot) error {
	if (v.Version != 1 && v.Version != 2) || v.Steps == nil || v.Seq == nil || v.Drafts == nil || v.Operations == nil {
		return errors.New("unsupported or corrupt stored state")
	}
	s.steps, s.requests, s.questions, s.pages = v.Steps, v.Requests, v.Questions, v.Pages
	s.seq, s.drafts, s.operations = v.Seq, v.Drafts, v.Operations
	s.practices = v.Practices
	if s.practices == nil {
		s.practices = map[string]practice.State{}
	}
	s.events, s.notifications, s.tgLinks = v.Events, v.Notifications, v.TGLinks
	if s.tgLinks == nil {
		s.tgLinks = map[string]int64{}
	}
	return nil
}

func (s *Store) decode(raw []byte) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return s.restore(v)
}

// OpenPostgres создаёт схему и начальное состояние только при первом запуске.
// Повторный запуск не затирает данные фикстурами. При ошибке БД нет отката в память.
func OpenPostgres(ctx context.Context, d *mockdata.Data, today time.Time, url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	s := New(d, today)
	s.db = db
	err = func() error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Два первых запуска тоже не должны одновременно создавать системную таблицу.
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7394556)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS naparnik_state (
			id integer PRIMARY KEY CHECK (id = 1), payload jsonb NOT NULL)`); err != nil {
			return err
		}
		raw, err := json.Marshal(s.snapshot())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO naparnik_state(id, payload) VALUES (1, $1)
			ON CONFLICT (id) DO NOTHING`, raw); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return s.Refresh(ctx)
	}()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open persistent store: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Refresh обновляет представление для чтения; другой процесс мог изменить БД.
func (s *Store) Refresh(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM naparnik_state WHERE id = 1`).Scan(&raw); err != nil {
		return err
	}
	return s.decode(raw)
}

// Transaction применяет изменение к отдельной копии. При ошибке нет ни частичного
// изменения памяти, ни записи в БД. Вложенные действия используют ту же копию.
func (s *Store) Transaction(fn func(*Store) error) error {
	if s.inTransaction {
		return fn(s)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var tx *sql.Tx
	var raw []byte
	var err error
	if s.db != nil {
		tx, err = s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		err = tx.QueryRowContext(ctx, `SELECT payload FROM naparnik_state WHERE id = 1 FOR UPDATE`).Scan(&raw)
	} else {
		raw, err = json.Marshal(s.snapshot())
	}
	if err != nil {
		return err
	}
	work := &Store{Data: s.Data, Today: s.Today, inTransaction: true}
	if err := work.decode(raw); err != nil {
		return err
	}
	if err := fn(work); err != nil {
		return err
	}
	// Ещё одна копия не позволяет результату инструмента менять сохранённое состояние по указателю.
	raw, err = json.Marshal(work.snapshot())
	if err != nil {
		return err
	}
	if tx != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE naparnik_state SET payload = $1 WHERE id = 1`, raw); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return s.decode(raw)
}

func (s *Store) PutDraft(d ActionDraft) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drafts[d.ID] = d
}

func (s *Store) ActionDraft(id string) (ActionDraft, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	// Args — изменяемая map; наружу отдаём глубокую копию.
	raw, _ := json.Marshal(d)
	var out ActionDraft
	_ = json.Unmarshal(raw, &out)
	return out, ok
}

func (s *Store) Operation(key string) (Operation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.operations[key]
	v.Result = append(json.RawMessage(nil), v.Result...)
	return v, ok
}

func (s *Store) PutOperation(key string, v Operation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations[key] = v
}

func (s *Store) PendingDrafts(userID string) []ActionDraft {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []ActionDraft{}
	for _, d := range s.drafts {
		if d.UserID == userID && len(d.Result) == 0 {
			raw, _ := json.Marshal(d)
			var copy ActionDraft
			_ = json.Unmarshal(raw, &copy)
			out = append(out, copy)
		}
	}
	return out
}
