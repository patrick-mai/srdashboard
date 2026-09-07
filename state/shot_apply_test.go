package state

import (
	"math"
	"testing"
	"time"
)

func TestApplyShotRecordsTimestamps(t *testing.T) {
	ls := NewLiveState(1)
	before := time.Now()
	if !ls.ApplyShot(1, &ShotPayload{X: 10, Y: -5, DecValue: 10.4, FullValue: 10, ShotDateTime: "2026-07-30 19:15:04.250"}) {
		t.Fatal("ApplyShot reported an unknown range")
	}

	shots := ls.Snapshot()[0].Shots
	if len(shots) != 1 {
		t.Fatalf("got %d shots, want 1", len(shots))
	}
	shot := shots[0]
	if shot.At.IsZero() {
		t.Error("shot.At is zero; the payload carried a ShotDateTime")
	}
	if got, want := shot.At.Format("2006-01-02 15:04:05.000"), "2026-07-30 19:15:04.250"; got != want {
		t.Errorf("shot.At = %s, want %s", got, want)
	}
	if shot.ReceivedAt.Before(before) {
		t.Errorf("shot.ReceivedAt = %v, want >= %v", shot.ReceivedAt, before)
	}
	started := ls.Snapshot()[0].StartedAt
	if started.IsZero() {
		t.Error("StartedAt is zero after the first shot")
	}
	if !started.Equal(shot.At) {
		t.Errorf("StartedAt = %v, want shot.At %v", started, shot.At)
	}
}

func TestApplyShotWithoutTimestampLeavesAtZero(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{X: 1, Y: 1, DecValue: 9.5, FullValue: 9})

	shot := ls.Snapshot()[0].Shots[0]
	if !shot.At.IsZero() {
		t.Errorf("shot.At = %v, want zero when the payload has no ShotDateTime", shot.At)
	}
	if shot.ReceivedAt.IsZero() {
		t.Error("shot.ReceivedAt should always be set")
	}
}

func TestApplyShotDisciplineFromDiscType(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{
		DecValue: 10.0, FullValue: 10, DiscType: "LG",
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuItemName: "10 Schuss"},
	})
	snap := ls.Snapshot()[0]
	if snap.DiscType != "LG" {
		t.Fatalf("DiscType = %q, want LG", snap.DiscType)
	}
	if snap.Discipline != "LG" {
		t.Fatalf("Discipline = %q, want LG (not shot-count MenuItemName)", snap.Discipline)
	}
}

func TestApplyShotDisciplinePrefersMenuPointName(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{
		DecValue: 9.0, FullValue: 9, DiscType: "KK",
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuPointName: "KK-Gewehr", MenuItemName: "10 Schuss"},
	})
	snap := ls.Snapshot()[0]
	if snap.Discipline != "KK-Gewehr" {
		t.Fatalf("Discipline = %q, want KK-Gewehr", snap.Discipline)
	}
	if snap.DiscType != "KK" {
		t.Fatalf("DiscType = %q, want KK", snap.DiscType)
	}
}

func TestApplyShotDisciplinePrefersConcreteMenuItemOverSportordnung(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{
		DecValue: 10.3, FullValue: 10, DiscType: "LG",
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuPointName: "Sportordnung", MenuItemName: "LG 30 Schuss Auflage"},
	})
	snap := ls.Snapshot()[0]
	if snap.Discipline != "LG 30 Schuss Auflage" {
		t.Fatalf("Discipline = %q, want LG 30 Schuss Auflage", snap.Discipline)
	}
}


func TestApplyShotReportsUnknownRange(t *testing.T) {
	ls := NewLiveState(2)
	if ls.ApplyShot(7, &ShotPayload{DecValue: 10.0, FullValue: 10}) {
		t.Fatal("ApplyShot accepted a range outside the configured count")
	}
	for _, rs := range ls.Snapshot() {
		if len(rs.Shots) != 0 {
			t.Fatalf("range %d recorded a shot from an unknown range", rs.RangeNum)
		}
	}
}

