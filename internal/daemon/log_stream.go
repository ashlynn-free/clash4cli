package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"rocket4cli/internal/logger"
)

type logStreamHub struct {
	nextID      atomic.Uint64
	mu          sync.Mutex
	subscribers map[uint64]chan logger.Entry
}

func newLogStreamHub() *logStreamHub {
	return &logStreamHub{subscribers: make(map[uint64]chan logger.Entry)}
}

func (h *logStreamHub) subscribe() (uint64, <-chan logger.Entry, func()) {
	id := h.nextID.Add(1)
	ch := make(chan logger.Entry, 64)
	h.mu.Lock()
	h.subscribers[id] = ch
	h.mu.Unlock()
	return id, ch, func() {
		h.mu.Lock()
		if existing, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(existing)
		}
		h.mu.Unlock()
	}
}

func (h *logStreamHub) publish(entry logger.Entry) {
	h.mu.Lock()
	for _, ch := range h.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	h.mu.Unlock()
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, ch, unsubscribe := s.logHub.subscribe()
	defer unsubscribe()

	send := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send("ready", map[string]any{"revision": s.revision.Load(), "api_version": APIVersion}) {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if !send("ping", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano)}) {
				return
			}
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if !send("log", map[string]any{
				"time":     entry.Time.Format(time.RFC3339Nano),
				"level":    entry.Level.String(),
				"message":  entry.Message,
				"revision": s.revision.Load(),
			}) {
				return
			}
		}
	}
}
