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

func cardIndex(card []float64, want float64) int {
	wantTenth := int(want*10 + 0.5)
	for i, v := range card {
		if int(v*10+0.5) == wantTenth {
			return i
		}
	}
	return -1
}

func TestCardIsFiveByFiveAndShuffledAtStart(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	if len(gs.Card) != CardSize {
		t.Fatalf("card size %d want %d", len(gs.Card), CardSize)
	}
	counts := map[int]int{}
	for _, v := range gs.Card {
		counts[int(v*10+0.5)]++
	}
	for tenth := cardMinTenth; tenth <= cardMinTenth+CardSize-1; tenth++ {
		if counts[tenth] != 1 {
			t.Fatalf("value %.1f count %d want 1", float64(tenth)/10, counts[tenth])
		}
	}
	canonical := canonicalCard()
	same := true
	for i := range canonical {
		if gs.Card[i] != canonical[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("card should be shuffled at start")
	}

	sess, _, err := l.Control(sess, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	gs2, _ := unmarshalState(sess)
	same = true
	for i := range gs.Card {
		if gs.Card[i] != gs2.Card[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("start should shuffle a new card")
	}
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
	idx := cardIndex(gs.Card, 10.9)
	if idx < 0 || !gs.Players["1"].Marked[idx] {
		t.Fatal("first 10.9 should mark 10.9")
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
	var last []logicapi.PluginEvent
	missed := false
	for i := 0; i < CardSize; i++ {
		sess, last = fire(t, l, sess, 1, 9.2)
		for _, e := range last {
			if e.Type == "miss" {
				missed = true
			}
		}
		if missed {
			break
		}
	}
	if !missed {
		t.Fatalf("expected miss after cells ≤ 9.2 are gone, got %#v", last)
	}
	gs, _ := unmarshalState(sess)
	idx := cardIndex(gs.Card, 9.5)
	if idx >= 0 && gs.Players["1"].Marked[idx] {
		t.Fatal("9.2 must not mark 9.5")
	}
}

func TestLineWin(t *testing.T) {
	l, sess := startTwo(t)
	gs, _ := unmarshalState(sess)
	for _, v := range gs.Card[:GridN] {
		sess, _ = fire(t, l, sess, 1, v)
	}
	gs, _ = unmarshalState(sess)
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
