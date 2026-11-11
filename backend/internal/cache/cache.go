// Package cache — быстрое временное состояние, общее для экземпляров backend (стек Авито: PostgreSQL + Redis).
// Источник правды — PostgreSQL; здесь только то, что живёт минуты: одноразовые коды привязки Telegram,
// лимит запросов к модели и блокировка «отправляет один экземпляр». Без REDIS_URL — память процесса.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type KV interface {
	// SetTTL кладёт значение с временем жизни.
	SetTTL(ctx context.Context, key, value string, ttl time.Duration) error
	// Get возвращает значение, если оно есть и не истекло (кэш поиска по базе).
	Get(ctx context.Context, key string) (string, bool, error)
	// Take возвращает значение и удаляет его (одноразовый код).
	Take(ctx context.Context, key string) (string, bool, error)
	// Hit увеличивает счётчик окна и возвращает новое значение (лимит запросов).
	Hit(ctx context.Context, key string, window time.Duration) (int64, error)
	// Lock берёт блокировку на ttl; false — её держит другой.
	Lock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Kind() string
}

// New — Redis по адресу (redis://host:6379/0) или память, если адрес пустой.
func New(url string) (KV, error) {
	if url == "" {
		return NewMemory(), nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &redisKV{c}, nil
}

type redisKV struct{ c *redis.Client }

func (r *redisKV) Kind() string { return "redis" }
func (r *redisKV) SetTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.c.Set(ctx, key, value, ttl).Err()
}
func (r *redisKV) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.c.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	return v, err == nil, err
}
func (r *redisKV) Take(ctx context.Context, key string) (string, bool, error) {
	v, err := r.c.GetDel(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	return v, err == nil, err
}
func (r *redisKV) Hit(ctx context.Context, key string, window time.Duration) (int64, error) {
	n, err := r.c.Incr(ctx, key).Result()
	if err == nil && n == 1 {
		err = r.c.Expire(ctx, key, window).Err()
	}
	return n, err
}
func (r *redisKV) Lock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return r.c.SetNX(ctx, key, "1", ttl).Result()
}

type item struct {
	v   string
	exp time.Time
	n   int64
}

type memory struct {
	mu  sync.Mutex
	m   map[string]item
	now func() time.Time
}

func NewMemory() KV { return &memory{m: map[string]item{}, now: time.Now} }

func (m *memory) Kind() string { return "memory" }

func (m *memory) get(key string) (item, bool) {
	it, ok := m.m[key]
	if ok && !it.exp.IsZero() && !m.now().Before(it.exp) {
		delete(m.m, key)
		return item{}, false
	}
	return it, ok
}

func (m *memory) SetTTL(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = item{v: value, exp: m.now().Add(ttl)}
	return nil
}
func (m *memory) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.get(key)
	return it.v, ok, nil
}
func (m *memory) Take(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.get(key)
	delete(m.m, key)
	return it.v, ok, nil
}
func (m *memory) Hit(_ context.Context, key string, window time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.get(key)
	if !ok {
		it = item{exp: m.now().Add(window)}
	}
	it.n++
	m.m[key] = it
	return it.n, nil
}
func (m *memory) Lock(_ context.Context, key string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.get(key); ok {
		return false, nil
	}
	m.m[key] = item{v: "1", exp: m.now().Add(ttl)}
	return true, nil
}
