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

func TestBestTeiler_IgnoresZeroMiss(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{Distance: 40, DecValue: 9.0, FullValue: 9})
	ls.ApplyShot(1, &ShotPayload{Distance: 0, DecValue: 0, FullValue: 0}) // miss at centre coords
	rs := ls.Snapshot()[0]
	if rs.BestTeiler != 40 || rs.BestTeilerShot != 1 {
		t.Fatalf("miss stole BestTeiler: best=%v shot=%d, want 40 / 1", rs.BestTeiler, rs.BestTeilerShot)
	}
}

func TestWarmupCompetitionRoundTripResetsFooter(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{Distance: 20, DecValue: 10.0, FullValue: 10, IsWarmup: true})
	ls.ApplyShot(1, &ShotPayload{Distance: 15, DecValue: 10.2, FullValue: 10, IsWarmup: false})
	if got := ls.Snapshot()[0]; got.ShotNumber != 1 || got.OverallSumInt != 10 {
		t.Fatalf("after warmup→comp: shots=%d sum=%d, want 1 / 10", got.ShotNumber, got.OverallSumInt)
	}
	ls.ApplyShot(1, &ShotPayload{Distance: 12, DecValue: 10.5, FullValue: 10, IsWarmup: false})
	ls.ApplyShot(1, &ShotPayload{Distance: 50, DecValue: 8.0, FullValue: 8, IsWarmup: true})
	got := ls.Snapshot()[0]
	if got.ShotNumber != 1 || got.OverallSumInt != 8 || !got.IsWarmup {
		t.Fatalf("after comp→warmup: shots=%d sum=%d warmup=%v, want 1 / 8 / true",
			got.ShotNumber, got.OverallSumInt, got.IsWarmup)
	}
}
