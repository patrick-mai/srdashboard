package rangestate

import (
	"path/filepath"
	"testing"

	_ "srdashboard/host/games/munitionstest"
	"srdashboard/host/loader"
	"srdashboard/state"
)

func TestMunitionstestRangesStaySeparate(t *testing.T) {
	dir := t.TempDir()
	pluginsRoot := filepath.Join(dir, "plugins")
	src := filepath.Join("..", "..", "plugins", "munitionstest")
	if err := copyDir(src, filepath.Join(pluginsRoot, "munitionstest")); err != nil {
		t.Fatal(err)
	}
	pm := loader.NewManager(pluginsRoot)
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(2, pm, "munitionstest")
	m.SetLiveSource(state.NewLiveState(2))
	if err := m.Activate("munitionstest"); err != nil {
		t.Fatal(err)
	}
	if m.sharedMode {
		t.Fatal("munitionstest must keep a session per range")
	}
	if err := m.Control("addLot", map[string]any{"name": "R50", "rangeNum": 1}); err != nil {
		t.Fatal(err)
	}
	if name := lotName(t, m, 2); name != "R50" {
		t.Fatalf("range 2 lot = %q", name)
	}
	if err := m.Control("selectLot", map[string]any{"rangeNum": 2, "lotId": 1}); err != nil {
		t.Fatal(err)
	}
	m.OnShot(2, state.Shot{X: 0, Y: 0}, 1)
	m.OnShot(2, state.Shot{X: 50, Y: 0}, 2) // 0.20 rings
	m.OnShot(1, state.Shot{X: 900, Y: 0}, 1)
	if shots := shotCount(t, m, 1); shots != 0 {
		t.Fatalf("range 1 shots = %d", shots)
	}
	if shots := shotCount(t, m, 2); shots != 2 {
		t.Fatalf("range 2 shots = %d", shots)
	}
	if err := m.Control("abort", map[string]any{"rangeNum": 2}); err != nil {
		t.Fatal(err)
	}
	if err := m.Control("addLot", map[string]any{"name": "R10", "rangeNum": 2, "select": true}); err != nil {
		t.Fatal(err)
	}
	m.OnShot(2, state.Shot{X: 0, Y: 0}, 3)
	m.OnShot(2, state.Shot{X: 70, Y: 0}, 4) // 0.28 rings, first worsening
	active := activeMap(t, m, 2)
	if asTestInt(active["abortSteps"]) != 1 || asTestFloat(active["abortScale"]) != 1.5 {
		t.Fatalf("first worsening = %#v", active)
	}
	m.OnShot(2, state.Shot{X: 150, Y: 0}, 5) // 0.60 rings, four further tenths
	active = activeMap(t, m, 2)
	if asTestInt(active["abortSteps"]) != 5 || asTestFloat(active["abortScale"]) != 3.5 {
		t.Fatalf("further worsening = %#v", active)
	}
}

func lotName(t *testing.T, m *Manager, rangeNum int) string {
	t.Helper()
	lots, _ := m.SnapshotRange(rangeNum).ViewModel["lots"].([]map[string]any)
	if len(lots) == 0 {
		return ""
	}
	name, _ := lots[0]["name"].(string)
	return name
}

func shotCount(t *testing.T, m *Manager, rangeNum int) int {
	t.Helper()
	results, _ := m.SnapshotRange(rangeNum).ViewModel["results"].([]map[string]any)
	n := 0
	for _, r := range results {
		n += asTestInt(r["shotCount"])
	}
	return n
}

func activeMap(t *testing.T, m *Manager, rangeNum int) map[string]any {
	t.Helper()
	a, _ := m.SnapshotRange(rangeNum).ViewModel["active"].(map[string]any)
	if a == nil {
		t.Fatalf("range %d has no active lot", rangeNum)
	}
	return a
}

func asTestInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	default:
		return 0
	}
}

func asTestFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return 0
	}
}
