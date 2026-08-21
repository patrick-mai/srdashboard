package gameutil

import "testing"

func TestWarmupDiscard(t *testing.T) {
	discard, openNow := WarmupDiscard(false, true)
	if !discard || openNow {
		t.Fatalf("probe before open: discard=%v open=%v", discard, openNow)
	}
	discard, openNow = WarmupDiscard(false, false)
	if discard || !openNow {
		t.Fatalf("first competition: discard=%v open=%v", discard, openNow)
	}
	discard, openNow = WarmupDiscard(true, true)
	if discard || openNow {
		t.Fatalf("probe after open must count: discard=%v open=%v", discard, openNow)
	}
	discard, openNow = WarmupDiscard(true, false)
	if discard || openNow {
		t.Fatalf("later competition: discard=%v open=%v", discard, openNow)
	}
}

func TestInactiveSetFromParams(t *testing.T) {
	_, present := InactiveSetFromParams(nil)
	if present {
		t.Fatal("nil params")
	}
	_, present = InactiveSetFromParams(map[string]any{"numRanges": 4})
	if present {
		t.Fatal("missing key")
	}
	set, present := InactiveSetFromParams(map[string]any{"inactiveRanges": []any{}})
	if !present || len(set) != 0 {
		t.Fatalf("empty list present=%v set=%v", present, set)
	}
	set, present = InactiveSetFromParams(map[string]any{"inactiveRanges": []int{2, 5}})
	if !present || !set[2] || !set[5] || set[1] {
		t.Fatalf("set=%v", set)
	}
}

func TestLiveActive(t *testing.T) {
	if !LiveActive(nil, true) || LiveActive(nil, false) {
		t.Fatal("nil keeps default")
	}
	if LiveActive(map[string]any{"active": false}, true) {
		t.Fatal("explicit false")
	}
	if !LiveActive(map[string]any{"discipline": "LG"}, true) {
		t.Fatal("missing key keeps default")
	}
}
