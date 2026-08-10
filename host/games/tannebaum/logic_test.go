package tannebaum

import (
	"testing"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func TestMapShotDownRounding(t *testing.T) {
	cases := []struct {
		raw   float64
		stage string
		want  float64
		ok    bool
	}{
		{7.9, StageA, 7, true},
		{10.9, StageA, 10, true},
		{4.9, StageA, 0, false},
		{8.7, StageB, 8.5, true},
		{9.99, StageB, 9.5, true},
		{10.4, StageB, 10, true},
		{10.5, StageB, 0, false},
		{10.87, StageC, 10.8, true},
		{10.9, StageC, 10.9, true},
		{10.49, StageC, 0, false},
	}
	for _, tc := range cases {
		got, ok := mapShotToStage(tc.raw, tc.stage)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("mapShotToStage(%v,%s)=%v,%v want %v,%v", tc.raw, tc.stage, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCandidatesPreferPreciseStage(t *testing.T) {
	cands := mapShotCandidates(10.9)
	if len(cands) < 1 || cands[0].StageID != StageC || cands[0].Value != 10.9 {
		t.Fatalf("expected C/10.9 first, got %#v", cands)
	}
	cands = mapShotCandidates(9.3)
	if len(cands) < 1 || cands[0].StageID != StageB || cands[0].Value != 9.0 {
		t.Fatalf("expected B/9.0 first, got %#v", cands)
	}
}

func TestEinzelOwnStrikeAndWin(t *testing.T) {
	l := New(nil, ModeEinzel)
	sess, err := l.Init(map[string]any{"numRanges": 2, "autoStartWhenAllReady": false})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clear A(5–7), then B(8–10), then remaining A(8–10), then C.
	ordered := []float64{5, 6, 7, 8.0, 8.5, 9.0, 9.5, 10.0, 8.1, 9.1, 10.1, 10.5, 10.6, 10.7, 10.8, 10.9}

	for _, v := range ordered {
		var ev []logicapi.PluginEvent
		sess, ev, err = l.OnShotCtx(sess, logicapi.ShotContext{
			RangeNum: 1,
			Shot:     state.Shot{DecValue: v, FullValue: int(v)},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = ev
	}
	gs, _ := unmarshalState(sess)
	c := gs.Contenders["r1"]
	if c == nil || !c.Finished {
		t.Fatalf("expected r1 finished, contender remaining=%d stageA=%v", totalRemaining(c), c.Stages[StageA])
	}
	if gs.WinnerID != "r1" || gs.Phase != PhaseFinished {
		t.Fatalf("winner=%s phase=%s", gs.WinnerID, gs.Phase)
	}
}

func TestGiftWhenOwnStageLeafGone(t *testing.T) {
	l := New(nil, ModeEinzel)
	sess, err := l.Init(map[string]any{"numRanges": 2, "autoStartWhenAllReady": false})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Range 1 clears A(5)
	sess, _, err = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 5.2, FullValue: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Range 1 shoots 5.x again → gift to range 2
	sess, ev, err := l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 5.9, FullValue: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	foundGift := false
	for _, e := range ev {
		if e.Type == "strike" && e.Data["gift"] == true {
			foundGift = true
			if e.Data["contenderId"] != "r2" {
				t.Fatalf("gift target=%v", e.Data["contenderId"])
			}
		}
	}
	if !foundGift {
		t.Fatalf("expected gift event, got %#v", ev)
	}
	gs, _ := unmarshalState(sess)
	if hasLeaf(gs.Contenders["r2"].Stages[StageA], 5) {
		t.Fatal("r2 should have lost leaf 5 via gift")
	}
}

func TestSetTeamFromControl(t *testing.T) {
	l := New(nil, ModeTeam)
	sess, err := l.Init(map[string]any{"numRanges": 4, "autoStartWhenAllReady": false})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Stand 1 default odd → A; move to B
	sess, ev, err := l.Control(sess, "set_team", map[string]any{"rangeNum": 1, "team": "b"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range ev {
		if e.Type == "team_pick" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected team_pick event, got %#v", ev)
	}
	gs, _ := unmarshalState(sess)
	if gs.TeamPicks["1"] != "b" {
		t.Fatalf("pick=%v", gs.TeamPicks)
	}
	if gs.Shooters["1"].ContenderID != "team-b" {
		t.Fatalf("contender=%s", gs.Shooters["1"].ContenderID)
	}
	// Preserve tree progress after reassignment
	strikeLeaf(gs.Contenders["team-b"].Stages[StageA], 5)
	sess, err = marshalState(gs)
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "set_team", map[string]any{"rangeNum": 3, "team": "b"})
	if err != nil {
		t.Fatal(err)
	}
	gs, _ = unmarshalState(sess)
	if hasLeaf(gs.Contenders["team-b"].Stages[StageA], 5) {
		t.Fatal("team-b leaf 5 should stay cleared after membership change")
	}
}

func TestTeamSharedTree(t *testing.T) {
	l := New(nil, ModeTeam)
	sess, err := l.Init(map[string]any{"numRanges": 4, "autoStartWhenAllReady": false})
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err = l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	gs, _ := unmarshalState(sess)
	if len(gs.Contenders) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(gs.Contenders))
	}
	if gs.Shooters["1"].ContenderID != "team-a" || gs.Shooters["2"].ContenderID != "team-b" {
		t.Fatalf("assignment a=%s b=%s", gs.Shooters["1"].ContenderID, gs.Shooters["2"].ContenderID)
	}
	sess, _, err = l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: 1, Shot: state.Shot{DecValue: 6.1, FullValue: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	gs, _ = unmarshalState(sess)
	if hasLeaf(gs.Contenders["team-a"].Stages[StageA], 6) {
		t.Fatal("team-a should have struck 6")
	}
}
