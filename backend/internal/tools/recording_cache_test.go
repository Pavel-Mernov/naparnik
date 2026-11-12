package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/worker4556/naparnik/backend/internal/cache"
)

// BR-36: после обновления не читаем старые записи кеша с URL встречи в тексте ответа.
func TestSearchIgnoresLegacyRecordingCache(t *testing.T) {
	previous := KBCache
	KBCache = cache.NewMemory()
	t.Cleanup(func() { KBCache = previous })
	c := ctxFor(t, newStore(t), "intern.sokolov")
	query := "Что такое витрина?"
	oldKey := strings.Replace(kbKey(c, "search", query), "kb:public-v2:", "kb:", 1)
	raw, _ := json.Marshal([]Hit{{PageID: "kb-03", Body: "https://vks.mock.local/rec/LEGACY"}})
	if err := KBCache.SetTTL(t.Context(), oldKey, string(raw), time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := Run(c, "search_kb", map[string]any{"query": query})
		raw, _ := json.Marshal(r)
		if !r.OK || strings.Contains(string(raw), "/rec/LEGACY") || !strings.Contains(string(raw), "kb-03") {
			t.Fatalf("старый кеш раскрывает URL или потерян ответ: %s", raw)
		}
	}
}
