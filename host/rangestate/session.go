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

// maxSessionEvents bounds the pending-event buffer. Events are normally drained
// by each snapshot; the cap is the safety net for a session that nothing reads.
const maxSessionEvents = 256

// appendEvents queues events for the next snapshot, discarding the oldest once
// the buffer is full.
func (s *RangePluginSession) appendEvents(events []logicapi.PluginEvent) {
	if len(events) == 0 {
		return
	}
	s.Events = append(s.Events, events...)
	if overflow := len(s.Events) - maxSessionEvents; overflow > 0 {
		s.Events = append(s.Events[:0], s.Events[overflow:]...)
	}
}
