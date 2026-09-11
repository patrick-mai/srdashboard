package api

import (
	"encoding/json"
	"net/http"

	"srdashboard/state"
	"srdashboard/wettkampf"
)

// ServeWettkampf handles GET/PUT /api/wettkampf.
func (h *Handlers) ServeWettkampf(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_ = json.NewEncoder(w).Encode(h.wettkampfView())
	case http.MethodPut:
		if !h.checkControlToken(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if h.Wettkampf == nil {
			http.Error(w, "wettkampf not available", http.StatusServiceUnavailable)
			return
		}
		var patch wettkampf.Patch
		if !decodeJSONBody(w, r, &patch) {
			return
		}
		view := h.Wettkampf.Apply(patch)
		h.BroadcastWettkampf()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) wettkampfView() wettkampf.Snapshot {
	if h == nil || h.Wettkampf == nil {
		return wettkampf.Snapshot{Teams: []wettkampf.TeamView{}, Roster: []wettkampf.RosterView{}}
	}
	return h.Wettkampf.View()
}

// ObserveWettkampfShot upserts live lane scores into the competition store.
func (h *Handlers) ObserveWettkampfShot(snap state.RangeSnapshot) {
	if h == nil || h.Wettkampf == nil {
		return
	}
	h.Wettkampf.Observe(snap)
}

func (h *Handlers) BroadcastWettkampf() {
	if h == nil || h.Hub == nil {
		return
	}
	h.Hub.BroadcastAll(map[string]any{
		"type":      "wettkampf",
		"wettkampf": h.wettkampfView(),
	})
}
