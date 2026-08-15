package main

import (
	"os"
	"path/filepath"
	"testing"

	"srdashboard/host/loader"
	"srdashboard/host/rangestate"
	"srdashboard/state"
)

// The Windows binary is `go build -o srdashboard.exe .`. go test ./host/... can
// pass while this package still forgets a blank import — then the exe logs
// "no builtin logic registered" for every renamed game folder.
func TestMainRegistersBuiltinLogicForEveryPluginFolder(t *testing.T) {
	root := "plugins"
	if _, err := os.Stat(filepath.Join(root, "autorennen", "manifest.xml")); err != nil {
		t.Skip("plugins/ not next to the main package")
	}
	pm := loader.NewManager(root)
	if err := pm.Reload(); err != nil {
		t.Fatalf("main binary would fail to load plugins: %v", err)
	}
	for _, id := range []string{"autorennen", "barrikade", "ludo"} {
		if _, err := pm.Get(id); err != nil {
			t.Errorf("Get(%s): %v", id, err)
		}
	}
	ps := rangestate.NewManager(2, pm, "autorennen")
	ps.SetLiveSource(state.NewLiveState(2))
	if err := ps.Activate("autorennen"); err != nil {
		t.Fatalf("activate autorennen: %v", err)
	}
}
