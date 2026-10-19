package darts

import "testing"

func TestClassifyMeasuredBoard(t *testing.T) {
	cases := []struct {
		name     string
		x, y     int
		label    string
		points   int
		isDouble bool
	}{
		{name: "centre", x: 0, y: 0, label: "50", points: 50, isDouble: true},
		{name: "just outside inner bull", x: 0, y: 410, label: "25", points: 25, isDouble: false},
		{name: "double 20", x: 0, y: 6635, label: "D20", points: 40, isDouble: true},
		{name: "treble 6", x: 3960, y: 0, label: "T6", points: 18, isDouble: false},
		{name: "number ring", x: 0, y: 8000, label: "0", points: 0, isDouble: false},
		{name: "past double", x: 0, y: 7000, label: "0", points: 0, isDouble: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := Classify(tc.x, tc.y, 1)
			if h.Label != tc.label || h.Points != tc.points || h.IsDouble != tc.isDouble {
				t.Fatalf("Classify(%d,%d) = %+v, want %s/%d double=%v", tc.x, tc.y, h, tc.label, tc.points, tc.isDouble)
			}
		})
	}
}

func TestInnerBullEdgeStaysInner(t *testing.T) {
	h := Classify(0, 400, 1)
	if h.Label != "50" {
		t.Fatalf("r = 4.0 mm label %s, want 50", h.Label)
	}
}

func TestScaledBoardDoublesZones(t *testing.T) {
	z := ScaledRadii(2)
	if z.InnerBull != 8 || z.OuterBull != 16 || z.TrebleIn != 73.6 || z.TrebleOut != 84.8 ||
		z.DoubleIn != 126.8 || z.DoubleOut != 138.6 || z.NumberIn != 142.8 || z.NumberOut != 170.6 {
		t.Fatalf("scaled radii %+v", z)
	}
	// Original T20 / D20 radii land in the scaled rings of 20.
	t20 := Classify(0, 7920, 2)
	if t20.Label != "T20" || t20.Points != 60 {
		t.Fatalf("scaled T20 = %+v", t20)
	}
	d20 := Classify(0, 13270, 2)
	if d20.Label != "D20" || d20.Points != 40 || !d20.IsDouble {
		t.Fatalf("scaled D20 = %+v", d20)
	}
}

func TestCalibrationScale(t *testing.T) {
	scale, ok := CalibrationScale(0, int(AssumedOuterDoubleDSG*2))
	if !ok || scale != 2 {
		t.Fatalf("twice outer double = %v ok=%v, want 2", scale, ok)
	}
	if _, ok := CalibrationScale(0, 0); ok {
		t.Fatal("centre must not set a scale")
	}
	if _, ok := CalibrationScale(0, 200); ok {
		t.Fatal("inner bull must not set a scale")
	}
}
