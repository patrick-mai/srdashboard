package api

import (
	"encoding/json"
	"log"
	"net/http"

	"srdashboard/config"
)

type runtimeRequest struct {
	InactiveRanges []int `json:"inactiveRanges"`
}

func (h *Handlers) runtimePath() string {
	return config.RuntimePath(h.ConfigPath)
}

func (h *Handlers) inactiveRangeList() []int {
	max := 0
	if h.Cfg != nil {
		max = h.cfgSnapshot().Ranges
	}
	h.runtimeMu.RLock()
	defer h.runtimeMu.RUnlock()
	if h.Runtime == nil {
		return []int{}
	}
	list := h.Runtime.InactiveList(max)
	if list == nil {
		return []int{}
	}
	return list
}

func (h *Handlers) isRangeInactive(rng int) bool {
	h.runtimeMu.RLock()
	defer h.runtimeMu.RUnlock()
	return h.Runtime != nil && h.Runtime.IsInactive(rng)
}

func (h *Handlers) pruneRuntime(max int) {
	h.runtimeMu.Lock()
	defer h.runtimeMu.Unlock()
	if h.Runtime == nil {
		h.Runtime = &config.Runtime{}
	}
	before := h.Runtime.InactiveRanges
	h.Runtime.Prune(max)
	if h.Runtime.InactiveRanges == before {
		return
	}
	if err := config.SaveRuntime(h.runtimePath(), h.Runtime); err != nil {
		log.Printf("runtime: prune save: %v", err)
	}
}

// AllowShot is the UDP membership gate: configured 1..N and not inactive on this instance.
func (h *Handlers) AllowShot(rng int) bool {
	if h == nil || rng < 1 {
		return false
	}
	n := h.cfgSnapshot().Ranges
	if n < 1 || rng > n {
		return false
	}
	return !h.isRangeInactive(rng)
}

// ServeRuntime handles GET/PUT /api/runtime — this process's inactive lanes.
func (h *Handlers) ServeRuntime(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runtimeRequest{InactiveRanges: h.inactiveRangeList()})
	case http.MethodPut:
		if !h.checkControlToken(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var req runtimeRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		h.applyInactiveRanges(req.InactiveRanges)
		if h.Hub != nil {
			h.Hub.BroadcastAll(map[string]any{"type": "config_changed", "config": h.configResponse()})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.configResponse())
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) applyInactiveRanges(nums []int) {
	max := h.cfgSnapshot().Ranges
	h.runtimeMu.Lock()
	if h.Runtime == nil {
		h.Runtime = &config.Runtime{}
	}
	old := make(map[int]bool)
	for _, n := range h.Runtime.InactiveList(max) {
		old[n] = true
	}
	h.Runtime.SetInactiveList(nums, max)
	newly := make([]int, 0)
	for _, n := range h.Runtime.InactiveList(max) {
		if !old[n] {
			newly = append(newly, n)
		}
	}
	list := h.Runtime.InactiveList(max)
	path := h.runtimePath()
	rt := *h.Runtime
	h.runtimeMu.Unlock()

	if err := config.SaveRuntime(path, &rt); err != nil {
		log.Printf("runtime: save: %v", err)
	}
	for _, n := range newly {
		if h.State != nil && h.State.ResetRange(n) {
			h.broadcastLiveRange(n)
		}
	}
	if h.PluginState != nil {
		h.PluginState.SetInactiveRanges(list)
	}
}