func TestSeriesSumsAreBounded(t *testing.T) {
	ls := NewLiveState(1)
	// Same shooter throughout, so nothing resets the footer: 10 shots per series.
	for i := 0; i < (maxSeriesSums+20)*10; i++ {
		ls.ApplyShot(1, &ShotPayload{DecValue: 10.0, FullValue: 10})
	}

	snap := ls.Snapshot()[0]
	if len(snap.SeriesSums) > maxSeriesSums {
		t.Errorf("SeriesSums grew to %d, cap is %d", len(snap.SeriesSums), maxSeriesSums)
	}
	if len(snap.SeriesSumsInt) > maxSeriesSums {
		t.Errorf("SeriesSumsInt grew to %d, cap is %d", len(snap.SeriesSumsInt), maxSeriesSums)
	}
	if len(snap.SeriesSums) != maxSeriesSums {
		t.Errorf("SeriesSums = %d, want the cap %d to be reached", len(snap.SeriesSums), maxSeriesSums)
	}
	if len(snap.SeriesShots) != maxSeriesSums {
		t.Errorf("SeriesShots = %d, want the cap %d", len(snap.SeriesShots), maxSeriesSums)
	}
	for i, series := range snap.SeriesShots {
		if len(series) != 10 {
			t.Errorf("SeriesShots[%d] len = %d, want 10", i, len(series))
		}
	}
	if len(snap.Last10Values) > 10 {
		t.Errorf("Last10Values = %d, want at most 10", len(snap.Last10Values))
	}
}

func TestSeriesShotsStoredOnComplete(t *testing.T) {
	ls := NewLiveState(1)
	for i := 0; i < 10; i++ {
		ls.ApplyShot(1, &ShotPayload{X: i, Y: -i, DecValue: 10.0 - float64(i)*0.1, FullValue: 10 - i/10})
	}
	snap := ls.Snapshot()[0]
	if len(snap.SeriesShots) != 1 {
		t.Fatalf("SeriesShots len = %d, want 1 after first completed series", len(snap.SeriesShots))
	}
	if len(snap.SeriesShots[0]) != 10 {
		t.Fatalf("first series shot count = %d, want 10", len(snap.SeriesShots[0]))
	}
	if snap.SeriesShots[0][0].X != 0 || snap.SeriesShots[0][9].X != 9 {
		t.Errorf("series shots not preserved in order: first X=%d last X=%d",
			snap.SeriesShots[0][0].X, snap.SeriesShots[0][9].X)
	}
	// 11th shot starts a new series on the target; archived series remains.
	ls.ApplyShot(1, &ShotPayload{X: 100, Y: 0, DecValue: 9.0, FullValue: 9})
	snap = ls.Snapshot()[0]
	if len(snap.SeriesShots) != 1 {
		t.Fatalf("SeriesShots len = %d after 11th shot, want 1", len(snap.SeriesShots))
	}
	if len(snap.Shots) != 1 || snap.Shots[0].X != 100 {
		t.Fatalf("live shots = %+v, want single shot X=100", snap.Shots)
	}
	if len(snap.SeriesSumsInt) != 2 || snap.SeriesSumsInt[1] != 9 {
		t.Fatalf("SeriesSumsInt after 11th = %v, want [completed, 9]", snap.SeriesSumsInt)
	}
}

