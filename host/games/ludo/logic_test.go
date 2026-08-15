package ludo

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

func TestYardNineStays(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 9.0)
	gs, _ := unmarshalState(sess)
	if !gs.Players["1"].InYard {
		t.Fatal("9.0 must not enter from yard")
	}
}

func TestTenEnters(t *testing.T) {
	l, sess := startTwo(t)
	sess, ev := fire(t, l, sess, 1, 10.0)
	found := false
	for _, e := range ev {
		if e.Type == "enter" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected enter, %#v", ev)
	}
	gs, _ := unmarshalState(sess)
	p := gs.Players["1"]
	if p.InYard || p.RingCell != p.Entry {
		t.Fatalf("inYard=%v cell=%d entry=%d", p.InYard, p.RingCell, p.Entry)
	}
}

func TestEnterCapturesOccupant(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.0)
	gs, _ := unmarshalState(sess)
	// Put player 1 on player 2's entry.
	p2 := gs.Players["2"]
	p1 := gs.Players["1"]
	p1.RingCell = p2.Entry
	p1.LapProgress = 1
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 2, 10.0)
	found := false
	for _, e := range ev {
		if e.Type == "capture" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected capture, %#v", ev)
	}
	gs, _ = unmarshalState(sess)
	if !gs.Players["1"].InYard {
		t.Fatal("player 1 should be sent to yard")
	}
}

func TestNineWalksOneTenFiveWalksTwo(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.0)
	sess, _ = fire(t, l, sess, 1, 9.0)
	gs, _ := unmarshalState(sess)
	if gs.Players["1"].LapProgress != 1 {
		t.Fatalf("progress=%d want 1", gs.Players["1"].LapProgress)
	}
	sess, _ = fire(t, l, sess, 1, 10.5)
	gs, _ = unmarshalState(sess)
	if gs.Players["1"].LapProgress != 3 {
		t.Fatalf("progress=%d want 3", gs.Players["1"].LapProgress)
	}
}

func TestSevenNoMove(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.0)
	sess, _ = fire(t, l, sess, 1, 7.0)
	gs, _ := unmarshalState(sess)
	if gs.Players["1"].LapProgress != 0 {
		t.Fatal("7.0 must not walk")
	}
}

func TestHomeClampWins(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.0)
	gs, _ := unmarshalState(sess)
	p := gs.Players["1"]
	p.LapProgress = gs.RingSize + 2 // home 3
	p.Home = 3
	p.InYard = false
	p.RingCell = -1
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ev := fire(t, l, sess, 1, 10.5) // +2 → clamp to home 4
	gs, _ = unmarshalState(sess)
	if gs.WinnerRange != 1 || gs.Phase != "finished" {
		t.Fatalf("winner=%d phase=%s home=%d ev=%#v", gs.WinnerRange, gs.Phase, gs.Players["1"].Home, ev)
	}
}

func TestJumpDoesNotCapture(t *testing.T) {
	l, sess := startTwo(t)
	sess, _ = fire(t, l, sess, 1, 10.0)
	sess, _ = fire(t, l, sess, 2, 10.0)
	gs, _ := unmarshalState(sess)
	p1 := gs.Players["1"]
	p2 := gs.Players["2"]
	// p1 at entry+1, p2 walks +2 from entry over that cell
	p1.LapProgress = 1
	p1.RingCell = (p1.Entry + 1) % gs.RingSize
	p2.LapProgress = 0
	p2.RingCell = p2.Entry
	sess, err := marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	// From entry, +2 lands on entry+2, jumping entry+1 where p1 sits.
	// But p2's entry is 8, p1's entry is 0. Put p1 on p2.entry+1.
	gs, _ = unmarshalState(sess)
	p1 = gs.Players["1"]
	p2 = gs.Players["2"]
	p1.LapProgress = 9
	p1.RingCell = (p2.Entry + 1) % gs.RingSize
	p1.InYard = false
	sess, err = marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, _ = fire(t, l, sess, 2, 10.5)
	gs, _ = unmarshalState(sess)
	if gs.Players["1"].InYard {
		t.Fatal("jumped pawn must stay")
	}
	if gs.Players["2"].LapProgress != 2 {
		t.Fatalf("p2 progress=%d", gs.Players["2"].LapProgress)
	}
}

func TestTwoPlayerUsesFullClassicBoard(t *testing.T) {
	_, sess := startTwo(t)
	gs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if gs.BoardArms != 4 {
		t.Fatalf("boardArms=%d want 4 so 2 players sit on a classic cross", gs.BoardArms)
	}
	if gs.RingSize < 40 {
		t.Fatalf("ringSize=%d want >= 40 (10 cells × 4 arms)", gs.RingSize)
	}
	p1, p2 := gs.Players["1"], gs.Players["2"]
	if p1 == nil || p2 == nil {
		t.Fatal("expected two seated players")
	}
	if p1.Entry != 0 {
		t.Fatalf("p1 entry=%d want 0", p1.Entry)
	}
	want := gs.RingSize / 2
	if p2.Entry != want {
		t.Fatalf("p2 entry=%d want opposite arm %d", p2.Entry, want)
	}
}
