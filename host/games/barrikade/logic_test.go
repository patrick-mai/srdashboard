package barrikade

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
		RangeNum: rn, Shot: state.Shot{DecValue: dec, FullValue: int(dec)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, ev
}

func TestLowShotsStayPut(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 7.0)
	sess, _ = fire(t, l, sess, 1, 8.5)
	gs, _ := unmarshalState(sess)
	if gs.Players["1"].Cell != 0 {
		t.Fatalf("cell=%d want 0", gs.Players["1"].Cell)
	}
}

func TestNineStepsOne(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 9.0)
	gs, _ := unmarshalState(sess)
	if gs.Players["1"].Cell != 1 {
		t.Fatalf("cell=%d want 1", gs.Players["1"].Cell)
	}
}

func TestNineBlockedAtBarricade(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	gs.Players["1"].Cell = 7
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 1, 9.0)
	found := false
	for _, e := range ev {
		if e.Type == "blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked, %#v", ev)
	}
	gs, _ = unmarshalState(sess)
	if gs.Players["1"].Cell != 7 {
		t.Fatalf("cell=%d want 7", gs.Players["1"].Cell)
	}
}

func TestTenLiftsBarricade(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	gs.Players["1"].Cell = 7
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 1, 10.0)
	found := false
	for _, e := range ev {
		if e.Type == "lift" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected lift, %#v", ev)
	}
	gs, _ = unmarshalState(sess)
	if gs.isBarricade(8) {
		t.Fatalf("barricade still on 8: %v", gs.Barricades)
	}
	if gs.Players["1"].Cell < 8 {
		t.Fatalf("should walk onto/past 8, cell=%d", gs.Players["1"].Cell)
	}
}

func TestSurgeStopsBeforeBarricade(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	gs.Players["1"].Cell = 6
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, _ = fire(t, l, sess, 1, 10.5)
	gs, _ = unmarshalState(sess)
	if gs.Players["1"].Cell != 7 {
		t.Fatalf("10.5 from 6 should stop at 7 before wall 8, cell=%d", gs.Players["1"].Cell)
	}
	if !gs.isBarricade(8) {
		t.Fatal("should not lift wall that is not the next cell")
	}
}

func TestJumpPawn(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	gs.Players["1"].Cell = 1
	gs.Players["2"].Cell = 2
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, _ = fire(t, l, sess, 1, 9.0)
	gs, _ = unmarshalState(sess)
	if gs.Players["1"].Cell != 3 {
		t.Fatalf("should jump pawn on 2 and land on 3, cell=%d", gs.Players["1"].Cell)
	}
}

func TestFirstToCitadelWins(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	gs.Players["1"].Cell = 24
	gs.Barricades = nil
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 1, 9.0)
	gs, _ = unmarshalState(sess)
	if gs.WinnerRange != 1 || gs.Phase != "finished" {
		t.Fatalf("winner=%d phase=%s cell=%d ev=%#v", gs.WinnerRange, gs.Phase, gs.Players["1"].Cell, ev)
	}
}
