package api

import (
	"encoding/json"
	"net/http"
)

func (h *Handlers) PluginControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Action string         `json:"action"`
		Type   string         `json:"type"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Action == "" {
		http.Error(w, "action required", http.StatusBadRequest)
		return
	}
	params := req.Params
	if params == nil {
		params = map[string]any{}
	}
	if req.Type != "" {
		params["type"] = req.Type
	}
	if h.PluginState == nil {
		http.Error(w, "plugin state unavailable", http.StatusInternalServerError)
		return
	}
	if err := h.PluginState.Control(req.Action, params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "action": req.Action})
}
