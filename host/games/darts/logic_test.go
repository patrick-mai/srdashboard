package darts

import (
	"math"
	"strconv"
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func started(t *testing.T, cfg map[string]any) (*Logic, logicapi.SessionState) {
	t.Helper()
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg["numRanges"] = 2
	l := New(nil)
	sess, err := l.Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	return l, sess
}

func fire(t *testing.T, l *Logic, sess logicapi.SessionState, rn, x, y int, dec float64, warmup bool) (logicapi.SessionState, []logicapi.PluginEvent) {
	t.Helper()
	sess, ev, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: rn,
		Shot:     state.Shot{X: x, Y: y, DecValue: dec, FullValue: 10, IsWarmup: warmup},
		Live:     logicapi.LiveRangeInfo{IsWarmup: warmup, ShooterName: "S" + strconv.Itoa(rn)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sess, ev
}

func stateOf(t *testing.T, sess logicapi.SessionState) *GameState {
	t.Helper()
	gs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	return gs
}

func rem(gs *GameState, rn int) int {
	p := gs.Players[strconv.Itoa(rn)]
	if p == nil {
		return -1
	}
	return p.Remaining
}

func at(mm, deg float64) (int, int) {
	rad := deg * math.Pi / 180
	return int(math.Round(math.Sin(rad) * mm * 100)), int(math.Round(math.Cos(rad) * mm * 100))
}

type dart struct {
	x, y, pts int
	label     string
	double    bool
}

func knownDarts() []dart {
	s1x, s1y := at(20, 18)
	return []dart{
		{0, 3960, 60, "T20", false},
		{0, 0, 50, "50", true},
		{0, 6635, 40, "D20", true},
		{0, 600, 25, "25", false},
		{0, 2000, 20, "S20", false},
		{s1x, s1y, 1, "S1", false},
	}
}

func reduceTo(t *testing.T, l *Logic, sess logicapi.SessionState, rn, target int) logicapi.SessionState {
	t.Helper()
	for i := 0; i < 40; i++ {
		cur := rem(stateOf(t, sess), rn)
		if cur == target {
			return sess
		}
		gs := stateOf(t, sess)
		fired := false
		for _, d := range knownDarts() {
			next := cur - d.pts
			if next < target || next < 0 {
				continue
			}
			if gs.doubleOut() && (next == 1 || (next == 0 && !d.double)) {
				continue
			}
			if gs.doubleIn() && !gs.Players[strconv.Itoa(rn)].Opened && !d.double {
				continue
			}
			sess, _ = fire(t, l, sess, rn, d.x, d.y, 8.0, false)
			got := stateOf(t, sess)
			if rem(got, rn) != next {
				t.Fatalf("%s from %d left %d, want %d", d.label, cur, rem(got, rn), next)
			}
			fired = true
			break
		}
		if !fired {
			t.Fatalf("cannot reach %d from %d", target, cur)
		}
	}
	t.Fatalf("too many darts reducing to %d", target)
	return sess
}

func TestDefaultIsPlain501(t *testing.T) {
	l, sess := started(t, nil)
	gs := stateOf(t, sess)
	if gs.RulesLabel != "501" || rem(gs, 1) != 501 || gs.Calibrating {
		t.Fatalf("start %s rem=%d cal=%v", gs.RulesLabel, rem(gs, 1), gs.Calibrating)
	}
	vm, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	game := vm["game"].(map[string]any)
	if game["rulesLabel"] != "501" || game["doubleIn"] != false || game["doubleOut"] != false {
		t.Fatalf("view %+v", game)
	}
}

func TestWarmupAndMissIgnoreDecimal(t *testing.T) {
	l, sess := started(t, map[string]any{"doubleIn": true})
	sess, _ = fire(t, l, sess, 1, 0, 6635, 10.9, true)
	gs := stateOf(t, sess)
	if rem(gs, 1) != 501 || gs.Players["1"].Opened {
		t.Fatalf("warmup rem=%d opened=%v", rem(gs, 1), gs.Players["1"].Opened)
	}
	sess, _ = fire(t, l, sess, 1, 0, 8000, 10.9, false)
	gs = stateOf(t, sess)
	last := gs.Shots[len(gs.Shots)-1]
	if rem(gs, 1) != 501 || last.Points != 0 || last.Label != "0" || last.DecValue != 10.9 {
		t.Fatalf("decimal miss rem=%d mark=%+v", rem(gs, 1), last)
	}
}

func TestPlainCountdown(t *testing.T) {
	for _, start := range []int{301, 701} {
		_, sess := started(t, map[string]any{"startScore": start})
		if rem(stateOf(t, sess), 1) != start {
			t.Fatalf("start %d rem %d", start, rem(stateOf(t, sess), 1))
		}
	}

	l, sess := started(t, map[string]any{"startScore": 301})
	sess = reduceTo(t, l, sess, 1, 21)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 9.0, false) // S20 leaves 1
	gs := stateOf(t, sess)
	if rem(gs, 1) != 1 || gs.Phase == "finished" {
		t.Fatalf("leave 1 rem=%d phase=%s", rem(gs, 1), gs.Phase)
	}

	l, sess = started(t, map[string]any{"startScore": 301})
	sess = reduceTo(t, l, sess, 1, 20)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 4.2, false)
	gs = stateOf(t, sess)
	if rem(gs, 1) != 0 || gs.WinnerRange != 1 {
		t.Fatalf("single checkout rem=%d winner=%d", rem(gs, 1), gs.WinnerRange)
	}

	l, sess = started(t, map[string]any{"startScore": 301})
	sess = reduceTo(t, l, sess, 1, 20)
	sess, _ = fire(t, l, sess, 1, 0, 3960, 8.0, false) // T20
	gs = stateOf(t, sess)
	if rem(gs, 1) != 20 || gs.WinnerRange != 0 {
		t.Fatalf("below 0 rem=%d winner=%d", rem(gs, 1), gs.WinnerRange)
	}
}

