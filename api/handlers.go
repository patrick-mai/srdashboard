package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"srdashboard/config"
	"srdashboard/host/loader"
	"srdashboard/host/rangestate"
	"srdashboard/state"
)

// Handlers provides HTTP handlers for the API
type Handlers struct {
	State       *state.LiveState
	Cfg         *config.Config
	ConfigPath  string
	Plugins     *loader.Manager
	PluginState *rangestate.Manager
	Hub         *Hub
}

// LiveResponse is the JSON response for GET /api/live
type LiveResponse struct {
	Ranges []RangeResponse `json:"ranges"`
}

// RangeResponse is the per-range data for the API
type RangeResponse struct {
	RangeNum         int          `json:"rangeNum"`
	ShooterName      string       `json:"shooterName"`
	ClubName         string       `json:"clubName"`
	Discipline       string       `json:"discipline"`
	IsWarmup         bool         `json:"isWarmup"`
	Shots            []state.Shot `json:"shots"`
	ShotNumber       int          `json:"shotNumber"`
	CurrentValue     float64      `json:"currentValue"`
	CurrentTeiler    float64      `json:"currentTeiler"`
	BestTeiler       float64      `json:"bestTeiler"`
	BestTeilerShot   int          `json:"bestTeilerShot"`
	OverallSumInt    int          `json:"overallSumInt"`
	OverallSumDec    float64      `json:"overallSumDecimal"`
	PredictionInt    int          `json:"predictionInt"`
	PredictionDec    float64      `json:"predictionDecimal"`
	SeriesSumsInt    []int        `json:"seriesSumsInt"`
	SeriesSums       []float64    `json:"seriesSums"`
	Last10Values     []float64    `json:"last10Values"`
	TotalShotsToFire int          `json:"totalShotsToFire"`
}

// Live returns the current live state for all ranges. Uses a thread-safe snapshot to avoid data races with UDP.
// POST /api/live/reset?range=N clears one range to defaults.
// PUT /api/live with LiveResponse body replaces the listed ranges (used to restore after restart).
func (h *Handlers) Live(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.liveGet(w, r)
	case http.MethodPut:
		h.liveReplace(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// LiveReset handles POST /api/live/reset?range=N.
func (h *Handlers) LiveReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("range"))
	if err != nil || n < 1 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}
	if !h.State.ResetRange(n) {
		http.Error(w, "range not found", http.StatusNotFound)
		return
	}
	h.broadcastLiveRange(n)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "range": n})
}

func (h *Handlers) liveGet(w http.ResponseWriter, r *http.Request) {
	if rng := r.URL.Query().Get("range"); rng != "" {
		n, err := strconv.Atoi(rng)
		if err != nil {
			http.Error(w, "invalid range", http.StatusBadRequest)
			return
		}
		snap := h.State.Snapshot()
		for _, rs := range snap {
			if rs.RangeNum == n {
				resp := LiveResponse{Ranges: []RangeResponse{rangeSnapshotToResponse(rs)}}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		}
		http.Error(w, "range not found", http.StatusNotFound)
		return
	}
	snap := h.State.Snapshot()
	resp := LiveResponse{
		Ranges: make([]RangeResponse, 0, len(snap)),
	}
	for _, s := range snap {
		resp.Ranges = append(resp.Ranges, rangeSnapshotToResponse(s))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) liveReplace(w http.ResponseWriter, r *http.Request) {
	var req LiveResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	for _, rr := range req.Ranges {
		if !h.State.ReplaceRange(responseToSnapshot(rr)) {
			http.Error(w, "range not found", http.StatusNotFound)
			return
		}
		h.broadcastLiveRange(rr.RangeNum)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "count": len(req.Ranges)})
}

func (h *Handlers) broadcastLiveRange(rng int) {
	if h.Hub == nil {
		return
	}
	for _, rs := range h.State.Snapshot() {
		if rs.RangeNum == rng {
			h.Hub.BroadcastRange(rng, map[string]any{
				"type":  "live",
				"range": rs,
			})
			return
		}
	}
}

func rangeSnapshotToResponse(s state.RangeSnapshot) RangeResponse {
	return RangeResponse{
		RangeNum:         s.RangeNum,
		ShooterName:      s.ShooterName,
		ClubName:         s.ClubName,
		Discipline:       s.Discipline,
		IsWarmup:         s.IsWarmup,
		Shots:            s.Shots,
		ShotNumber:       s.ShotNumber,
		CurrentValue:     s.CurrentValue,
		CurrentTeiler:    s.CurrentTeiler,
		BestTeiler:       s.BestTeiler,
		BestTeilerShot:   s.BestTeilerShot,
		OverallSumInt:    s.OverallSumInt,
		OverallSumDec:    s.OverallSumDec,
		PredictionInt:    s.PredictionInt,
		PredictionDec:    s.PredictionDec,
		SeriesSumsInt:    s.SeriesSumsInt,
		SeriesSums:       s.SeriesSums,
		Last10Values:     s.Last10Values,
		TotalShotsToFire: s.TotalShotsToFire,
	}
}

func responseToSnapshot(r RangeResponse) state.RangeSnapshot {
	return state.RangeSnapshot{
		RangeNum:         r.RangeNum,
		ShooterName:      r.ShooterName,
		ClubName:         r.ClubName,
		Discipline:       r.Discipline,
		IsWarmup:         r.IsWarmup,
		Shots:            r.Shots,
		ShotNumber:       r.ShotNumber,
		CurrentValue:     r.CurrentValue,
		CurrentTeiler:    r.CurrentTeiler,
		BestTeiler:       r.BestTeiler,
		BestTeilerShot:   r.BestTeilerShot,
		OverallSumInt:    r.OverallSumInt,
		OverallSumDec:    r.OverallSumDec,
		PredictionInt:    r.PredictionInt,
		PredictionDec:    r.PredictionDec,
		SeriesSumsInt:    r.SeriesSumsInt,
		SeriesSums:       r.SeriesSums,
		Last10Values:     r.Last10Values,
		TotalShotsToFire: r.TotalShotsToFire,
	}
}
