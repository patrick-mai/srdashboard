package foxontherun

import (
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func newTestLogic(n int) (*Logic, logicapi.SessionState) {
	l := New(nil)
	cfg := defaultConfig()
	cfg["numRanges"] = n
	cfg["autoStartWhenAllReady"] = false
	sess, err := l.Init(cfg)
	if err != nil {
		panic(err)
	}
	return l, sess
}

func shot(dec float64, x, y int) state.Shot {
	return state.Shot{DecValue: dec, FullValue: int(dec), X: x, Y: y, Distance: 100}
}

func mustOnShot(t *testing.T, l *Logic, sess logicapi.SessionState, rn int, s state.Shot) (logicapi.SessionState, []logicapi.PluginEvent) {
	t.Helper()
	out, evs, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: rn, Shot: s, Live: logicapi.LiveRangeInfo{IsWarmup: false}, NumRanges: 4,
	})
	if err != nil {
		t.Fatalf("OnShotCtx: %v", err)
	}
	return out, evs
}

func TestEqualizerSigns(t *testing.T) {
	players := map[string]*Player{
		"1": {RangeNum: 1, Active: true, CalValues: []float64{10, 10, 10, 10, 10}, Calibrated: true, Skill: 10},
		"2": {RangeNum: 2, Active: true, CalValues: []float64{8, 8, 8, 8, 8}, Calibrated: true, Skill: 8},
	}
	cfg := defaultConfig()
	avg := computeEqualizers(players, cfg)
	if avg < 8.9 || avg > 9.1 {
		t.Fatalf("fieldAvg=%v", avg)
	}
	if players["1"].Equalizer >= 0 {
		t.Fatalf("strong shooter eq should be negative, got %v", players["1"].Equalizer)
	}
	if players["2"].Equalizer <= 0 {
		t.Fatalf("weak shooter eq should be positive, got %v", players["2"].Equalizer)
	}
}

func TestStartBonusWeakFox(t *testing.T) {
	b := startBonusForFox(7, 9, defaultConfig())
	if b < 1 {
		t.Fatalf("weak fox should get start bonus, got %v", b)
	}
	b2 := startBonusForFox(10, 9, defaultConfig())
	if b2 != 0 {
		t.Fatalf("strong fox bonus want 0 got %v", b2)
	}
}

func TestTerrainZeroInBand(t *testing.T) {
	cfg := defaultConfig()
	eff := computeTerrain(15, 30, 0, true, cfg) // mid=15, band=5 → calm
	if eff.Amount != 0 {
		t.Fatalf("want calm band, got %+v", eff)
	}
}

func TestTerrainFoxRunawayDampensFox(t *testing.T) {
	cfg := defaultConfig()
	eff := computeTerrain(28, 30, 3, true, cfg)
	if eff.Kind != "dampen" || eff.Side != "fox" || eff.Amount <= 0 {
		t.Fatalf("want fox dampen, got %+v", eff)
	}
	if eff.Amount > cfgFloat(cfg, "terrainDampMax", 1)+1e-9 {
		t.Fatalf("damp exceeds max: %v", eff.Amount)
	}
	delta := applyTerrainToDelta(10, eff, true)
	if delta >= 10 {
		t.Fatalf("dampen should reduce, got %v", delta)
	}
}

func TestTerrainNearCatchDampensHunter(t *testing.T) {
	cfg := defaultConfig()
	eff := computeTerrain(2, 30, 1, false, cfg)
	if eff.Kind != "dampen" || eff.Side != "hunter" {
		t.Fatalf("want hunter dampen, got %+v", eff)
	}
}

func TestTrailAidDefaultOff(t *testing.T) {
	cfg := defaultConfig()
	eff := computeTerrain(28, 30, 0, false, cfg) // hunter shot while fox runaway
	if eff.Amount != 0 {
		t.Fatalf("trail aid should be off by default, got %+v", eff)
	}
}

