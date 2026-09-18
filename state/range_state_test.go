package state

import (
	"math"
	"testing"
)

func TestParseTotalShotsFromMenuItem(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"LG 20 Schuss", 20},
		{"LG 40 Schuss", 40},
		{"LG 30 Schuss Auflage", 30},
		{"LP 40 Schuss", 40},
		{"LP 30 Schuss Auflage", 30},
		{"40 Schuss", 40},
		{"30 Schuss", 30},
		{"60 Schuss", 60},
		{"3x20", 60},
		{"3x20 Schuss", 60},
		{"KK 3×20", 60},
		{"3x40", 120},
		{"20/20/20", 60},
		{"unbegrenzt", 100},
		{"Unbegrenzt", 100},
		{"UNBEGRENZT", 100},
		{"Luftgewehr unbegrenzt", 100},
		{"1000 Schuss", 100},
		{"1000 Schuss unbegrenzt", 100},
		{"", 0},
		{"something else", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseTotalShotsFromMenuItem(tt.name); got != tt.want {
				t.Errorf("ParseTotalShotsFromMenuItem(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestPrediction(t *testing.T) {
	tests := []struct {
		name             string
		shotNumber       int
		overallSumInt    int
		overallSumDec    float64
		totalShotsToFire int
		wantInt          int     // predicted integer (ring) sum
		wantDec          float64 // predicted decimal sum (float comparison with tolerance)
	}{
		{
			name:             "no shots",
			shotNumber:       0,
			overallSumInt:    0,
			overallSumDec:    0,
			totalShotsToFire: 30,
			wantInt:          0,
			wantDec:          0,
		},
		{
			name:             "10 shots sum 105/105.3 total 30",
			shotNumber:       10,
			overallSumInt:    105,
			overallSumDec:    105.3,
			totalShotsToFire: 30,
			wantInt:          315,   // (105/10)*30
			wantDec:          315.9, // (105.3/10)*30
		},
		{
			name:             "5 shots sum 52/52.5 total 30",
			shotNumber:       5,
			overallSumInt:    52,
			overallSumDec:    52.5,
			totalShotsToFire: 30,
			wantInt:          312, // (52/5)*30
			wantDec:          315, // (52.5/5)*30
		},
		{
			name:             "1 shot 10/10.7 total 40",
			shotNumber:       1,
			overallSumInt:    10,
			overallSumDec:    10.7,
			totalShotsToFire: 40,
			wantInt:          400,
			wantDec:          428,
		},
		{
			name:             "20 shots sum 209/209.4 total 30",
			shotNumber:       20,
			overallSumInt:    209,
			overallSumDec:    209.4,
			totalShotsToFire: 30,
			wantInt:          314,   // Round((209/20)*30) = Round(313.5)
			wantDec:          314.1, // (209.4/20)*30
		},
		{
			name:             "already past planned total returns current sum",
			shotNumber:       35,
			overallSumInt:    340,
			overallSumDec:    342.5,
			totalShotsToFire: 30,
			wantInt:          340,
			wantDec:          342.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &RangeState{
				ShotNumber:       tt.shotNumber,
				OverallSumInt:    tt.overallSumInt,
				OverallSumDec:    tt.overallSumDec,
				TotalShotsToFire: tt.totalShotsToFire,
			}
			gotInt, gotDec := rs.Prediction()
			if gotInt != tt.wantInt {
				t.Errorf("Prediction() int = %v, want %v", gotInt, tt.wantInt)
			}
			if math.Abs(gotDec-tt.wantDec) > 0.0001 {
				t.Errorf("Prediction() dec = %v, want %v", gotDec, tt.wantDec)
			}
		})
	}
}
