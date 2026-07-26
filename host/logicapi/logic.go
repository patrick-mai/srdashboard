package logicapi

import (
	"encoding/json"

	"srdashboard/state"
)

const HostVersion = "1.0.0"

// SessionState is opaque JSON-serializable plugin session state owned by the server.
type SessionState json.RawMessage

// PluginEvent is a transient event for UI animations.
type PluginEvent struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// Logic is the host-side contract for WASM (or in-process) plugin scoring logic.
// Display plugins have no Logic; the host synthesizes view models from live range state.
type Logic interface {
	ID() string
	Label() string
	Version() string
	DefaultConfig() map[string]any
	ConfigSchema() map[string]any
	Init(cfg map[string]any) (SessionState, error)
	OnShot(sess SessionState, rangeNum int, shot state.Shot, shotIndex int) (SessionState, []PluginEvent, error)
	ViewModel(sess SessionState, rangeNum int) (map[string]any, error)
}