func TestSeriesSumsFollowEachShot(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{DecValue: 10.5, FullValue: 10})
	snap := ls.Snapshot()[0]
	if len(snap.SeriesSumsInt) != 1 || snap.SeriesSumsInt[0] != 10 {
		t.Fatalf("after shot 1: SeriesSumsInt = %v, want [10]", snap.SeriesSumsInt)
	}
	if len(snap.SeriesSums) != 1 || snap.SeriesSums[0] != 10.5 {
		t.Fatalf("after shot 1: SeriesSums = %v, want [10.5]", snap.SeriesSums)
	}
	if len(snap.SeriesShots) != 0 {
		t.Fatalf("SeriesShots after shot 1 = %d, want 0 (series not complete)", len(snap.SeriesShots))
	}

	ls.ApplyShot(1, &ShotPayload{DecValue: 9.4, FullValue: 9})
	snap = ls.Snapshot()[0]
	if len(snap.SeriesSumsInt) != 1 || snap.SeriesSumsInt[0] != 19 {
		t.Fatalf("after shot 2: SeriesSumsInt = %v, want [19]", snap.SeriesSumsInt)
	}
	if len(snap.SeriesSums) != 1 || math.Abs(snap.SeriesSums[0]-19.9) > 1e-9 {
		t.Fatalf("after shot 2: SeriesSums = %v, want [19.9]", snap.SeriesSums)
	}

	for i := 0; i < 8; i++ {
		ls.ApplyShot(1, &ShotPayload{DecValue: 10.0, FullValue: 10})
	}
	snap = ls.Snapshot()[0]
	if len(snap.SeriesSumsInt) != 1 || snap.SeriesSumsInt[0] != 99 {
		t.Fatalf("after 10 shots: SeriesSumsInt = %v, want [99]", snap.SeriesSumsInt)
	}
	if len(snap.SeriesShots) != 1 {
		t.Fatalf("SeriesShots after 10 shots = %d, want 1", len(snap.SeriesShots))
	}

	ls.ApplyShot(1, &ShotPayload{DecValue: 8.1, FullValue: 8})
	snap = ls.Snapshot()[0]
	if len(snap.SeriesSumsInt) != 2 {
		t.Fatalf("after opening shot of series 2: %d columns, want 2", len(snap.SeriesSumsInt))
	}
	if snap.SeriesSumsInt[1] != 8 {
		t.Fatalf("new series started with %d, want 8", snap.SeriesSumsInt[1])
	}
	if math.Abs(snap.SeriesSums[1]-8.1) > 1e-9 {
		t.Fatalf("new series decimal = %v, want 8.1", snap.SeriesSums[1])
	}
}

func TestApplyShotConcurrentWithSnapshot(t *testing.T) {
	ls := NewLiveState(4)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			ls.ApplyShot(1+i%4, &ShotPayload{DecValue: 10.0, FullValue: 10})
		}
		close(done)
	}()
	for {
		select {
		case <-done:
			return
		default:
			for _, rs := range ls.Snapshot() {
				_ = rs.Shots
			}
		}
	}
}

func testShooter(first, last, club string) *struct {
	Firstname string `json:"Firstname"`
	Lastname  string `json:"Lastname"`
	Club      *struct {
		Name string `json:"Name"`
	} `json:"Club"`
} {
	s := &struct {
		Firstname string `json:"Firstname"`
		Lastname  string `json:"Lastname"`
		Club      *struct {
			Name string `json:"Name"`
		} `json:"Club"`
	}{Firstname: first, Lastname: last}
	if club != "" {
		s.Club = &struct {
			Name string `json:"Name"`
		}{Name: club}
	}
	return s
}

func TestStartedAt_ShooterChangeResets(t *testing.T) {
	ls := NewLiveState(1)
	t1 := time.Date(2026, 9, 7, 13, 45, 2, 0, time.Local)
	ls.ApplyShotAt(1, &ShotPayload{
		DecValue: 10, FullValue: 10,
		Shooter: testShooter("Anna", "Müller", "SV Adler"),
	}, t1, t1)
	t2 := t1.Add(5 * time.Minute)
	ls.ApplyShotAt(1, &ShotPayload{
		DecValue: 9, FullValue: 9,
		Shooter: testShooter("Jonas", "Becker", "KSG Mitte"),
	}, t2, t2)
	got := ls.Snapshot()[0]
	if !got.StartedAt.Equal(t2) {
		t.Fatalf("after shooter change StartedAt=%v, want %v", got.StartedAt, t2)
	}
}

func TestStartedAt_WarmupToCompKeepsStart(t *testing.T) {
	ls := NewLiveState(1)
	t1 := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	ls.ApplyShotAt(1, &ShotPayload{DecValue: 10, FullValue: 10, IsWarmup: true}, t1, t1)
	t2 := t1.Add(time.Minute)
	ls.ApplyShotAt(1, &ShotPayload{DecValue: 10, FullValue: 10, IsWarmup: false}, t2, t2)
	got := ls.Snapshot()[0]
	if !got.StartedAt.Equal(t1) {
		t.Fatalf("warmup→comp StartedAt=%v, want %v", got.StartedAt, t1)
	}
}
