package rangestate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"srdashboard/host/loader"
	"srdashboard/state"
)

func TestActivateDisplayPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginsRoot := filepath.Join(dir, "plugins")
	src := filepath.Join("..", "..", "plugins", "classic-range")
	if _, err := os.Stat(src); err != nil {
		t.Skip("classic-range not present")
	}
	if err := copyDir(src, filepath.Join(pluginsRoot, "classic-range")); err != nil {
		t.Fatal(err)
	}

	pm := loader.NewManager(pluginsRoot)
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}

	live := state.NewLiveState(2)
	ps := NewManager(2, pm, "classic-range")
	ps.SetLiveSource(live)

	if err := ps.EnsureActive(); err != nil {
		t.Fatal(err)
	}
	if ps.ActivePluginID() != "classic-range" {
		t.Fatalf("active = %q", ps.ActivePluginID())
	}

	snap := ps.SnapshotRange(1)
	if snap.PluginID != "classic-range" || snap.Phase != PhaseActive {
		t.Fatalf("session = %+v", snap)
	}
	if snap.ViewModel == nil || snap.ViewModel["kind"] != "display" {
		t.Fatalf("viewModel = %#v", snap.ViewModel)
	}

	shot := state.Shot{X: 1, Y: 2, DecValue: 10.5, ReceivedAt: time.Now()}
	ps.OnShot(1, shot, 0)
	snap = ps.SnapshotRange(1)
	if snap.ShotCount != 1 {
		t.Fatalf("shotCount = %d", snap.ShotCount)
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
