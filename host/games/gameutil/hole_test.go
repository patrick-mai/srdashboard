package gameutil

import "testing"

func TestCircleOverlapRatioIdentical(t *testing.T) {
	if got := CircleOverlapRatio(0, 0, 0, 0, 225); got != 1 {
		t.Fatalf("identical circles overlap=%v want 1", got)
	}
}

func TestCircleOverlapRatioSeparated(t *testing.T) {
	if got := CircleOverlapRatio(0, 0, 1000, 0, 225); got != 0 {
		t.Fatalf("far circles overlap=%v want 0", got)
	}
}

func TestHoleInHoleValueGate(t *testing.T) {
	cfg := map[string]any{}
	ok, _ := HoleInHole(0, 0, 0, 0, 8.5, cfg)
	if ok {
		t.Fatal("8.5 must not pass the strict > 8.5 gate")
	}
	ok, _ = HoleInHole(0, 0, 0, 0, 8.6, cfg)
	if !ok {
		t.Fatal("8.6 identical hole must count")
	}
}

func TestHoleInHoleIgnoresHandicapShape(t *testing.T) {
	cfg := map[string]any{"handicaps": map[string]any{"1": 2.0}}
	ok, _ := HoleInHole(0, 0, 0, 0, 8.4, cfg)
	if ok {
		t.Fatal("raw 8.4 must not count even if a handicap would lift it")
	}
}

func TestHoleInHoleFiftyPercentDistance(t *testing.T) {
	cfg := map[string]any{"shotDiameterMm": 4.5, "dsgPerMm": 100.0, "holeInHoleMinOverlap": 0.5}
	// 50% of equal 4.5 mm circles is ~182 DSG centre distance.
	ok, ov := HoleInHole(0, 0, 180, 0, 9.0, cfg)
	if !ok {
		t.Fatalf("180 DSG should be over 50%% overlap, got %v", ov)
	}
	ok, ov = HoleInHole(0, 0, 220, 0, 9.0, cfg)
	if ok {
		t.Fatalf("220 DSG should be under 50%% overlap, got %v", ov)
	}
}