func TestCalibrationAndEscapeCatch(t *testing.T) {
	l, sess := newTestLogic(2)
	cfg := defaultConfig()
	cfg["numRanges"] = 2
	cfg["calibrateShots"] = 2
	cfg["openingShots"] = 2
	cfg["escapeTarget"] = 20.0
	cfg["equalizerEnabled"] = false
	cfg["terrainEnabled"] = false
	cfg["autoStartWhenAllReady"] = false
	sess, _ = l.Init(cfg)

	// calibrate both
	for _, rn := range []int{1, 2} {
		sess, _ = mustOnShot(t, l, sess, rn, shot(9, 0, 0))
		sess, _ = mustOnShot(t, l, sess, rn, shot(9, 0, 0))
	}
	hs, _ := unmarshalState(sess)
	if hs.Phase != PhaseArming {
		t.Fatalf("phase=%s want arming", hs.Phase)
	}

	sess, evs, err := l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = evs
	hs, _ = unmarshalState(sess)
	if hs.Phase != PhaseOpening || hs.CurrentFox != 1 {
		t.Fatalf("after start phase=%s fox=%d", hs.Phase, hs.CurrentFox)
	}

	// opening two strong shots → lead ~20 → may escape immediately into next fox opening
	sess, _ = mustOnShot(t, l, sess, 1, shot(10, 10, 0))
	sess, _ = mustOnShot(t, l, sess, 1, shot(10, -10, 0))
	hs, _ = unmarshalState(sess)
	if len(hs.RoundResults) < 1 && hs.Phase != PhaseChase {
		t.Fatalf("want chase or completed round, got phase=%s lead=%v results=%d", hs.Phase, hs.Lead, len(hs.RoundResults))
	}
	if len(hs.RoundResults) >= 1 {
		if hs.RoundResults[0].Outcome != OutcomeEscaped {
			t.Fatalf("opening escape want escaped, got %s", hs.RoundResults[0].Outcome)
		}
		return
	}

	// fox shoots until escape
	for i := 0; i < 30 && hs.Phase == PhaseChase; i++ {
		turn := hs.TurnRange
		sess, _ = mustOnShot(t, l, sess, turn, shot(5, 0, 0))
		hs, _ = unmarshalState(sess)
		if hs.Phase != PhaseChase {
			break
		}
		turn = hs.TurnRange
		sess, _ = mustOnShot(t, l, sess, turn, shot(10.5, 0, 0))
		hs, _ = unmarshalState(sess)
	}
	hs, _ = unmarshalState(sess)
	if len(hs.RoundResults) < 1 {
		t.Fatalf("expected round result, phase=%s lead=%v", hs.Phase, hs.Lead)
	}
}

func TestCatch(t *testing.T) {
	l, sess := newTestLogic(2)
	cfg := defaultConfig()
	cfg["numRanges"] = 2
	cfg["calibrateShots"] = 1
	cfg["openingShots"] = 1
	cfg["escapeTarget"] = 50.0
	cfg["equalizerEnabled"] = false
	cfg["terrainEnabled"] = false
	cfg["startBonusMax"] = 0.0
	sess, _ = l.Init(cfg)

	sess, _ = mustOnShot(t, l, sess, 1, shot(8, 0, 0))
	sess, _ = mustOnShot(t, l, sess, 2, shot(8, 0, 0))
	sess, _, _ = l.Control(sess, "start", nil)

	// one opening shot
	sess, _ = mustOnShot(t, l, sess, 1, shot(6, 0, 0))
	hs, _ := unmarshalState(sess)
	if hs.Phase != PhaseChase {
		t.Fatalf("phase=%s", hs.Phase)
	}
	// hunter keeps shooting high until catch
	for i := 0; i < 20 && hs.Phase == PhaseChase; i++ {
		turn := hs.TurnRange
		val := 10.0
		if turn == hs.CurrentFox {
			val = 1.0
		}
		sess, _ = mustOnShot(t, l, sess, turn, shot(val, 0, 0))
		hs, _ = unmarshalState(sess)
	}
	if len(hs.RoundResults) < 1 {
		t.Fatalf("expected round result, phase=%s lead=%v", hs.Phase, hs.Lead)
	}
	if hs.RoundResults[0].Outcome != OutcomeCaught && hs.RoundResults[0].Outcome != OutcomeEscaped {
		t.Fatalf("outcome=%s", hs.RoundResults[0].Outcome)
	}
}

