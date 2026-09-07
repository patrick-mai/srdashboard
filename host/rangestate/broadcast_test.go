package rangestate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "srdashboard/host/games/autorennen"
	_ "srdashboard/host/games/zehnerbingo"
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
	src := filepath.Join("..", "..", "plugins", "autorennen")
	if _, err := os.Stat(src); err != nil {
		t.Skip("autorennen not present")
	}
	if err := copyDir(src, filepath.Join(pluginsRoot, "autorennen")); err != nil {
		t.Fatal(err)
	}
	pm := loader.NewManager(pluginsRoot)
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(ranges, pm, "autorennen")
	m.SetLiveSource(state.NewLiveState(ranges))
	b := &recordingBroadcaster{}
	m.SetBroadcaster(b)
	if err := m.Activate("autorennen"); err != nil {
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

// Shared games send one plugin_session (range 1) to every client. A shot on
// stand 2 must still put events on that payload, or hall/tablets stay silent.
func TestSharedShotOnNonFirstRangeBroadcastsEvents(t *testing.T) {
	m, b := newSharedManager(t, 2)
	startRace(t, m, 2)
	b.reset()

	m.OnShot(2, state.Shot{DecValue: 10.2, FullValue: 10, ReceivedAt: time.Now()}, 1)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sessions) != 1 {
		t.Fatalf("got %d plugin_session messages, want 1", len(b.sessions))
	}
	if len(b.sessions[0].Events) == 0 {
		t.Fatal("shot on range 2 produced no events in the shared broadcast")
	}
}

func newBingoManager(t *testing.T, ranges int) (*Manager, *recordingBroadcaster) {
	t.Helper()
	pm := loader.NewManager(filepath.Join("..", "..", "plugins"))
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(ranges, pm, "zehner-bingo")
	m.SetLiveSource(state.NewLiveState(ranges))
	b := &recordingBroadcaster{}
	m.SetBroadcaster(b)
	if err := m.Activate("zehner-bingo"); err != nil {
		t.Fatal(err)
	}
	live := map[string]any{}
	for i := 1; i <= ranges; i++ {
		live[itoa(i)] = map[string]any{"totalShotsToFire": 40, "isWarmup": false}
	}
	if err := m.Control("start", map[string]any{"live": live}); err != nil {
		t.Fatal(err)
	}
	return m, b
}

func TestBingoShotOnNonFirstRangeBroadcastsEvents(t *testing.T) {
	m, b := newBingoManager(t, 2)
	b.reset()

	m.OnShot(2, state.Shot{DecValue: 10.9, FullValue: 10, ReceivedAt: time.Now()}, 1)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sessions) != 1 {
		t.Fatalf("got %d plugin_session messages, want 1", len(b.sessions))
	}
	got := map[string]bool{}
	for _, ev := range b.sessions[0].Events {
		got[ev.Type] = true
	}
	if !got["mark"] && !got["miss"] {
		t.Fatalf("bingo shot on range 2 produced %#v, want mark or miss so tablets can beep", b.sessions[0].Events)
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

func TestReplaySuppressesPerShotBroadcast(t *testing.T) {
	m, b := newSharedManager(t, 2)
	startRace(t, m, 2)
	b.reset()

	m.BeginReplay()
	m.OnShot(1, state.Shot{DecValue: 10.5, FullValue: 10, ReceivedAt: time.Now()}, 1)
	m.OnShot(2, state.Shot{DecValue: 9.8, FullValue: 9, ReceivedAt: time.Now()}, 1)
	if n := b.sessionMessages(); n != 0 {
		t.Fatalf("replay leaked %d plugin_session messages", n)
	}
	m.EndReplay()
	if n := b.sessionMessages(); n == 0 {
		t.Fatal("EndReplay should notify clients once")
	}
}

func TestReplayOnShotStartsAutorennenWithoutProgram(t *testing.T) {
	m, b := newSharedManager(t, 2)
	b.reset()
	m.BeginReplay()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	m.OnShot(1, state.Shot{DecValue: 10.9, FullValue: 10, At: now, ReceivedAt: now}, 1)
	m.OnShot(2, state.Shot{DecValue: 10.8, FullValue: 10, At: now.Add(time.Second), ReceivedAt: now.Add(time.Second)}, 1)
	m.EndReplay()

	snap := m.SnapshotRange(1)
	if snap.ViewModel == nil {
		t.Fatal("missing view model after replay")
	}
	race, _ := snap.ViewModel["race"].(map[string]any)
	if race == nil {
		t.Fatalf("race missing: %#v", snap.ViewModel)
	}
	if race["phase"] != "racing" {
		t.Fatalf("phase=%v, want racing (replay should auto-start without OpticScore program length)", race["phase"])
	}
	me, _ := snap.ViewModel["me"].(map[string]any)
	if me == nil {
		t.Fatalf("me missing: %#v", snap.ViewModel)
	}
	fired, _ := me["shotsFired"].(int)
	if fired < 1 {
		t.Fatalf("shotsFired=%v, want at least the recovered grid shot", me["shotsFired"])
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
