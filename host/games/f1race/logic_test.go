package f1race

import (
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func TestInitAndViewModel(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 3})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	race, _ := vm["race"].(map[string]any)
	if race == nil || race["phase"] != PhaseWarmup {
		t.Fatalf("race = %#v", race)
	}
}

func TestHoleInHoleOverlap(t *testing.T) {
	// identical centers → full overlap
	if r := circleOverlapRatio(0, 0, 0, 0, 100); r < 0.99 {
		t.Fatalf("identical = %v", r)
	}
	// far apart
	if r := circleOverlapRatio(0, 0, 1000, 0, 100); r > 0.01 {
		t.Fatalf("far = %v", r)
	}
	// partial
	r := circleOverlapRatio(0, 0, 50, 0, 100)
	if r < 0.5 || r > 0.9 {
		t.Fatalf("partial = %v", r)
	}
}

func TestReadyAndStartGate(t *testing.T) {
	l := New(nil)
	sess, _ := l.Init(map[string]any{"numRanges": 2, "autoStartWhenAllReady": false})

	now := time.Now()
	sess, _, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1,
		Shot:     state.Shot{IsWarmup: true, DecValue: 10},
		Live:     logicapi.LiveRangeInfo{IsWarmup: true, TotalShotsToFire: 40, Discipline: "40 Schuss"},
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	// leave warmup
	sess, evs, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1,
		Shot:     state.Shot{IsWarmup: false, DecValue: 9},
		Live:     logicapi.LiveRangeInfo{IsWarmup: false, TotalShotsToFire: 40, Discipline: "40 Schuss"},
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundReady := false
	for _, e := range evs {
		if e.Type == "ready" {
			foundReady = true
		}
	}
	if !foundReady {
		t.Fatalf("expected ready event, got %#v", evs)
	}

	// start blocked until both have totals — range 2 missing
	_, _, err = l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
			"2": map[string]any{"totalShotsToFire": 0, "isWarmup": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, _ := l.ViewModel(sess, 1)
	// re-get state after control — Control returns new state
	sess2, evs, err := l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
			"2": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = vm
	started := false
	for _, e := range evs {
		if e.Type == "race_start" {
			started = true
		}
	}
	if !started {
		t.Fatalf("expected race_start, %#v", evs)
	}
	vm2, _ := l.ViewModel(sess2, 1)
	race := vm2["race"].(map[string]any)
	if race["phase"] != PhaseRacing {
		t.Fatalf("phase = %v", race["phase"])
	}
}

func TestSkipRoundCrash(t *testing.T) {
	l := New(nil)
	sess, _ := l.Init(map[string]any{"numRanges": 2, "roundDurationSec": 1, "skippedRoundsToCrash": 2})
	now := time.Now()
	sess, _, _ = l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
			"2": map[string]any{"totalShotsToFire": 40, "isWarmup": false},
		},
		"now": now.Format(time.RFC3339Nano),
	})

	// Grid shots for both
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 10.5, FullValue: 10}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 2, Shot: state.Shot{DecValue: 9.0, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})

	// Open round 2 with car 1 only
	t2 := now.Add(time.Second)
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 9, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: t2,
	})

	// Expire round — car 2 skipped
	sess, evs, changed, err := l.Tick(sess, t2.Add(2*time.Second))
	if err != nil || !changed {
		t.Fatalf("tick1 changed=%v err=%v", changed, err)
	}
	_ = evs

	// Open next round with car 1, expire again — car 2 crashes
	t3 := t2.Add(3 * time.Second)
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 8, FullValue: 8}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: t3,
	})
	sess, evs, changed, err = l.Tick(sess, t3.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	crashed := false
	for _, e := range evs {
		if e.Type == "crash" {
			crashed = true
		}
	}
	if !crashed {
		vm, _ := l.ViewModel(sess, 2)
		t.Fatalf("expected crash, events=%#v me=%#v", evs, vm["me"])
	}
}

func TestSyncCarsToNumRanges(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 12})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Cars) != 12 {
		t.Fatalf("cars=%d", len(rs.Cars))
	}
	rs.NumRanges = 6
	rs.syncCarsToNumRanges()
	if len(rs.Cars) != 6 {
		t.Fatalf("after sync cars=%d want 6", len(rs.Cars))
	}
	for i := 1; i <= 6; i++ {
		if rs.Cars[strconv.Itoa(i)] == nil {
			t.Fatalf("missing car %d", i)
		}
	}
	if rs.Cars["12"] != nil {
		t.Fatal("car 12 should be removed")
	}
	vm, err := l.ViewModel(mustMarshal(t, rs), 1)
	if err != nil {
		t.Fatal(err)
	}
	race := vm["race"].(map[string]any)
	cars := race["cars"].([]map[string]any)
	if len(cars) != 6 {
		t.Fatalf("viewModel cars=%d want 6", len(cars))
	}
}

