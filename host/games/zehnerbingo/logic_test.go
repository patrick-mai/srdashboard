package zehnerbingo

import (
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func startTwo(t *testing.T) (*Logic, logicapi.SessionState) {
	t.Helper()
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 2, "autoStartWhenAllReady": false})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	return l, sess
}

func fire(t *testing.T, l *Logic, sess logicapi.SessionState, rn int, dec float64) (logicapi.SessionState, []logicapi.PluginEvent) {
	t.Helper()
	out, ev, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: rn,
		Shot:     state.Shot{DecValue: dec, FullValue: int(dec)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, ev
}

func TestSevenIsMiss(t *testing.T) {
	l, sess := startTwo(t)
	sess, ev := fire(t, l, sess, 1, 7.0)
	found := false
	for _, e := range ev {
		if e.Type == "miss" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected miss, got %#v", ev)
	}
	gs, _ := unmarshalState(sess)
	for _, m := range gs.Players["1"].Marked {
		if m {
			t.Fatal("7.0 must not mark a cell")
		}
	}
}

func TestWaterfallSecondTenNine(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.9)
	gs, _ := unmarshalState(sess)
	if !gs.Players["1"].Marked[0] {
		t.Fatal("first 10.9 should mark 10.9 (index 0)")
	}
	sess, ev := fire(t, l, sess, 1, 10.9)
	found := false
	for _, e := range ev {
		if e.Type == "mark" && gameFloat(e.Data["value"]) == 10.8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("second 10.9 should waterfall to 10.8, got %#v", ev)
	}
}

func TestNineTwoCannotTakeNineFive(t *testing.T) {
	l, sess := startTwo(t)
	// mark 9.0 (index 5) first with a 9.2
	sess, _ = fire(t, l, sess, 1, 9.2)
	gs, _ := unmarshalState(sess)
	if !gs.Players["1"].Marked[5] {
		t.Fatalf("9.2 should mark 9.0, marked=%v", gs.Players["1"].Marked)
	}
	sess, ev := fire(t, l, sess, 1, 9.2)
	found := false
	for _, e := range ev {
		if e.Type == "miss" {
			found = true
		}
	}
	if !found {
		t.Fatalf("second 9.2 should miss (cannot take 9.5), got %#v", ev)
	}
}

func TestLineWin(t *testing.T) {
	l, sess := startTwo(t)
	// Bottom row: 10.3, 10.8, 10.6 (indices 6,7,8)
	for _, v := range []float64{10.3, 10.8, 10.6} {
		var ev []logicapi.PluginEvent
		sess, ev = fire(t, l, sess, 1, v)
		_ = ev
	}
	gs, _ := unmarshalState(sess)
	if gs.WinnerRange != 1 || gs.Phase != "finished" {
		t.Fatalf("winner=%d phase=%s marked=%v", gs.WinnerRange, gs.Phase, gs.Players["1"].Marked)
	}
}

func TestHandicapLetsEightCountAsNine(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{
		"numRanges": 2, "autoStartWhenAllReady": false,
		"handicaps": map[string]any{"1": 1.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 1, 8.0)
	found := false
	for _, e := range ev {
		if e.Type == "mark" && gameFloat(e.Data["value"]) == 9.0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("8.0 + handicap 1.0 should mark 9.0, got %#v", ev)
	}
}

func gameFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	}
	return 0
}
