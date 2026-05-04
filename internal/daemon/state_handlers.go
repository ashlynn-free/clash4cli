package daemon

import (
	"encoding/json"
	"net/http"

	"rocket4cli/internal/config"
)

func (s *Server) handleConfigState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg == nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "config is nil"})
		return
	}
	var incoming config.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid config payload"})
		return
	}
	s.configMu.Lock()
	*s.cfg = incoming
	s.configMu.Unlock()
	if err := s.cfg.Save(); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.bumpRevision()
	s.writeJSON(w, http.StatusOK, map[string]any{"saved": true, "revision": s.revision.Load()})
}