func mustMarshal(t *testing.T, rs *RaceState) logicapi.SessionState {
	t.Helper()
	b, err := marshalState(rs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEnforceMinGap(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 3, "gridGap": 0.055})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sess, _, err = l.Control(sess, "start", map[string]any{
		"numRanges": 3,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 20},
			"2": map[string]any{"totalShotsToFire": 20},
			"3": map[string]any{"totalShotsToFire": 20},
		},
		"now": now.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	// Force all cars onto the same progress — must be unpacked by enforceMinGap
	for _, c := range rs.Cars {
		c.Progress = 0.5
		c.Status = StatusRacing
	}
	rs.enforceMinGap()
	progs := make([]float64, 0, 3)
	for i := 1; i <= 3; i++ {
		progs = append(progs, rs.Cars[strconv.Itoa(i)].Progress)
	}
	sort.Float64s(progs)
	for i := 1; i < len(progs); i++ {
		gap := progs[i] - progs[i-1]
		if gap < 0.054 {
			t.Fatalf("cars too close: %v (gap %v)", progs, gap)
		}
	}
}

func TestOvertakeMaintainsGap(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{
		"numRanges": 2, "gridGap": 0.08, "overtakeRatio": 1.2, "fieldEventsEnabled": false, "stintSize": 10,
		"paceCompress": 1, "drsStackPerPlace": 0, "drsSections": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	// Fall back to Spa circuit zones when drsSections empty
	rs.DRSZones = circuitDRS["spa"]
	lead := rs.Cars["1"]
	chase := rs.Cars["2"]
	lead.Status = StatusRacing
	chase.Status = StatusRacing
	lead.Position = 1
	chase.Position = 2
	lead.LastSpeed = 8
	chase.LastSpeed = 8
	lead.ShotsFired = 5
	chase.ShotsFired = 5
	rs.snapFieldToSection(5) // section 5 mid=0.45 — outside Spa/Melbourne DRS
	leadProg := lead.Progress
	chaseProg := chase.Progress
	if leadProg <= chaseProg {
		t.Fatalf("leader should be ahead geographically: lead=%v chase=%v", leadProg, chaseProg)
	}
	// Not fast enough to pass outside DRS — order unchanged
	ot := rs.trySectionPass(chase, 8, 5)
	if ot.passed {
		t.Fatal("unexpected pass without pace advantage")
	}
	if chase.Position != 2 || lead.Position != 1 {
		t.Fatalf("positions changed without pass: lead=%d chase=%d", lead.Position, chase.Position)
	}
	// Fast enough to pass — swap race order then snap
	ot = rs.trySectionPass(chase, 8*1.5, 5)
	if !ot.passed {
		t.Fatal("expected section pass")
	}
	if chase.Position != 1 || lead.Position != 2 {
		t.Fatalf("pass did not swap positions: lead=%d chase=%d", lead.Position, chase.Position)
	}
	rs.snapFieldToSection(5)
	if chase.Progress <= lead.Progress {
		t.Fatalf("passer not ahead after snap: chase=%v lead=%v", chase.Progress, lead.Progress)
	}
}

func TestSectionSnapKeepsFieldTogether(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{"numRanges": 6, "stintSize": 10, "fieldEventsEnabled": false})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		c := rs.Cars[strconv.Itoa(i)]
		c.Status = StatusRacing
		c.Position = i
		c.ShotsFired = 7
	}
	rs.snapFieldToSection(7) // section 7 of lap 0 → base 0.7
	progs := make([]float64, 0, 6)
	for i := 1; i <= 6; i++ {
		progs = append(progs, rs.Cars[strconv.Itoa(i)].Progress)
	}
	sort.Float64s(progs)
	span := progs[len(progs)-1] - progs[0]
	if span > 0.1 {
		t.Fatalf("field span too wide for one section: %v (progs=%v)", span, progs)
	}
	// All cars should sit near section 7 marker (0.7)
	mid := (progs[0] + progs[len(progs)-1]) / 2
	if mid < 0.55 || mid > 0.75 {
		t.Fatalf("field not at section 7: mid=%v progs=%v", mid, progs)
	}
}

func TestFieldEvent(t *testing.T) {
	l := New(nil)
	sess, _ := l.Init(map[string]any{"numRanges": 2, "fieldEventsEnabled": false})
	now := time.Now()
	sess, _, _ = l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 30},
			"2": map[string]any{"totalShotsToFire": 30},
		},
	})
	sess, evs, err := l.Control(sess, "field_event", map[string]any{"type": "puncture", "now": now.Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, e := range evs {
		if e.Type == "field_event" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("events %#v", evs)
	}
	vm, _ := l.ViewModel(sess, 1)
	race := vm["race"].(map[string]any)
	if race["fieldEvent"] == nil {
		t.Fatal("expected fieldEvent in view model")
	}
}

func TestDRSActiveEvent(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{
		"numRanges": 2, "circuitId": "melbourne", "fieldEventsEnabled": false, "stintSize": 10,
		"drsSections": "2,3,4,7,8", "paceCompress": 1, "drsStackPerPlace": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sess, _, err = l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40},
			"2": map[string]any{"totalShotsToFire": 40},
		},
		"now": now.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Grid
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 10, FullValue: 10}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 2, Shot: state.Shot{DecValue: 9, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})

	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}
	// After grid, next shot is #2 → section 2 (in Sweet spot A DRS list).
	rs.Cars["1"].Position = 2
	rs.Cars["2"].Position = 1
	rs.Cars["2"].LastSpeed = 9
	rs.Cars["1"].LastSpeed = 8
	sess = mustMarshal(t, rs)

	sess, evs, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 9.5, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		if e.Type == "drs_active" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected drs_active on DRS section, got %#v", evs)
	}
}