func TestDoubleIn(t *testing.T) {
	l, sess := started(t, map[string]any{"doubleIn": true})
	sess, _ = fire(t, l, sess, 1, 0, 2000, 10.9, false)
	sess, _ = fire(t, l, sess, 1, 0, 600, 10.9, false)
	gs := stateOf(t, sess)
	if rem(gs, 1) != 501 || gs.Players["1"].Opened {
		t.Fatalf("non-double rem=%d opened=%v", rem(gs, 1), gs.Players["1"].Opened)
	}
	sess, _ = fire(t, l, sess, 1, 0, 0, 10.9, false)
	gs = stateOf(t, sess)
	if !gs.Players["1"].Opened || rem(gs, 1) != 451 {
		t.Fatalf("inner bull rem=%d opened=%v", rem(gs, 1), gs.Players["1"].Opened)
	}

	l, sess = started(t, map[string]any{"doubleIn": true})
	sess, _ = fire(t, l, sess, 1, 0, 6635, 1.0, false)
	gs = stateOf(t, sess)
	if !gs.Players["1"].Opened || rem(gs, 1) != 461 {
		t.Fatalf("D20 rem=%d opened=%v", rem(gs, 1), gs.Players["1"].Opened)
	}
	sess, _ = fire(t, l, sess, 1, 0, 2000, 1.0, false)
	if rem(stateOf(t, sess), 1) != 441 {
		t.Fatalf("later single rem=%d", rem(stateOf(t, sess), 1))
	}
}

func TestDoubleOut(t *testing.T) {
	l, sess := started(t, map[string]any{"doubleOut": true, "startScore": 501})
	if stateOf(t, sess).RulesLabel != "501, Double-Out" {
		t.Fatalf("rules %s", stateOf(t, sess).RulesLabel)
	}
	sess = reduceTo(t, l, sess, 1, 40)
	sess, _ = fire(t, l, sess, 1, 0, 6635, 5.0, false)
	gs := stateOf(t, sess)
	if gs.WinnerRange != 1 || rem(gs, 1) != 0 {
		t.Fatalf("D20 checkout rem=%d winner=%d", rem(gs, 1), gs.WinnerRange)
	}

	l, sess = started(t, map[string]any{"doubleOut": true, "startScore": 301})
	sess = reduceTo(t, l, sess, 1, 50)
	sess, _ = fire(t, l, sess, 1, 0, 0, 10.9, false)
	gs = stateOf(t, sess)
	if gs.WinnerRange != 1 || rem(gs, 1) != 0 {
		t.Fatalf("inner bull checkout rem=%d winner=%d", rem(gs, 1), gs.WinnerRange)
	}

	l, sess = started(t, map[string]any{"doubleOut": true, "startScore": 301})
	sess = reduceTo(t, l, sess, 1, 20)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 9.0, false)
	gs = stateOf(t, sess)
	if rem(gs, 1) != 20 || gs.WinnerRange != 0 {
		t.Fatalf("non-double zero rem=%d winner=%d", rem(gs, 1), gs.WinnerRange)
	}

	l, sess = started(t, map[string]any{"doubleOut": true, "startScore": 301})
	sess = reduceTo(t, l, sess, 1, 21)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 9.0, false)
	if rem(stateOf(t, sess), 1) != 21 {
		t.Fatalf("leave 1 rem=%d", rem(stateOf(t, sess), 1))
	}

	l, sess = started(t, map[string]any{"doubleOut": true, "startScore": 301})
	sess = reduceTo(t, l, sess, 1, 25)
	sess, _ = fire(t, l, sess, 1, 0, 600, 9.0, false)
	if rem(stateOf(t, sess), 1) != 25 {
		t.Fatalf("outer bull finish rem=%d", rem(stateOf(t, sess), 1))
	}

	l, sess = started(t, map[string]any{"doubleOut": true, "startScore": 301})
	sess = reduceTo(t, l, sess, 1, 10)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 9.0, false)
	if rem(stateOf(t, sess), 1) != 10 {
		t.Fatalf("below 0 rem=%d", rem(stateOf(t, sess), 1))
	}
}

