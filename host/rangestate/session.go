package rangestate

import (
	"time"

	"srdashboard/host/logicapi"
)

const (
	PhaseIdle    = "idle"
	PhaseActive  = "active"
	PhaseRunning = "running" // alias kept for older clients; same as active
)

// RangePluginSession holds per-range plugin session state.
type RangePluginSession struct {
	RangeNum  int                    `json:"rangeNum"`
	PluginID  string                 `json:"pluginId"`
	Phase     string                 `json:"phase"`
	Config    map[string]any         `json:"config,omitempty"`
	State     logicapi.SessionState  `json:"-"`
	ViewModel map[string]any         `json:"viewModel,omitempty"`
	ShotCount int                    `json:"shotCount"`
	StartedAt time.Time              `json:"startedAt,omitempty"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Events    []logicapi.PluginEvent `json:"events,omitempty"`
}

// SessionSnapshot is API-safe copy with cleared ephemeral events after read.
type SessionSnapshot struct {
	RangeNum  int                    `json:"rangeNum"`
	PluginID  string                 `json:"pluginId"`
	Phase     string                 `json:"phase"`
	ViewModel map[string]any         `json:"viewModel,omitempty"`
	ShotCount int                    `json:"shotCount"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Events    []logicapi.PluginEvent `json:"events,omitempty"`
}

func (s *RangePluginSession) snapshot() SessionSnapshot {
	return SessionSnapshot{
		RangeNum:  s.RangeNum,
		PluginID:  s.PluginID,
		Phase:     s.Phase,
		ViewModel: s.ViewModel,
		ShotCount: s.ShotCount,
		UpdatedAt: s.UpdatedAt,
		Events:    append([]logicapi.PluginEvent(nil), s.Events...),
	}
}

func (s *RangePluginSession) clearEvents() {
	s.Events = nil
}