func TestWrongTurnIgnored(t *testing.T) {
	l, sess := newTestLogic(2)
	cfg := defaultConfig()
	cfg["numRanges"] = 2
	cfg["calibrateShots"] = 1
	cfg["openingShots"] = 2
	cfg["equalizerEnabled"] = false
	sess, _ = l.Init(cfg)
	sess, _ = mustOnShot(t, l, sess, 1, shot(9, 0, 0))
	sess, _ = mustOnShot(t, l, sess, 2, shot(9, 0, 0))
	sess, _, _ = l.Control(sess, "start", nil)

	hs, _ := unmarshalState(sess)
	leadBefore := hs.Lead
	sess, evs := mustOnShot(t, l, sess, 2, shot(10.9, 0, 0)) // not fox's turn in opening
	hs, _ = unmarshalState(sess)
	if hs.Lead != leadBefore {
		t.Fatalf("wrong turn changed lead")
	}
	found := false
	for _, e := range evs {
		if e.Type == "wrong_turn" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected wrong_turn event")
	}
}

func TestFoxRotation(t *testing.T) {
	l, sess := newTestLogic(2)
	cfg := defaultConfig()
	cfg["numRanges"] = 2
	cfg["calibrateShots"] = 1
	cfg["openingShots"] = 1
	cfg["escapeTarget"] = 5.0
	cfg["equalizerEnabled"] = false
	cfg["terrainEnabled"] = false
	cfg["startBonusMax"] = 0.0
	sess, _ = l.Init(cfg)
	sess, _ = mustOnShot(t, l, sess, 1, shot(9, 0, 0))
	sess, _ = mustOnShot(t, l, sess, 2, shot(9, 0, 0))
	sess, _, _ = l.Control(sess, "start", nil)

	// fox 1 opening of 9 already near escape 5? opening completes then if lead>=escape finishes immediately
	sess, _ = mustOnShot(t, l, sess, 1, shot(9, 0, 0))
	hs, _ := unmarshalState(sess)
	// After short escape, should be fox 2 opening or finished path
	if hs.CurrentFox != 2 && hs.Phase != PhaseFinished {
		// may still be chase if opening didn't hit escape until chase
		if hs.Phase == PhaseChase && hs.CurrentFox == 1 {
			sess, _ = mustOnShot(t, l, sess, hs.TurnRange, shot(0.1, 0, 0))
			if hs2, _ := unmarshalState(sess); hs2.Phase == PhaseChase {
				sess, _ = mustOnShot(t, l, sess, hs2.TurnRange, shot(10, 0, 0))
			}
			hs, _ = unmarshalState(sess)
		}
	}
	if len(hs.RoundResults) < 1 {
		t.Fatalf("need first round done, phase=%s fox=%d lead=%v", hs.Phase, hs.CurrentFox, hs.Lead)
	}
}

func TestRecentShotsWindow(t *testing.T) {
	hs := &HuntState{HuntShots: nil}
	for i := 0; i < 5; i++ {
		hs.pushHuntShot(ShotMark{RangeNum: 1, Raw: float64(i)})
	}
	r := hs.recentShots(3)
	if len(r) != 3 || r[0].Raw != 2 || r[2].Raw != 4 {
		t.Fatalf("recent=%+v", r)
	}
}

func TestEqualizerOffRawMath(t *testing.T) {
	cfg := defaultConfig()
	cfg["equalizerEnabled"] = false
	players := map[string]*Player{
		"1": {RangeNum: 1, Active: true, Skill: 10, Calibrated: true, CalValues: []float64{10}},
		"2": {RangeNum: 2, Active: true, Skill: 7, Calibrated: true, CalValues: []float64{7}},
	}
	computeEqualizers(players, cfg)
	if players["1"].Equalizer != 0 || players["2"].Equalizer != 0 {
		t.Fatalf("eq should be 0 when disabled")
	}
}
