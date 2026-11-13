package app

import (
	"testing"
	"time"
)

// Опрос решений включается только с настоящим сервисом отсутствий; интервал — ABSENCE_SYNC_SEC.
func TestAbsenceSyncConfig(t *testing.T) {
	t.Setenv("ABSENCE_SERVICE_URL", "")
	if absenceSource(absenceFromEnv()) != nil {
		t.Fatal("без ABSENCE_SERVICE_URL опрашивать нечего")
	}
	t.Setenv("ABSENCE_SERVICE_URL", "http://absence.example/")
	h := absenceFromEnv()
	if h == nil || h.Base != "http://absence.example" || absenceSource(h) == nil {
		t.Fatalf("клиент сервиса: %+v", h)
	}
	t.Setenv("ABSENCE_SYNC_SEC", "")
	if syncInterval() != time.Minute {
		t.Fatal("по умолчанию — раз в минуту")
	}
	t.Setenv("ABSENCE_SYNC_SEC", "15")
	if syncInterval() != 15*time.Second {
		t.Fatal("ABSENCE_SYNC_SEC")
	}
	t.Setenv("ABSENCE_SYNC_SEC", "-3")
	if syncInterval() != time.Minute {
		t.Fatal("некорректное значение — по умолчанию")
	}
}
