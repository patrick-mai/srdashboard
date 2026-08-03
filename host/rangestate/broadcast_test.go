package rangestate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "srdashboard/host/games/f1race"
	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

// recordingBroadcaster captures what the manager pushes to WebSocket clients.
type recordingBroadcaster struct {
	mu       sync.Mutex
	toRange  []map[string]any
	toAll    []map[string]any
	sessions []SessionSnapshot
}

func (b *recordingBroadcaster) BroadcastRange(rangeNum int, payload any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, _ := payload.(map[string]any)
	b.toRange = append(b.toRange, m)
	b.record(m)
}

func (b *recordingBroadcaster) BroadcastAll(payload any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, _ := payload.(map[string]any)
	b.toAll = append(b.toAll, m)
	b.record(m)
}

func (b *recordingBroadcaster) record(m map[string]any) {
	if m == nil || m["type"] != "plugin_session" {
		return
	}
	if s, ok := m["session"].(SessionSnapshot); ok {
		b.sessions = append(b.sessions, s)
	}
}

func (b *recordingBroadcaster) sessionMessages() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sessions)
}

func (b *recordingBroadcaster) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.toRange = nil
	b.toAll = nil
	b.sessions = nil
}

func newSharedManager(t *testing.T, ranges int) (*Manager, *recordingBroadcaster) {
	t.Helper()
	pluginsRoot := filepath.Join(t.TempDir(), "plugins")
	src := filepath.Join("..", "..", "plugins", "f1-race")
	if _, err := os.Stat(src); err != nil {
		t.Skip("f1-race not present")
	}
	if err := copyDir(src, filepath.Join(pluginsRoot, "f1-race")); err != nil {
		t.Fatal(err)
	}
	pm := loader.NewManager(pluginsRoot)
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(ranges, pm, "f1-race")
	m.SetLiveSource(state.NewLiveState(ranges))
	b := &recordingBroadcaster{}
	m.SetBroadcaster(b)
	if err := m.Activate("f1-race"); err != nil {
		t.Fatal(err)
	}
	return m, b
}

func startRace(t *testing.T, m *Manager, ranges int) {
	t.Helper()
	live := map[string]any{}
	for i := 1; i <= ranges; i++ {
		live[itoa(i)] = map[string]any{"totalShotsToFire": 40, "isWarmup": false}
	}
	if err := m.Control("start", map[string]any{"live": live}); err != nil {
		t.Fatal(err)
	}
}

// A shared-mode update reaches range-filtered clients through BroadcastAll
	// already, so sending it once (not once per range) is enough.
	func TestSharedModeBroadcastsSessionOnce(t *testing.T) {
	m, b := newSharedManager(t, 2)
	startRace(t, m, 2)
	b.reset()

	m.OnShot(1, state.Shot{DecValue: 10.2, FullValue: 10, ReceivedAt: time.Now()}, 1)

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, msg := range b.toRange {
		if msg != nil && msg["type"] == "plugin_session" {
			t.Fatal("shared mode sent plugin_session via BroadcastRange as well as BroadcastAll")
		}
	}
	if len(b.sessions) != 1 {
		t.Fatalf("got %d plugin_session messages for shared race, want 1", len(b.sessions))
	}
}

func TestEventsClearedAfterBroadcast(t *testing.T) {
	m, b := newSharedManager(t, 2)
	startRace(t, m, 2)
	b.reset()

	m.mu.Lock()
	m.sessions[1].appendEvents([]logicapi.PluginEvent{{Type: "overtake"}, {Type: "pit"}})
	m.notifyRangeLocked(1)
	pending := len(m.sessions[1].Events)
	m.mu.Unlock()

	if pending != 0 {
		t.Fatalf("%d events still queued after broadcast; they would replay in every later update", pending)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	var delivered int
	for _, s := range b.sessions {
		delivered += len(s.Events)
	}
	if delivered != 2 {
		t.Fatalf("broadcast carried %d events, want the 2 that were queued", delivered)
	}
}

// Without draining, every later update replays the same events.
func TestEventsDoNotAccumulateAcrossShots(t *testing.T) {
	m, _ := newSharedManager(t, 2)
	startRace(t, m, 2)

	for i := 1; i <= 40; i++ {
		m.OnShot(1, state.Shot{DecValue: 10.5, FullValue: 10, ReceivedAt: time.Now()}, i)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for i := 1; i <= 2; i++ {
		if n := len(m.sessions[i].Events); n > 0 {
			t.Errorf("range %d has %d undrained events after 40 shots", i, n)
		}
	}
}

func TestAppendEventsIsBounded(t *testing.T) {
	s := &RangePluginSession{RangeNum: 1}
	for i := 0; i < maxSessionEvents*3; i++ {
		s.appendEvents([]logicapi.PluginEvent{{Type: "tick"}})
	}
	if len(s.Events) != maxSessionEvents {
		t.Fatalf("Events = %d, want capped at %d", len(s.Events), maxSessionEvents)
	}
}

func TestActivateUnknownPluginKeepsPreviousSessions(t *testing.T) {
	m, _ := newSharedManager(t, 2)
	before := m.ActivePluginID()

	if err := m.Activate("does-not-exist"); err == nil {
		t.Fatal("Activate accepted an unknown plugin")
	}
	if got := m.ActivePluginID(); got != before {
		t.Fatalf("active plugin = %q after a failed activate, want %q", got, before)
	}
	for i := 1; i <= 2; i++ {
		if snap := m.SnapshotRange(i); snap.PluginID != before {
			t.Fatalf("range %d plugin = %q, want %q", i, snap.PluginID, before)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
