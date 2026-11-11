package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleSlotRoutes(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/slots/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	slotID, err := strconv.Atoi(parts[0])
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid slot id"})
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		state := s.slotMgr.GetSlotState(slotID)
		if state == nil {
			s.writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("slot %d is not running", slotID)})
			return
		}
		s.writeJSON(w, http.StatusOK, state)
		return
	}

	action := parts[1]
	switch action {
	case "start":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req StartSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid start request"})
			return
		}
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = "rule"
		}
		if err := s.slotMgr.StartSlotWithMode(slotID, req.SubID, req.Nodes, req.SelectedNode, mode); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.onRuntimeMutation()
		state := s.slotMgr.GetSlotState(slotID)
		s.writeJSON(w, http.StatusOK, state)
	case "stop":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := s.slotMgr.StopSlot(slotID); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.onRuntimeMutation()
		s.writeJSON(w, http.StatusOK, map[string]any{"stopped": true, "slot_id": slotID})
	case "mode":
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req SetModeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid mode request"})
			return
		}
		if err := s.slotMgr.SetSlotMode(slotID, req.Mode); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.onRuntimeMutation()
		state := s.slotMgr.GetSlotState(slotID)
		s.writeJSON(w, http.StatusOK, state)
	case "traffic":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		up, down, count, err := s.slotMgr.GetTrafficAndConnCountForSlot(slotID)
		if err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"upload_total":   up,
			"download_total": down,
			"conn_count":     count,
		})
	case "connections":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		snapshot, err := s.slotMgr.GetConnectionsForSlot(slotID)
		if err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, snapshot)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
