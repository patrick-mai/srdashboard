package rangestate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "srdashboard/host/games/autorennen"
	"srdashboard/host/loader"
	"srdashboard/state"
)

func TestSharedAutorennenActivateAndControl(t *testing.T) {
	dir := t.TempDir()
	pluginsRoot := filepath.Join(dir, "plugins")
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

	live := state.NewLiveState(2)
	// Seed discipline totals via ApplyShot menu - or Control live map
	ps := NewManager(2, pm, "autorennen")
	ps.SetLiveSource(live)

	if err := ps.Activate("autorennen"); err != nil {
		t.Fatal(err)
	}
	if !ps.sharedMode {
		t.Fatal("expected shared mode")
	}

	err := ps.Control("start", map[string]any{
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
			"2": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ps.OnShot(1, state.Shot{DecValue: 10.2, FullValue: 10, ReceivedAt: time.Now()}, 1)
	snap1 := ps.SnapshotRange(1)
	snap2 := ps.SnapshotRange(2)
	if snap1.ViewModel == nil || snap2.ViewModel == nil {
		t.Fatal("expected view models on both ranges")
	}
	r1, _ := snap1.ViewModel["race"].(map[string]any)
	r2, _ := snap2.ViewModel["race"].(map[string]any)
	if r1 == nil || r2 == nil {
		t.Fatalf("race missing: %#v %#v", snap1.ViewModel, snap2.ViewModel)
	}
	if r1["phase"] != "racing" {
		t.Fatalf("phase=%v", r1["phase"])
	}
}
