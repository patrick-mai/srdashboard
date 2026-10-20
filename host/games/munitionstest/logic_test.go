package munitionstest

import (
	"math"
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func newLogic(t *testing.T) *Logic {
	t.Helper()
	return New(nil)
}

func mustInit(t *testing.T, l *Logic) logicapi.SessionState {
	t.Helper()
	sess, err := l.Init(map[string]any{"shotsPerLot": 10})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func mustControl(t *testing.T, l *Logic, sess logicapi.SessionState, action string, params map[string]any) logicapi.SessionState {
	t.Helper()
	out, _, err := l.Control(sess, action, params)
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	return out
}

func mustShot(t *testing.T, l *Logic, sess logicapi.SessionState, x, y int) logicapi.SessionState {
	t.Helper()
	return mustShotDisc(t, l, sess, x, y, "")
}

func mustShotDisc(t *testing.T, l *Logic, sess logicapi.SessionState, x, y int, disc string) logicapi.SessionState {
	t.Helper()
	out, _, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1,
		Shot:     state.Shot{X: x, Y: y},
		Live:     logicapi.LiveRangeInfo{DiscType: disc},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func active(t *testing.T, l *Logic, sess logicapi.SessionState) map[string]any {
	t.Helper()
	vm, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := vm["active"].(map[string]any)
	return a
}

func TestFirstShotIsCentreAndButtonStaysNormal(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50", "select": true})
	a := active(t, l, sess)
	if a["hasAbort"] != true || a["abortScale"] != 1.0 || a["abortSteps"] != 0 {
		t.Fatalf("open lot before any shot: %#v", a)
	}
	sess = mustShot(t, l, sess, 0, 0)
	a = active(t, l, sess)
	if a["hasGroup"] == true || a["shotCount"] != 1 || a["abortScale"] != 1.0 {
		t.Fatalf("centre shot: %#v", a)
	}
	// 50 DSG = 0.5 mm = 0.2 Luftgewehr rings, a good 10-shot lot.
	sess = mustShot(t, l, sess, 50, 0)
	a = active(t, l, sess)
	if a["hasGroup"] != true || a["abortSteps"] != 0 || a["abortScale"] != 1.0 {
		t.Fatalf("no other lot, good group must stay scale 1: %#v", a)
	}
	if a["diameterMm"] != 1.0 {
		t.Fatalf("diameter = %v want 1", a["diameterMm"])
	}
	if rings, _ := a["rings"].(float64); rings < 0.199 || rings > 0.201 {
		t.Fatalf("rings = %v want 0.2", a["rings"])
	}
	if a["teiler"] != 50.0 {
		t.Fatalf("teiler = %v want 50", a["teiler"])
	}
	shots, _ := a["shots"].([]map[string]any)
	if len(shots) != 2 || shots[1]["dxMm"] != 0.5 || shots[1]["dyMm"] != 0.0 || shots[1]["teiler"] != 50.0 {
		t.Fatalf("offset from centre shot: %#v", shots)
	}
	if shots[0]["dxMm"] != 0.0 || shots[0]["teiler"] != 0.0 {
		t.Fatalf("centre shot must be the origin: %#v", shots[0])
	}
}

func TestTeilerUsesCentreShotNotBull(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50", "select": true})
	// Centre sits 3 mm right and 4 mm high of the bull. The next hole is
	// 0.150 mm right and 0.400 mm high of that centre, so the raw hole is
	// not the teiler.
	sess = mustShot(t, l, sess, 300, 400)
	sess = mustShot(t, l, sess, 315, 440)
	a := active(t, l, sess)
	shots, _ := a["shots"].([]map[string]any)
	dx, _ := shots[1]["dxMm"].(float64)
	dy, _ := shots[1]["dyMm"].(float64)
	if math.Abs(dx-0.15) > 1e-9 || math.Abs(dy-0.40) > 1e-9 {
		t.Fatalf("mm offset: %#v", shots[1])
	}
	// hypot(15, 40) = 42.720, OpticScore teiler rounded to 0.1 is 42.7.
	if a["teiler"] != 42.7 || shots[1]["teiler"] != 42.7 {
		t.Fatalf("teiler = %v shot %#v", a["teiler"], shots[1])
	}
}

func TestAbortGrowsWhenWorseThanAnotherLot(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50", "select": true})
	sess = mustShot(t, l, sess, 0, 0)
	sess = mustShot(t, l, sess, 50, 0) // 0.20 rings
	sess = mustControl(t, l, sess, "abort", nil)

	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R10", "select": true})
	sess = mustShot(t, l, sess, 0, 0)
	sess = mustShot(t, l, sess, 40, 0) // 0.16 rings, still better
	a := active(t, l, sess)
	if a["abortSteps"] != 0 || a["abortScale"] != 1.0 || a["hasAbort"] != true {
		t.Fatalf("still better: %#v", a)
	}
	if a["bestName"] != "R50" {
		t.Fatalf("best = %v", a["bestName"])
	}

	sess = mustShot(t, l, sess, 60, 0) // 0.24 rings, first time worse
	a = active(t, l, sess)
	if a["abortSteps"] != 1 || a["abortScale"] != 1.5 {
		t.Fatalf("first worsening: %#v", a)
	}

	sess = mustShot(t, l, sess, 20, 0) // inside, no further growth
	a = active(t, l, sess)
	if a["abortSteps"] != 1 || a["abortScale"] != 1.5 {
		t.Fatalf("shot inside the circle: %#v", a)
	}

	sess = mustShot(t, l, sess, 100, 0) // 0.40 rings, two further tenths
	a = active(t, l, sess)
	if a["abortSteps"] != 3 || a["abortScale"] != 2.5 {
		t.Fatalf("two tenths worse: %#v", a)
	}

	sess = mustControl(t, l, sess, "undo", nil)
	a = active(t, l, sess)
	if a["abortSteps"] != 1 || a["shotCount"] != 4 {
		t.Fatalf("undo: %#v", a)
	}
}

func TestKleinkaliberUsesWiderRings(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50", "select": true})
	sess = mustShotDisc(t, l, sess, 0, 0, "KK")
	sess = mustShotDisc(t, l, sess, 160, 0, "KK") // 1.6 mm = 0.2 KK rings
	a := active(t, l, sess)
	if rings, _ := a["rings"].(float64); rings < 0.199 || rings > 0.201 {
		t.Fatalf("KK rings = %v want 0.2", a["rings"])
	}
}

func TestTenthShotClosesAndHidesAbort(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "SK", "select": true})
	for i := 0; i < 10; i++ {
		sess = mustShot(t, l, sess, i*10, 0)
	}
	vm, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	if vm["active"] == nil {
		t.Fatal("finished lot should stay on screen")
	}
	a := active(t, l, sess)
	if a["full"] != true || a["hasAbort"] != false || a["shotCount"] != 10 {
		t.Fatalf("full lot: %#v", a)
	}
	before := a["shotCount"]
	sess = mustShot(t, l, sess, 9000, 0)
	a = active(t, l, sess)
	if a["shotCount"] != before {
		t.Fatalf("shot past 10 counted: %#v", a)
	}
}

func TestShotWithoutLotIsIgnored(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50"})
	sess = mustShot(t, l, sess, 100, 0)
	vm, err := l.ViewModel(sess, 3)
	if err != nil {
		t.Fatal(err)
	}
	results, _ := vm["results"].([]map[string]any)
	if len(results) != 0 || vm["active"] != nil {
		t.Fatalf("unselected lot took a shot: %#v", vm)
	}
}

func TestApplyFanoutKeepsRangeResults(t *testing.T) {
	l := newLogic(t)
	sess := mustInit(t, l)
	sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "R50", "select": true})
	sess = mustShot(t, l, sess, 0, 0)
	other := mustInit(t, l)
	out, _, err := l.Control(sess, "addLot", map[string]any{"name": "R10"})
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := l.Control(sess, "addLot", map[string]any{"name": "R10"})
	if err != nil {
		t.Fatal(err)
	}
	_ = out
	var payload map[string]any
	for _, ev := range events {
		if ev.Type == logicapi.EventFanout {
			payload = ev.Data
		}
	}
	if payload == nil {
		t.Fatal("missing fanout")
	}
	merged, err := l.ApplyFanout(other, payload)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := l.ViewModel(merged, 2)
	if err != nil {
		t.Fatal(err)
	}
	lots, _ := vm["lots"].([]map[string]any)
	if len(lots) != 2 {
		t.Fatalf("fanout lots = %v", lots)
	}
	kept, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	a := kept["active"].(map[string]any)
	if a["shotCount"] != 1 {
		t.Fatalf("origin shots lost: %#v", a)
	}
}
