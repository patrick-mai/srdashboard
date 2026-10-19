package rangestate

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"srdashboard/host/loader"
	"srdashboard/state"
)

func TestDartsCalibrationPersistsScale(t *testing.T) {
	pluginsRoot := filepath.Join(t.TempDir(), "plugins")
	src := filepath.Join("..", "..", "plugins", "darts")
	if err := copyDir(src, filepath.Join(pluginsRoot, "darts")); err != nil {
		t.Fatal(err)
	}
	pm := loader.NewManager(pluginsRoot)
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(2, pm, "darts")
	m.SetLiveSource(state.NewLiveState(2))
	if err := m.Activate("darts"); err != nil {
		t.Fatal(err)
	}
	if err := m.Control("start", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Control("calibrate", nil); err != nil {
		t.Fatal(err)
	}
	before := dartsRemaining(t, m, 1)
	m.OnShot(1, state.Shot{X: 0, Y: 0, DecValue: 10.9, FullValue: 10}, 1)
	if dartsScale(t, m) != 1 || dartsRemaining(t, m, 1) != before {
		t.Fatalf("centre scale=%v rem=%d", dartsScale(t, m), dartsRemaining(t, m, 1))
	}
	m.OnShot(1, state.Shot{X: 0, Y: 8000, DecValue: 9.0, FullValue: 9}, 2)
	got := dartsScale(t, m)
	want := 8000.0 / 6930.0
	if math.Abs(got-want) > 1e-9 || dartsRemaining(t, m, 1) != before {
		t.Fatalf("scale=%v want %v rem=%d", got, want, dartsRemaining(t, m, 1))
	}
	raw, err := os.ReadFile(filepath.Join(pluginsRoot, "darts", "config.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(raw), "boardScale") {
		t.Fatalf("config.xml missing boardScale: %s", raw)
	}

	pm2 := loader.NewManager(pluginsRoot)
	if err := pm2.Reload(); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(2, pm2, "darts")
	m2.SetLiveSource(state.NewLiveState(2))
	if err := m2.Activate("darts"); err != nil {
		t.Fatal(err)
	}
	if math.Abs(dartsScale(t, m2)-want) > 1e-6 {
		t.Fatalf("restart scale=%v want %v", dartsScale(t, m2), want)
	}
}

func dartsGame(t *testing.T, m *Manager) map[string]any {
	t.Helper()
	vm := m.SnapshotRange(1).ViewModel
	game, _ := vm["game"].(map[string]any)
	if game == nil {
		t.Fatalf("missing game view: %#v", vm)
	}
	return game
}

func dartsScale(t *testing.T, m *Manager) float64 {
	t.Helper()
	switch v := dartsGame(t, m)["boardScale"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		t.Fatalf("boardScale %T %v", v, v)
		return 0
	}
}

func dartsRemaining(t *testing.T, m *Manager, rn int) int {
	t.Helper()
	players, _ := dartsGame(t, m)["players"].([]map[string]any)
	if players == nil {
		raw, _ := dartsGame(t, m)["players"].([]any)
		for _, item := range raw {
			p, _ := item.(map[string]any)
			if n, _ := p["rangeNum"].(float64); int(n) == rn {
				switch rem := p["remaining"].(type) {
				case float64:
					return int(rem)
				case int:
					return rem
				}
			}
		}
		t.Fatal("player missing")
	}
	for _, p := range players {
		if n, _ := p["rangeNum"].(int); n == rn {
			if rem, ok := p["remaining"].(int); ok {
				return rem
			}
		}
	}
	t.Fatal("player missing")
	return -1
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexContains(s, sub))
}

func indexContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