func TestDoubleInAndOut(t *testing.T) {
	l, sess := started(t, map[string]any{"doubleIn": true, "doubleOut": true, "startScore": 301})
	if stateOf(t, sess).RulesLabel != "301, Double-In, Double-Out" {
		t.Fatalf("rules %s", stateOf(t, sess).RulesLabel)
	}
	sess, _ = fire(t, l, sess, 1, 0, 2000, 8.0, false)
	if rem(stateOf(t, sess), 1) != 301 {
		t.Fatal("single opened")
	}
	sess, _ = fire(t, l, sess, 1, 0, 6635, 8.0, false)
	gs := stateOf(t, sess)
	if rem(gs, 1) != 261 || !gs.Players["1"].Opened {
		t.Fatalf("open rem=%d opened=%v", rem(gs, 1), gs.Players["1"].Opened)
	}
	sess = reduceTo(t, l, sess, 1, 20)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 8.0, false) // S20 would finish, not a double
	if rem(stateOf(t, sess), 1) != 20 {
		t.Fatalf("non-double finish rem=%d", rem(stateOf(t, sess), 1))
	}
	dx, dy := at(66.35, 108) // D10
	sess, _ = fire(t, l, sess, 1, dx, dy, 8.0, false)
	gs = stateOf(t, sess)
	if gs.Shots[len(gs.Shots)-1].Label != "D10" || gs.WinnerRange != 1 || rem(gs, 1) != 0 {
		t.Fatalf("double finish label=%s rem=%d winner=%d", gs.Shots[len(gs.Shots)-1].Label, rem(gs, 1), gs.WinnerRange)
	}
}

func TestParallelBustAndLastThree(t *testing.T) {
	l, sess := started(t, map[string]any{"startScore": 301})
	sess = reduceTo(t, l, sess, 1, 20)
	sess, _ = fire(t, l, sess, 1, 0, 3960, 7.0, false)
	sess = reduceTo(t, l, sess, 2, 20)
	sess, _ = fire(t, l, sess, 2, 0, 2000, 7.0, false)
	gs := stateOf(t, sess)
	if rem(gs, 1) != 20 || rem(gs, 2) != 0 || gs.WinnerRange != 2 {
		t.Fatalf("parallel rem=%d/%d winner=%d", rem(gs, 1), rem(gs, 2), gs.WinnerRange)
	}

	l, sess = started(t, nil)
	sess, _ = fire(t, l, sess, 1, 0, 2000, 8.0, false)
	sess, _ = fire(t, l, sess, 2, 0, 2000, 8.0, false)
	sess, _ = fire(t, l, sess, 1, 2000, 0, 8.0, false)
	sess, _ = fire(t, l, sess, 1, 0, 8000, 8.0, false)
	sess, _ = fire(t, l, sess, 1, 0, 6635, 8.0, false)
	gs = stateOf(t, sess)
	three := gs.Players["1"].LastThree
	if len(three) != 3 || three[0] != "S6" || three[1] != "0" || three[2] != "D20" {
		t.Fatalf("last three %v", three)
	}
	if rem(gs, 2) != 481 {
		t.Fatalf("other lane rem=%d", rem(gs, 2))
	}
}

func TestCalibrationShotSetsScaleOnly(t *testing.T) {
	l, sess := started(t, map[string]any{"boardScale": 1.5})
	sess, _, err := l.Control(sess, "calibrate", nil)
	if err != nil {
		t.Fatal(err)
	}
	before := rem(stateOf(t, sess), 1)
	sess, _ = fire(t, l, sess, 1, 0, 0, 10.9, false)
	gs := stateOf(t, sess)
	if cfgScale(gs.Config) != 1.5 || !gs.Calibrating || rem(gs, 1) != before {
		t.Fatalf("centre scale=%v cal=%v rem=%d", cfgScale(gs.Config), gs.Calibrating, rem(gs, 1))
	}
	sess, ev := fire(t, l, sess, 1, 0, int(AssumedOuterDoubleDSG*2), 10.9, false)
	gs = stateOf(t, sess)
	if cfgScale(gs.Config) != 2 || gs.Calibrating || rem(gs, 1) != before {
		t.Fatalf("scale=%v cal=%v rem=%d", cfgScale(gs.Config), gs.Calibrating, rem(gs, 1))
	}
	found := false
	for _, e := range ev {
		if e.Type == "persist_config" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing persist_config")
	}
	if !stringsContains(gs.StatusLine, "138,6") {
		t.Fatalf("status %q", gs.StatusLine)
	}
	sess, _ = fire(t, l, sess, 1, 0, 7920, 8.0, false)
	gs = stateOf(t, sess)
	if gs.Shots[len(gs.Shots)-1].Label != "T20" || rem(gs, 1) != before-60 {
		t.Fatalf("T20 %+v rem=%d", gs.Shots[len(gs.Shots)-1], rem(gs, 1))
	}
	sess, _ = fire(t, l, sess, 1, 0, 13270, 8.0, false)
	gs = stateOf(t, sess)
	if gs.Shots[len(gs.Shots)-1].Label != "D20" || rem(gs, 1) != before-100 {
		t.Fatalf("D20 %+v rem=%d", gs.Shots[len(gs.Shots)-1], rem(gs, 1))
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
