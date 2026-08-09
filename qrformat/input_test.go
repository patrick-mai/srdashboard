package qrformat

import (
	"testing"

	"srdashboard/state"
)

func TestOpenShotsSkippedWhenSeriesComplete(t *testing.T) {
	last := make([]state.Shot, 10)
	for i := range last {
		last[i].DecValue = 10
	}
	snap := state.RangeSnapshot{
		DiscType:    "LG",
		SeriesShots: [][]state.Shot{last, last, last},
		Shots:       last, // completed 3rd series still on target
	}
	in := FromRangeSnapshot(snap)
	if len(in.Series) != 3 {
		t.Fatalf("series=%d", len(in.Series))
	}
	if len(in.OpenShots) != 0 {
		t.Fatalf("open=%d want 0 (avoid duplicating last series)", len(in.OpenShots))
	}
}

func TestOpenShotsIncludedWhenIncomplete(t *testing.T) {
	snap := state.RangeSnapshot{
		SeriesShots: [][]state.Shot{{{DecValue: 10}}},
		Shots:       []state.Shot{{DecValue: 9}, {DecValue: 9.5}},
	}
	in := FromRangeSnapshot(snap)
	if len(in.OpenShots) != 2 {
		t.Fatalf("open=%d want 2", len(in.OpenShots))
	}
}
