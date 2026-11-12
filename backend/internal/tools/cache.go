package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	syncatomic "sync/atomic"
	"time"

	"github.com/worker4556/naparnik/backend/internal/cache"
)

// Кэш поиска по базе и подбора эксперта (Redis; без него — память процесса).
// Это самые тяжёлые вызовы агента: каждый раз заново разбираются все страницы базы.
// Ключ = пользователь + версия базы + запрос. Версия меняется при любой публикации или правке страницы,
// поэтому кэш не отдаёт устаревший ответ, а ответ не уходит человеку с другими правами (BR-40).

// KBCache задаётся при сборке приложения; nil — без кэша (тесты, CLI).
var KBCache cache.KV

const kbCacheTTL = 10 * time.Minute

var cacheHits, cacheMisses syncatomic.Int64

// CacheStats — сколько раз кэш сработал (для /api/health).
func CacheStats() (hits, misses int64) { return cacheHits.Load(), cacheMisses.Load() }

// kbVersion — отпечаток базы, видимой пользователю: id, статус и дата обновления страниц.
func kbVersion(c Ctx) string {
	h := sha256.New()
	for _, p := range readablePages(c) {
		h.Write([]byte(p.ID + "|" + p.Status + "|" + p.UpdatedAt + "|" + p.Title + "\n"))
		h.Write([]byte{byte(len(p.Body)), byte(len(p.Body) >> 8)})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// kbKey — ключ по основам слов, а не по тексту: поиск и подбор эксперта зависят только от них,
// поэтому «Что такое витрина?» и «витрина что это» — один и тот же ключ.
func kbKey(c Ctx, kind, query string) string {
	stems := make([]string, 0)
	for st := range Stems(query) {
		stems = append(stems, st)
	}
	sort.Strings(stems)
	q := strings.Join(stems, " ")
	sum := sha256.Sum256([]byte(q))
	// Новый namespace не читает ответы прежней версии с URL записей в тексте.
	return "kb:public-v2:" + kind + ":" + c.User.ID + ":" + kbVersion(c) + ":" + hex.EncodeToString(sum[:8])
}

// cached достаёт значение из кэша или считает его и кладёт. Сбой кэша не ломает ответ — просто считаем.
func cached[T any](c Ctx, kind, query string, compute func() T) T {
	if KBCache == nil {
		return compute()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	key := kbKey(c, kind, query)
	if raw, ok, err := KBCache.Get(ctx, key); err == nil && ok {
		var v T
		if json.Unmarshal([]byte(raw), &v) == nil {
			cacheHits.Add(1)
			return v
		}
	}
	cacheMisses.Add(1)
	v := compute()
	if raw, err := json.Marshal(v); err == nil {
		_ = KBCache.SetTTL(ctx, key, string(raw), kbCacheTTL)
	}
	return v
}

type expertPick struct {
	ID         string  `json:"id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}
