package rangestate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "srdashboard/host/games/ansageduell"
	_ "srdashboard/host/games/autorennen"
	_ "srdashboard/host/games/bankoderrisiko"
	_ "srdashboard/host/games/barrikade"
	_ "srdashboard/host/games/biathlon"
	_ "srdashboard/host/games/darts"
	_ "srdashboard/host/games/foxontherun"
	_ "srdashboard/host/games/kettenreaktion"
	_ "srdashboard/host/games/kopokal"
	_ "srdashboard/host/games/kronenduell"
	_ "srdashboard/host/games/ludo"
	_ "srdashboard/host/games/munitionstest"
	_ "srdashboard/host/games/schiessgolf"
	_ "srdashboard/host/games/schrumpfenderkreis"
	_ "srdashboard/host/games/tannebaum"
	_ "srdashboard/host/games/tauziehen"
	_ "srdashboard/host/games/turmbau"
	_ "srdashboard/host/games/zehnerbingo"
	"srdashboard/host/loader"
	"srdashboard/state"
)

func bundledPluginsRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "plugins")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("plugins dir: %v", err)
	}
	return root
}

func TestAllBundledPluginsLoadAndActivate(t *testing.T) {
	pm := loader.NewManager(bundledPluginsRoot(t))
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"analyse", "ansage-duell", "autorennen", "bank-oder-risiko", "barrikade", "biathlon",
		"classic-range", "classic-range-condensed", "darts", "fox-on-the-run", "kettenreaktion", "ko-pokal",
		"kronen-duell", "ludo", "munitionstest", "schiessgolf", "schrumpfender-kreis",
		"tannebaum-einzel", "tannebaum-team", "tauziehen", "turmbau", "zehner-bingo",
	}
	wantSet := map[string]bool{}
	for _, id := range want {
		wantSet[id] = true
		ap, err := pm.Get(id)
		if err != nil {
			t.Errorf("Get(%s): %v", id, err)
			continue
		}
		if ap.Manifest.ID != id {
			t.Errorf("%s: loaded id %q != folder %q", id, ap.Manifest.ID, id)
		}
		if ap.Logic == nil {
			t.Errorf("%s: no logic (builtin factory miss after rename)", id)
		}
	}
	listed, err := pm.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range listed {
		if !wantSet[info.ID] {
			t.Errorf("unexpected bundled plugin %q", info.ID)
		}
	}

	live := state.NewLiveState(2)
	ps := NewManager(2, pm, "classic-range")
	ps.SetLiveSource(live)
	for _, id := range want {
		if err := ps.Activate(id); err != nil {
			t.Errorf("Activate(%s): %v — GUI would keep the previous plugin / blank host", id, err)
			continue
		}
		if ps.ActivePluginID() != id {
			t.Errorf("active after %s = %q", id, ps.ActivePluginID())
		}
		snap := ps.SnapshotRange(1)
		if snap.PluginID != id {
			t.Errorf("%s session pluginId = %q", id, snap.PluginID)
		}
		if snap.ViewModel == nil {
			t.Errorf("%s: nil view model — hall/shooter paint has nothing to render", id)
		}
	}
}

func TestActivateLudoAndBarrikadeProduceSharedViewModels(t *testing.T) {
	for _, id := range []string{"ludo", "barrikade"} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			pluginsRoot := filepath.Join(dir, "plugins")
			src := filepath.Join("..", "..", "plugins", id)
			if err := copyDir(src, filepath.Join(pluginsRoot, id)); err != nil {
				t.Fatal(err)
			}
			pm := loader.NewManager(pluginsRoot)
			if err := pm.Reload(); err != nil {
				t.Fatal(err)
			}
			ps := NewManager(2, pm, id)
			ps.SetLiveSource(state.NewLiveState(2))
			if err := ps.Activate(id); err != nil {
				t.Fatal(err)
			}
			if !ps.sharedMode {
				t.Fatalf("%s expected shared mode", id)
			}
			if err := ps.Control("start", map[string]any{
				"live": map[string]any{
					"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
					"2": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
				},
			}); err != nil {
				t.Fatal(err)
			}
			if snap := ps.SnapshotRange(1); snap.ViewModel == nil {
				t.Fatal("nil view model after start")
			}
			if snap := ps.SnapshotRange(2); snap.ViewModel == nil {
				t.Fatal("range 2 missing shared view model")
			}
		})
	}
}

func TestPluginSwitchKeepsGame(t *testing.T) {
	pm := loader.NewManager(bundledPluginsRoot(t))
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	ps := NewManager(2, pm, "autorennen")
	ps.SetLiveSource(state.NewLiveState(2))
	if err := ps.Activate("autorennen"); err != nil {
		t.Fatal(err)
	}
	if err := ps.Control("start", map[string]any{
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
			"2": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
		},
	}); err != nil {
		t.Fatal(err)
	}
	shot := state.Shot{DecValue: 10.2, FullValue: 10, ReceivedAt: time.Now()}
	ps.OnShot(1, shot, 1)
	phase, fired, last := autorennenProgress(t, ps)
	if phase != "racing" || fired < 1 || last != 10.2 {
		t.Fatalf("before switch phase=%s shots=%d last=%.1f", phase, fired, last)
	}

	for _, id := range []string{"analyse", "classic-range"} {
		if err := ps.Activate(id); err != nil {
			t.Fatalf("Activate(%s): %v", id, err)
		}
		if ps.ActivePluginID() != id {
			t.Fatalf("active = %q, want %s", ps.ActivePluginID(), id)
		}
	}
	ps.OnShot(1, state.Shot{DecValue: 10.4, FullValue: 10, ReceivedAt: time.Now()}, 2)
	if err := ps.Activate("ludo"); err != nil {
		t.Fatal(err)
	}
	if ps.ActivePluginID() != "ludo" {
		t.Fatalf("active = %q, want ludo", ps.ActivePluginID())
	}

	if err := ps.Activate("autorennen"); err != nil {
		t.Fatal(err)
	}
	phase, fired, last = autorennenProgress(t, ps)
	if phase != "racing" {
		t.Fatalf("switch restarted the race, phase=%s", phase)
	}
	if fired < 1 || last != 10.4 {
		t.Fatalf("shot while another plugin was showing was dropped: shots=%d last=%.1f", fired, last)
	}

	// Replay clear and lane resize re-activate the plugin that is already running.
	if err := ps.Activate("autorennen"); err != nil {
		t.Fatal(err)
	}
	phase, fired, _ = autorennenProgress(t, ps)
	if phase == "racing" || fired != 0 {
		t.Fatalf("same-plugin activate should start fresh, phase=%s shots=%d", phase, fired)
	}
}

func autorennenProgress(t *testing.T, ps *Manager) (string, int, float64) {
	t.Helper()
	snap := ps.SnapshotRange(1)
	race, _ := snap.ViewModel["race"].(map[string]any)
	if race == nil {
		t.Fatalf("race missing: %#v", snap.ViewModel)
	}
	phase, _ := race["phase"].(string)
	me, _ := snap.ViewModel["me"].(map[string]any)
	if me == nil {
		t.Fatalf("me missing: %#v", snap.ViewModel)
	}
	fired, _ := me["shotsFired"].(int)
	last, _ := me["lastShotValue"].(float64)
	return phase, fired, last
}
