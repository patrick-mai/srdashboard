package tauziehen

import (
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func TestRopeMovesAndTeamAWins(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 2, "autoStartWhenAllReady": false, "ropeTarget": 8.0})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	fire := func(rn int, dec float64, x, y int) {
		t.Helper()
		var ev []logicapi.PluginEvent
		sess, ev, err = l.OnShotCtx(sess, logicapi.ShotContext{
			RangeNum: rn,
			Shot:     state.Shot{DecValue: dec, FullValue: int(dec), X: x, Y: y},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = ev
	}
	for i := 0; i < 12; i++ {
		fire(1, 10.9, 10, 10)
		gs, _ := unmarshalState(sess)
		if gs.Phase == "finished" {
			break
		}
		fire(2, 5.0, 400, 0)
		gs, _ = unmarshalState(sess)
		if gs.Phase == "finished" {
			if gs.WinnerTeam != "A" {
				t.Fatalf("winner=%s want A", gs.WinnerTeam)
			}
			return
		}
	}
	gs, _ := unmarshalState(sess)
	if gs.Phase != "finished" || gs.WinnerTeam != "A" {
		t.Fatalf("phase=%s winner=%s rope=%v", gs.Phase, gs.WinnerTeam, gs.Rope)
	}
}

func TestHoleInHoleNeverNegative(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 2, "parOverrides": map[string]any{"1": 10.5}})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	shot := func(dec float64, x, y int) {
		t.Helper()
		sess, _, err = l.OnShotCtx(sess, logicapi.ShotContext{
			RangeNum: 1, Shot: state.Shot{DecValue: dec, FullValue: int(dec), X: x, Y: y},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	shot(9.0, 20, 20)
	shot(9.0, 20, 20)
	gs, _ := unmarshalState(sess)
	p := gs.Players["1"]
	if p.LastPull <= 0 {
		t.Fatalf("hole-in-hole pull=%v must be positive", p.LastPull)
	}
}
