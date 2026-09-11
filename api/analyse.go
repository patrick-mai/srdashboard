package api

import (
	"encoding/json"
	"net/http"
)

// AnalyseResults lists or returns one session result.
// GET /api/analyse/results
// GET /api/analyse/results?id=live-1 | id=12
func (h *Handlers) AnalyseResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.State == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.URL.Query().Get("id")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	if id == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": h.State.SessionResults(),
		})
		return
	}
	info, snap, ok := h.State.SessionResult(id)
	if !ok {
		http.Error(w, "result not found", http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         info.ID,
		"live":       info.Live,
		"archivedAt": info.ArchivedAt,
		"range":      snap,
	})
}
