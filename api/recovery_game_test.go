package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "srdashboard/host/games/autorennen"
	"srdashboard/host/loader"
	"srdashboard/host/rangestate"
	"srdashboard/state"
	"srdashboard/udp"
)

func TestRecoveryReplayDrivesAutorennen(t *testing.T) {
	h, _ := testHandlers(t, "")
	src := filepath.Join("..", "plugins", "autorennen")
	if _, err := os.Stat(src); err != nil {
		t.Skip("autorennen plugin not present")
	}
	pluginDir := filepath.Join(filepath.Dir(h.ConfigPath), "plugins", "autorennen")
	if err := copyDir(src, pluginDir); err != nil {
		t.Fatal(err)
	}
	pm := loader.NewManager(filepath.Join(filepath.Dir(h.ConfigPath), "plugins"))
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	ps := rangestate.NewManager(2, pm, "autorennen")
	ps.SetLiveSource(h.State)
	if err := ps.Activate("autorennen"); err != nil {
		t.Fatal(err)
	}
	h.Plugins = pm
	h.PluginState = ps
	p := udp.Pipeline{
		State:  h.State,
		Filter: h,
		OnShot: func(rng int, shot state.Shot, shotIndex int) {
			ps.OnShot(rng, shot, shotIndex)
			if shot.IsWarmup {
				ps.SyncLiveReady()
			} else {
				ps.SyncLiveReadyIfArming()
			}
		},
	}
	h.ReplayLog = func(packets [][]byte) int {
		n := 0
		for _, pkt := range packets {
			n += p.IngestReplay(pkt)
		}
		return n
	}

	body := `{"clear":true,"log":` + jsonString(t, strings.Join([]string{
		"UDP: shot applied range=1 X=0 Y=0 DecValue=10.9 at=-",
		"UDP: shot applied range=2 X=0 Y=0 DecValue=10.9 at=-",
		"UDP: shot applied range=1 X=17 Y=19 DecValue=10.8 at=-",
		"UDP: shot applied range=2 X=17 Y=19 DecValue=10.8 at=-",
	}, "\n")) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/recovery/replay", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.RecoveryReplay(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp recoveryReplayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Applied != 4 {
		t.Fatalf("applied=%d want 4", resp.Applied)
	}

	snap := ps.SnapshotRange(1)
	if snap.ViewModel == nil {
		t.Fatal("missing game view model after recovery")
	}
	race, _ := snap.ViewModel["race"].(map[string]any)
	if race == nil {
		t.Fatalf("race missing: %#v", snap.ViewModel)
	}
	if race["phase"] != "racing" {
		t.Fatalf("phase=%v, want racing", race["phase"])
	}
	me, _ := snap.ViewModel["me"].(map[string]any)
	if me == nil {
		t.Fatalf("me missing: %#v", snap.ViewModel)
	}
	fired, _ := me["shotsFired"].(int)
	if fired < 1 {
		t.Fatalf("shotsFired=%v after recovered shots", me["shotsFired"])
	}
	if n := len(h.State.Snapshot()[0].Shots); n != 2 {
		t.Fatalf("live range 1 shots=%d want 2", n)
	}
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
