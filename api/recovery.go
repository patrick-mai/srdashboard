package api

import (
	"encoding/json"
	"io"
	"net/http"

	"srdashboard/recovery"
)

const recoveryMaxBody = 8 << 20 // 8 MiB for pasted session logs

type recoveryReplayRequest struct {
	Log   string `json:"log"`
	Clear bool   `json:"clear"`
}

type recoveryReplayResponse struct {
	Status  string `json:"status"`
	Lines   int    `json:"lines"`
	Applied int    `json:"applied"`
}

// RecoveryReplay handles POST /api/recovery/replay — parses pasted console or
// OpticScore JSON and applies shots through the validated UDP ingest path,
// including the active game plugin (random field events are not in the log).
func (h *Handlers) RecoveryReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, recoveryMaxBody)
	var req recoveryReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err == io.EOF {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if h.ReplayLog == nil {
		http.Error(w, "replay not available", http.StatusServiceUnavailable)
		return
	}
	if h.PluginState != nil {
		h.PluginState.BeginReplay()
		defer h.PluginState.EndReplay()
	}
	if req.Clear {
		h.State.ResetAllRanges()
		if h.PluginState != nil {
			if id := h.PluginState.ActivePluginID(); id != "" {
				_ = h.PluginState.Activate(id)
			}
		}
	}
	lines := recovery.ParseLog(req.Log)
	if h.State != nil {
		h.State.SeedMissingProgramLength(60)
	}
	if h.PluginState != nil {
		h.PluginState.SyncLiveReady()
	}
	applied := h.ReplayLog(lines)
	h.broadcastAllLiveRanges()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recoveryReplayResponse{
		Status:  "ok",
		Lines:   len(lines),
		Applied: applied,
	})
}
