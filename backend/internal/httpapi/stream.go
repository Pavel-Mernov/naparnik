package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/worker4556/naparnik/backend/internal/mockdata"
)

// Живые обновления (SSE): раз в пару секунд сервер сверяет отпечаток данных пользователя и, если что-то
// изменилось (ответ эксперта, решение по заявке, новое уведомление), шлёт событие «changed» — веб
// перечитывает экран. Отпечаток берётся из общего хранилища, поэтому работает и при нескольких
// экземплярах backend. Если поток оборвался, веб продолжает опрос раз в 30 секунд.
var (
	streamTick = 2 * time.Second
	streamPing = 25 * time.Second
)

func (s *Server) stream(w http.ResponseWriter, r *http.Request, u *mockdata.User) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx не копит поток
	w.WriteHeader(http.StatusOK)
	last := s.Store.ChangeMark(u.ID)
	fmt.Fprintf(w, "retry: 5000\nevent: ready\ndata: %s\n\n", last)
	if rc.Flush() != nil {
		return
	}
	tick, ping := time.NewTicker(streamTick), time.NewTicker(streamPing)
	defer tick.Stop()
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case <-tick.C:
			if s.Store.Refresh(r.Context()) != nil {
				continue
			}
			mark := s.Store.ChangeMark(u.ID)
			if mark == last {
				continue
			}
			last = mark
			fmt.Fprintf(w, "event: changed\ndata: %s\n\n", mark)
		}
		if rc.Flush() != nil {
			return
		}
	}
}
