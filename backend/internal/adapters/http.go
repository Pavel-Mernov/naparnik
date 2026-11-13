package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPAbsence — клиент настоящего сервиса отсутствий по REST (контракт — docs/03-requirements/11-INTEGRATION.md):
//
//	GET  {base}/catalog   → AbsenceCatalog
//	POST {base}/requests  ← AbsenceSubmit, заголовок Idempotency-Key → Receipt
//	GET  {base}/requests/{id} → AbsenceStatus (опрос решения согласующего)
//
// Авторизация — сервисный токен в Authorization: Bearer (ABSENCE_SERVICE_TOKEN). Любой сетевой сбой,
// 5xx или 429 → ErrUnavailable: черновик у стажёра сохраняется, «Повторить» безопасно благодаря ключу.
// Справочник кэшируется на CatalogTTL, чтобы объяснение порядка не ходило в сервис на каждый вопрос.
type HTTPAbsence struct {
	Base, Token string
	Client      *http.Client
	CatalogTTL  time.Duration

	mu      sync.Mutex
	catalog *AbsenceCatalog
	fetched time.Time
}

func NewHTTPAbsence(base, token string) *HTTPAbsence {
	return &HTTPAbsence{Base: base, Token: token, Client: &http.Client{Timeout: 5 * time.Second}, CatalogTTL: 10 * time.Minute}
}

func (h *HTTPAbsence) do(ctx context.Context, method, path, key string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.Base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return ErrUnavailable
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("сервис отсутствий: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (h *HTTPAbsence) Catalog(ctx context.Context) (AbsenceCatalog, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.catalog != nil && time.Since(h.fetched) < h.CatalogTTL {
		return *h.catalog, nil
	}
	var c AbsenceCatalog
	if err := h.do(ctx, http.MethodGet, "/catalog", "", nil, &c); err != nil {
		if h.catalog != nil { // сервис моргнул — отвечаем последним известным справочником
			return *h.catalog, nil
		}
		return AbsenceCatalog{}, err
	}
	h.catalog, h.fetched = &c, time.Now()
	return c, nil
}

func (h *HTTPAbsence) Submit(ctx context.Context, r AbsenceSubmit) (Receipt, error) {
	var out Receipt
	if err := h.do(ctx, http.MethodPost, "/requests", r.Key, r, &out); err != nil {
		return Receipt{}, err
	}
	if out.ID == "" {
		return Receipt{}, fmt.Errorf("сервис отсутствий не вернул номер заявки")
	}
	if out.Status == "" {
		out.Status = "pending"
	}
	return out, nil
}

// Status — статус заявки в сервисе: решение согласующего принимается там.
func (h *HTTPAbsence) Status(ctx context.Context, id string) (AbsenceStatus, error) {
	var out AbsenceStatus
	if err := h.do(ctx, http.MethodGet, "/requests/"+url.PathEscape(id), "", nil, &out); err != nil {
		return AbsenceStatus{}, err
	}
	if out.ID == "" {
		out.ID = id
	}
	return out, nil
}
