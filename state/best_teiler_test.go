package state

import (
	"testing"
)

func TestBestTeiler_FortyShotsPerRange(t *testing.T) {
	const (
		numRanges   = 6
		shotsPerRng = 40
		bestShot    = 17 // 1-based shot number with the lowest Distance
		bestDist    = 3.5
		otherDist   = 50.0
	)

	ls := NewLiveState(numRanges)

	for rng := 1; rng <= numRanges; rng++ {
		for i := 1; i <= shotsPerRng; i++ {
			dist := otherDist + float64(i) // always worse than bestDist
			if i == bestShot {
				dist = bestDist
			}
			ls.ApplyShot(rng, &ShotPayload{
				X:         int(dist),
				Y:         0,
				Distance:  dist,
				FullValue: 9,
				DecValue:  9.0,
				Range:     rng,
				IsWarmup:  false,
				Shooter: &struct {
					Firstname string `json:"Firstname"`
					Lastname  string `json:"Lastname"`
					Club      *struct {
						Name string `json:"Name"`
					} `json:"Club"`
				}{
					Firstname: "Test",
					Lastname:  "Shooter",
				},
				MenuItem: &struct {
					MenuPointName string `json:"MenuPointName"`
					MenuItemName  string `json:"MenuItemName"`
				}{
					MenuItemName: "LG 40 Schuss",
				},
			})
		}
	}

	snap := ls.Snapshot()
	if len(snap) != numRanges {
		t.Fatalf("Snapshot() len = %d, want %d", len(snap), numRanges)
	}
	for _, rs := range snap {
		if rs.ShotNumber != shotsPerRng {
			t.Errorf("range %d: ShotNumber = %d, want %d", rs.RangeNum, rs.ShotNumber, shotsPerRng)
		}
		if rs.BestTeiler != bestDist {
			t.Errorf("range %d: BestTeiler = %v, want %v", rs.RangeNum, rs.BestTeiler, bestDist)
		}
		if rs.BestTeilerShot != bestShot {
			t.Errorf("range %d: BestTeilerShot = %d, want %d", rs.RangeNum, rs.BestTeilerShot, bestShot)
		}
		if rs.CurrentTeiler != otherDist+float64(shotsPerRng) {
			t.Errorf("range %d: CurrentTeiler = %v, want last-shot distance", rs.RangeNum, rs.CurrentTeiler)
		}
	}
}

func TestBestTeiler_ResetsOnShooterChange(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{
		Distance: 5.0, DecValue: 10.0, FullValue: 10, Range: 1,
		Shooter: &struct {
			Firstname string `json:"Firstname"`
			Lastname  string `json:"Lastname"`
			Club      *struct {
				Name string `json:"Name"`
			} `json:"Club"`
		}{Firstname: "A", Lastname: "One"},
	})
	ls.ApplyShot(1, &ShotPayload{
		Distance: 99.0, DecValue: 8.0, FullValue: 8, Range: 1,
		Shooter: &struct {
			Firstname string `json:"Firstname"`
			Lastname  string `json:"Lastname"`
			Club      *struct {
				Name string `json:"Name"`
			} `json:"Club"`
		}{Firstname: "B", Lastname: "Two"},
	})
	rs := ls.Snapshot()[0]
	if rs.BestTeiler != 99.0 || rs.BestTeilerShot != 1 {
		t.Fatalf("after shooter change: best=%v shot=%d, want 99.0 / 1", rs.BestTeiler, rs.BestTeilerShot)
	}
}
