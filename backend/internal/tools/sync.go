package tools

import (
	"context"
	"log"
	"time"

	"github.com/worker4556/naparnik/backend/internal/adapters"
	"github.com/worker4556/naparnik/backend/internal/store"
)

// SyncAbsence — решения по заявкам на отсутствие, принятые в настоящем сервисе. Для каждой заявки
// «на согласовании» спрашиваем сервис и, если решение есть, применяем его через Decide от имени
// согласующего: история, уведомление стажёру и событие в журнале — как при решении в интерфейсе.
// Возвращает, сколько заявок изменилось. Сбой сервиса не трогает заявки: следующий опрос повторит.
func SyncAbsence(ctx context.Context, s *store.Store, src adapters.AbsenceStatusSource) int {
	if err := s.Refresh(ctx); err != nil {
		return 0
	}
	changed := 0
	for _, r := range s.Requests("") {
		if r.Kind != "absence" || r.Status != "pending" {
			continue
		}
		st, err := src.Status(ctx, r.ID)
		if err != nil {
			log.Printf("сервис отсутствий: статус %s: %v", r.ID, err)
			continue
		}
		approver, ok := s.User(r.ApproverID)
		if !ok {
			continue
		}
		c := Ctx{Store: s, User: approver}
		var e *Error
		switch st.Status {
		case "approved":
			_, e = Decide(c, r.ID, "approve", "", st.Comment)
		case "rejected":
			reason, comment := st.ReasonCode, st.Comment
			if !knownReason(c, reason) { // код сервиса не из справочника — «Другое» с пояснением
				reason = "OTHER"
				if comment == "" {
					comment = "Отклонено в сервисе отсутствий"
				}
			}
			_, e = Decide(c, r.ID, "reject", reason, comment)
		default:
			continue
		}
		if e != nil {
			log.Printf("сервис отсутствий: решение по %s не применено: %s", r.ID, e.Message)
			continue
		}
		changed++
	}
	return changed
}

func knownReason(c Ctx, code string) bool {
	for _, x := range reasonCodes(c, "absence") {
		if x.Code == code {
			return true
		}
	}
	return false
}

// RunAbsenceSync — опрос сервиса отсутствий раз в interval, пока жив ctx.
func RunAbsenceSync(ctx context.Context, s *store.Store, src adapters.AbsenceStatusSource, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, cancel := context.WithTimeout(ctx, interval)
			if n := SyncAbsence(c, s, src); n > 0 {
				log.Printf("сервис отсутствий: обновлено решений — %d", n)
			}
			cancel()
		}
	}
}