func TestSweetSpotAPaceAndDRSStack(t *testing.T) {
	l := New(nil)
	sess, err := l.Init(map[string]any{
		"numRanges": 2, "stintSize": 10, "fieldEventsEnabled": false,
		// Sweet spot A defaults explicitly
		"overtakeRatio": 1.12, "paceCompress": 0.5, "pacePivot": 8.0,
		"drsStackPerPlace": 0.12, "drsSections": "2,3,4,7,8",
	})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := unmarshalState(sess)
	if err != nil {
		t.Fatal(err)
	}

	// pace = 8 + (10-8)*0.5 = 9
	if p := rs.compressPace(10); math.Abs(p-9) > 1e-9 {
		t.Fatalf("compressPace(10)=%v want 9", p)
	}
	// pace = 8 + (4-8)*0.5 = 6
	if p := rs.compressPace(4); math.Abs(p-6) > 1e-9 {
		t.Fatalf("compressPace(4)=%v want 6", p)
	}

	for _, s := range []int{2, 3, 4, 7, 8} {
		if !rs.sectionInDRS(s) {
			t.Fatalf("section %d should be DRS", s)
		}
	}
	if rs.sectionInDRS(5) || rs.sectionInDRS(6) || rs.sectionInDRS(9) {
		t.Fatal("sections 5/6/9 should be outside DRS")
	}

	lead := rs.Cars["1"]
	chase := rs.Cars["2"]
	lead.Status = StatusRacing
	chase.Status = StatusRacing
	lead.Position = 1
	chase.Position = 2
	lead.LastSpeed = 8.5
	// Without stack, 8.0 < 8.5 — no pass. With P2 stack 1.12: 8*1.12=8.96 >= 8.5.
	ot := rs.trySectionPass(chase, 8.0, 3)
	if !ot.passed || !ot.usedDRS {
		t.Fatalf("expected DRS stack pass, got passed=%v viaDRS=%v", ot.passed, ot.usedDRS)
	}
	if chase.Position != 1 || lead.Position != 2 {
		t.Fatalf("positions after DRS stack: lead=%d chase=%d", lead.Position, chase.Position)
	}

	// Outside DRS, same speeds should not pass at ratio 1.12
	lead.Position = 1
	chase.Position = 2
	lead.LastSpeed = 8.5
	ot = rs.trySectionPass(chase, 8.0, 5)
	if ot.passed {
		t.Fatal("unexpected non-DRS pass without overtakeRatio margin")
	}
}

func TestPitCueArmsOnRoundEntry(t *testing.T) {
	l := New(nil)
	sess, _ := l.Init(map[string]any{
		"numRanges": 2, "stintSize": 10, "fieldEventsEnabled": false, "roundDurationSec": 120,
	})
	now := time.Now()
	sess, _, _ = l.Control(sess, "start", map[string]any{
		"numRanges": 2,
		"live": map[string]any{
			"1": map[string]any{"totalShotsToFire": 40},
			"2": map[string]any{"totalShotsToFire": 40},
		},
		"now": now.Format(time.RFC3339Nano),
	})
	// Grid = shot 1 → round becomes 2
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 10, FullValue: 10}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})
	sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 2, Shot: state.Shot{DecValue: 9, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: now,
	})

	// Fire rounds 2..9 for both cars so the advance into round 10 arms pit.
	tshot := now
	for round := 2; round <= 9; round++ {
		tshot = tshot.Add(time.Second)
		var evs []logicapi.PluginEvent
		sess, _, _ = l.OnShotCtx(sess, logicapi.ShotContext{
			RangeNum: 1, Shot: state.Shot{DecValue: 9, FullValue: 9}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: tshot,
		})
		sess, evs, _ = l.OnShotCtx(sess, logicapi.ShotContext{
			RangeNum: 2, Shot: state.Shot{DecValue: 8, FullValue: 8}, Live: logicapi.LiveRangeInfo{TotalShotsToFire: 40}, Now: tshot,
		})
		if round == 9 {
			hasPit := false
			for _, e := range evs {
				if e.Type == "pit_cue" {
					hasPit = true
				}
			}
			if !hasPit {
				t.Fatalf("expected pit_cue when entering round 10, events=%#v", evs)
			}
		}
	}

	vm, _ := l.ViewModel(sess, 1)
	race := vm["race"].(map[string]any)
	if race["currentRound"] != 10 {
		t.Fatalf("round=%v want 10", race["currentRound"])
	}
	if race["pitCueAt"] == nil {
		t.Fatal("expected shared pitCueAt after arming")
	}
	if race["isPitRound"] != true {
		t.Fatalf("isPitRound=%v", race["isPitRound"])
	}
	if race["pitWindowMs"] == nil {
		t.Fatal("expected pitWindowMs")
	}
}

