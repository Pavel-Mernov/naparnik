package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestMemory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	m := &memory{m: map[string]item{}, now: func() time.Time { return now }}
	if m.Kind() != "memory" {
		t.Fatal(m.Kind())
	}
	_ = m.SetTTL(ctx, "code", "u-1", time.Minute)
	if v, ok, _ := m.Take(ctx, "code"); !ok || v != "u-1" {
		t.Fatal("одноразовый код")
	}
	if _, ok, _ := m.Take(ctx, "code"); ok {
		t.Fatal("код используется один раз")
	}
	_ = m.SetTTL(ctx, "old", "x", time.Minute)
	now = now.Add(2 * time.Minute)
	if _, ok, _ := m.Take(ctx, "old"); ok {
		t.Fatal("истёкший код")
	}
	for i := 1; i <= 3; i++ {
		if n, _ := m.Hit(ctx, "rl", time.Minute); n != int64(i) {
			t.Fatal("счётчик")
		}
	}
	now = now.Add(2 * time.Minute)
	if n, _ := m.Hit(ctx, "rl", time.Minute); n != 1 {
		t.Fatal("окно сбрасывается")
	}
	if ok, _ := m.Lock(ctx, "l", time.Minute); !ok {
		t.Fatal("блокировка")
	}
	if ok, _ := m.Lock(ctx, "l", time.Minute); ok {
		t.Fatal("занято")
	}
	if kv, err := New(""); err != nil || kv.Kind() != "memory" {
		t.Fatal("без адреса — память")
	}
	if _, err := New("::bad::"); err == nil {
		t.Fatal("кривой адрес")
	}
	if _, err := New("redis://127.0.0.1:1/0"); err == nil {
		t.Fatal("недоступный Redis — ошибка запуска")
	}
}

// Та же логика на настоящем протоколе Redis (miniredis — Redis в памяти для тестов).
func TestRedis(t *testing.T) {
	srv := miniredis.RunT(t)
	kv, err := New("redis://" + srv.Addr() + "/0")
	if err != nil || kv.Kind() != "redis" {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = kv.SetTTL(ctx, "code", "u-1", time.Minute)
	if v, ok, _ := kv.Take(ctx, "code"); !ok || v != "u-1" {
		t.Fatal("код")
	}
	if _, ok, err := kv.Take(ctx, "code"); ok || err != nil {
		t.Fatal("код один раз")
	}
	_, _ = kv.Hit(ctx, "rl", time.Minute)
	if n, _ := kv.Hit(ctx, "rl", time.Minute); n != 2 {
		t.Fatal("счётчик")
	}
	srv.FastForward(2 * time.Minute)
	if n, _ := kv.Hit(ctx, "rl", time.Minute); n != 1 {
		t.Fatal("окно")
	}
	if ok, _ := kv.Lock(ctx, "l", time.Minute); !ok {
		t.Fatal("блокировка")
	}
	if ok, _ := kv.Lock(ctx, "l", time.Minute); ok {
		t.Fatal("занято")
	}
}
