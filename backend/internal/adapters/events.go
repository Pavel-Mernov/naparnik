package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/worker4556/naparnik/backend/internal/store"
)

// Экспорт журнала событий (BR-73) в аналитику Авито — Clickstream / Kafka. События уже без текстов вопросов,
// ответов и причин отсутствия (это гарантирует store.Emit), поэтому их можно отдавать в общую аналитику.
//
// Выгрузка асинхронная: буфер на BufferSize событий, отправка пачками раз в FlushEvery или при заполнении пачки.
// Сбой коллектора не влияет на работу Напарника: пачка повторяется со следующей, при переполнении буфера
// события отбрасываются со счётчиком (журнал в PostgreSQL остаётся источником правды).

// ClickEvent — формат события для коллектора.
type ClickEvent struct {
	EventID string            `json:"event_id"`
	TS      string            `json:"ts"`
	Source  string            `json:"source"`
	Type    string            `json:"event_type"`
	Actor   string            `json:"actor_id"`
	Object  string            `json:"object_id"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Writer — куда пишет экспортёр: HTTP-коллектор или поток JSONL.
type Writer interface {
	Write(ctx context.Context, batch []ClickEvent) error
}

type Exporter struct {
	W          Writer
	BatchSize  int
	FlushEvery time.Duration

	ch      chan ClickEvent
	mu      sync.Mutex
	sent    int
	dropped int
	failed  int
}

func NewExporter(w Writer, buffer int) *Exporter {
	return &Exporter{W: w, BatchSize: 100, FlushEvery: 2 * time.Second, ch: make(chan ClickEvent, buffer)}
}

// Sink — функция для store.EventSink: не блокирует транзакцию.
func (e *Exporter) Sink(evs []store.LogEvent) {
	for _, ev := range evs {
		select {
		case e.ch <- ClickEvent{EventID: ev.ID, TS: ev.At, Source: "naparnik", Type: ev.Type, Actor: ev.Actor, Object: ev.Object, Meta: ev.Meta}:
		default:
			e.mu.Lock()
			e.dropped++
			e.mu.Unlock()
		}
	}
}

// Run отправляет пачки до отмены контекста; перед выходом досылает накопленное.
func (e *Exporter) Run(ctx context.Context) {
	tick := time.NewTicker(e.FlushEvery)
	defer tick.Stop()
	var batch []ClickEvent
	flush := func() {
		if len(batch) == 0 {
			return
		}
		wctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := e.W.Write(wctx, batch)
		cancel()
		e.mu.Lock()
		if err != nil {
			e.failed++
			if e.failed == 1 || e.failed%50 == 0 {
				log.Printf("события: коллектор недоступен (%v) — повторю", err)
			}
			if len(batch) > 10*e.BatchSize { // долго лежит — не копим бесконечно
				e.dropped += len(batch) - e.BatchSize
				batch = batch[len(batch)-e.BatchSize:]
			}
			e.mu.Unlock()
			return
		}
		e.sent += len(batch)
		e.mu.Unlock()
		batch = nil
	}
	for {
		select {
		case ev := <-e.ch:
			batch = append(batch, ev)
			if len(batch) >= e.BatchSize {
				flush()
			}
		case <-tick.C:
			flush()
		case <-ctx.Done():
			for {
				select {
				case ev := <-e.ch:
					batch = append(batch, ev)
				default:
					flush()
					return
				}
			}
		}
	}
}

// Stats — сколько отправлено, отброшено и неудачных попыток (для /api/naparnik).
func (e *Exporter) Stats() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return map[string]int{"sent": e.sent, "dropped": e.dropped, "failed_batches": e.failed, "queued": len(e.ch)}
}

// HTTPCollector — POST пачки JSON в коллектор (Clickstream-шлюз или REST-прокси Kafka).
type HTTPCollector struct {
	URL, Token string
	Client     *http.Client
}

func (h HTTPCollector) Write(ctx context.Context, batch []ClickEvent) error {
	raw, _ := json.Marshal(map[string]any{"events": batch})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("коллектор: HTTP %d", resp.StatusCode)
	}
	return nil
}

// JSONLines — построчный JSON в поток (файл или stdout): для локальной проверки и сборщиков логов.
type JSONLines struct {
	mu  sync.Mutex
	Out io.Writer
}

func (j *JSONLines) Write(_ context.Context, batch []ClickEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	enc := json.NewEncoder(j.Out)
	for _, ev := range batch {
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
	return nil
}
