package logicapi

import (
	"encoding/json"
	"time"

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

// LiveRangeInfo carries live UDP-derived fields into game logic.
type LiveRangeInfo struct {
	IsWarmup         bool
	TotalShotsToFire int
	Discipline       string
	ShooterName      string
}

// ShotContext enriches OnShot for shared/builtin games.
type ShotContext struct {
	RangeNum  int
	Shot      state.Shot
	ShotIndex int
	Live      LiveRangeInfo
	NumRanges int
	Now       time.Time
}

// Logic is the host-side contract for WASM (or in-process) plugin scoring logic.
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

// ExtendedLogic is optional for builtin shared games (tick + control + live-aware shots).
type ExtendedLogic interface {
	Logic
	OnShotCtx(sess SessionState, ctx ShotContext) (SessionState, []PluginEvent, error)
	Tick(sess SessionState, now time.Time) (SessionState, []PluginEvent, bool, error)
	Control(sess SessionState, action string, params map[string]any) (SessionState, []PluginEvent, error)
}
