package rangestate

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

type Broadcaster interface {
	BroadcastRange(rangeNum int, payload any)
	BroadcastAll(payload any)
}

// LiveSource provides live range snapshots for display-plugin view models.
type LiveSource interface {
	Snapshot() []state.RangeSnapshot
}

type Manager struct {
	mu          sync.RWMutex
	sessions    map[int]*RangePluginSession
	plugins     *loader.Manager
	numRanges   int
	activeID    string
	broadcast   Broadcaster
	live        LiveSource
}

func NewManager(numRanges int, pm *loader.Manager, activePluginID string) *Manager {
	s := make(map[int]*RangePluginSession)
	for i := 1; i <= numRanges; i++ {
		s[i] = &RangePluginSession{RangeNum: i, Phase: PhaseIdle}
	}
	return &Manager{
		sessions:  s,
		plugins:   pm,
		numRanges: numRanges,
		activeID:  activePluginID,
	}
}

func (m *Manager) SetBroadcaster(b Broadcaster) { m.broadcast = b }
func (m *Manager) SetLiveSource(live LiveSource) { m.live = live }

func (m *Manager) ActivePluginID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeID
}

// Activate binds the given plugin as the site-active plugin on all ranges.
func (m *Manager) Activate(pluginID string) error {
	ap, err := m.plugins.Get(pluginID)
	if err != nil {
		return fmt.Errorf("plugin %q not loaded: %w", pluginID, err)
	}
	if ap.Manifest.Mode == loader.ModeShared {
		return fmt.Errorf("shared-mode plugins are not supported in v1")
	}

	cfg := m.plugins.MergedConfig(pluginID)
	if cfg == nil {
		cfg = map[string]any{}
	}
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 1; i <= m.numRanges; i++ {
		var sess logicapi.SessionState
		var vm map[string]any
		if ap.Logic != nil {
			sess, err = ap.Logic.Init(cfg)
			if err != nil {
				return err
			}
			vm, err = ap.Logic.ViewModel(sess, i)
			if err != nil {
				return err
			}
		} else {
			vm = m.displayViewModelLocked(i, ap)
		}
		m.sessions[i] = &RangePluginSession{
			RangeNum:  i,
			PluginID:  pluginID,
			Phase:     PhaseActive,
			Config:    cfg,
			State:     sess,
			ViewModel: vm,
			ShotCount: 0,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	m.activeID = pluginID
	m.notifyLocked()
	return nil
}

// EnsureActive starts the configured active plugin if sessions are idle.
func (m *Manager) EnsureActive() error {
	m.mu.RLock()
	id := m.activeID
	m.mu.RUnlock()
	if id == "" {
		id = "classic-range"
	}
	return m.Activate(id)
}

func (m *Manager) displayViewModelLocked(rangeNum int, ap *loader.ActivePlugin) map[string]any {
	vm := map[string]any{
		"rangeNum": rangeNum,
		"pluginId": ap.Manifest.ID,
		"kind":     ap.Manifest.Kind,
		"mode":     ap.Manifest.Mode,
		"label":    ap.Manifest.Label,
	}
	if m.live != nil {
		for _, rs := range m.live.Snapshot() {
			if rs.RangeNum == rangeNum {
				vm["range"] = rs
				break
			}
		}
	}
	return vm
}

func (m *Manager) OnShot(rangeNum int, shot state.Shot, shotIndex int) {
	m.mu.Lock()
	s, ok := m.sessions[rangeNum]
	if !ok || s.PluginID == "" || s.Phase == PhaseIdle {
		m.mu.Unlock()
		return
	}
	pluginID := s.PluginID
	m.mu.Unlock()

	ap, err := m.plugins.Get(pluginID)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok = m.sessions[rangeNum]
	if !ok || s.PluginID != pluginID {
		return
	}

	if ap.Logic != nil {
		newState, events, err := ap.Logic.OnShot(s.State, rangeNum, shot, shotIndex)
		if err != nil {
			return
		}
		s.State = newState
		s.ShotCount++
		vm, _ := ap.Logic.ViewModel(s.State, rangeNum)
		s.ViewModel = vm
		s.Events = append(s.Events, events...)
		s.UpdatedAt = time.Now()
		// Logic plugins need session/events pushed; display plugins paint from live WS only.
		m.notifyRangeLocked(rangeNum)
		return
	}
	s.ShotCount++
	s.ViewModel = m.displayViewModelLocked(rangeNum, ap)
	s.UpdatedAt = time.Now()
	// Skip plugin_session broadcast: doubles WS traffic and stalled the UDP loop
	// when any browser was slow. Live messages from main.go are enough for classic-range.
}

// RefreshDisplayViewModels rebuilds display view models from live state (e.g. after live poll).
func (m *Manager) RefreshDisplayViewModels() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ap, err := m.plugins.Get(m.activeID)
	if err != nil || ap.Logic != nil {
		return
	}
	for i := 1; i <= m.numRanges; i++ {
		s := m.sessions[i]
		if s == nil || s.PluginID != m.activeID {
			continue
		}
		s.ViewModel = m.displayViewModelLocked(i, ap)
		s.UpdatedAt = time.Now()
	}
}

func (m *Manager) SnapshotAll() ([]SessionSnapshot, ActiveSnapshot) {
	out := make([]SessionSnapshot, 0, m.numRanges)
	for i := 1; i <= m.numRanges; i++ {
		out = append(out, m.snapshotRangeInternal(i))
	}
	return out, m.activeSnapshot()
}

func (m *Manager) SnapshotRange(rangeNum int) SessionSnapshot {
	return m.snapshotRangeInternal(rangeNum)
}

func (m *Manager) snapshotRangeInternal(rangeNum int) SessionSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[rangeNum]
	if !ok {
		return SessionSnapshot{RangeNum: rangeNum, Phase: PhaseIdle}
	}
	// Refresh display VM from latest live data when reading
	if ap, err := m.plugins.Get(s.PluginID); err == nil && ap.Logic == nil {
		s.ViewModel = m.displayViewModelLocked(rangeNum, ap)
	}
	snap := s.snapshot()
	if snap.ViewModel != nil {
		cp := make(map[string]any, len(snap.ViewModel)+1)
		for k, v := range snap.ViewModel {
			cp[k] = v
		}
		cp["rangeNum"] = rangeNum
		snap.ViewModel = cp
	}
	s.clearEvents()
	return snap
}

// ActiveSnapshot describes the site-active plugin (replaces old match snapshot).
type ActiveSnapshot struct {
	Active   bool   `json:"active"`
	PluginID string `json:"pluginId,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Label    string `json:"label,omitempty"`
}

func (m *Manager) activeSnapshot() ActiveSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeID == "" {
		return ActiveSnapshot{Active: false}
	}
	snap := ActiveSnapshot{Active: true, PluginID: m.activeID}
	if ap, err := m.plugins.Get(m.activeID); err == nil {
		snap.Kind = ap.Manifest.Kind
		snap.Label = ap.Manifest.Label
	}
	return snap
}

func (m *Manager) Notify() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifyLocked()
}

func (m *Manager) notifyLocked() {
	if m.broadcast == nil {
		return
	}
	active := m.activeSnapshotUnlocked()
	m.broadcast.BroadcastAll(map[string]any{"type": "active_plugin", "active": active})
	// Back-compat alias for older frontends
	m.broadcast.BroadcastAll(map[string]any{
		"type":  "match",
		"match": map[string]any{"active": active.Active, "pluginId": active.PluginID},
	})
	for i := 1; i <= m.numRanges; i++ {
		m.notifyRangeLocked(i)
	}
}

func (m *Manager) activeSnapshotUnlocked() ActiveSnapshot {
	if m.activeID == "" {
		return ActiveSnapshot{Active: false}
	}
	snap := ActiveSnapshot{Active: true, PluginID: m.activeID}
	if ap, err := m.plugins.Get(m.activeID); err == nil {
		snap.Kind = ap.Manifest.Kind
		snap.Label = ap.Manifest.Label
	}
	return snap
}

func (m *Manager) notifyRangeLocked(rangeNum int) {
	if m.broadcast == nil {
		return
	}
	s, ok := m.sessions[rangeNum]
	if !ok {
		return
	}
	snap := s.snapshot()
	if snap.ViewModel != nil {
		cp := make(map[string]any, len(snap.ViewModel)+1)
		for k, v := range snap.ViewModel {
			cp[k] = v
		}
		cp["rangeNum"] = rangeNum
		snap.ViewModel = cp
	}
	payload, _ := json.Marshal(snap) // ensure JSON-serializable
	_ = payload
	m.broadcast.BroadcastRange(rangeNum, map[string]any{"type": "plugin_session", "session": snap})
}
