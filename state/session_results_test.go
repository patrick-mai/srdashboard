package state

import (
	"testing"
	"time"
)

func TestSessionArchive_ShooterChangeKeepsOldResult(t *testing.T) {
	ls := NewLiveState(1)
	t1 := time.Date(2026, 9, 10, 20, 1, 0, 0, time.Local)
	ls.ApplyShotAt(1, &ShotPayload{
		DecValue: 10.4, FullValue: 10,
		Shooter: testShooter("Anna", "Müller", "SV Adler"),
	}, t1, t1)
	ls.ApplyShotAt(1, &ShotPayload{
		DecValue: 9.2, FullValue: 9,
		Shooter: testShooter("Anna", "Müller", "SV Adler"),
	}, t1.Add(time.Second), t1.Add(time.Second))

	t2 := t1.Add(5 * time.Minute)
	ls.ApplyShotAt(1, &ShotPayload{
		DecValue: 8.0, FullValue: 8,
		Shooter: testShooter("Jonas", "Becker", "KSG Mitte"),
	}, t2, t2)

	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("results = %d, want 2 (live Jonas + archived Anna)", len(list))
	}
	if !list[0].Live || list[0].ShooterName != "Jonas Becker" {
		t.Fatalf("live row = %+v", list[0])
	}
	if list[1].Live || list[1].ShooterName != "Anna Müller" {
		t.Fatalf("archived row = %+v", list[1])
	}
	if list[1].ShotNumber != 2 {
		t.Fatalf("archived shotNumber = %d, want 2", list[1].ShotNumber)
	}

	_, snap, ok := ls.SessionResult(list[1].ID)
	if !ok {
		t.Fatal("archived id missing")
	}
	if snap.ShooterName != "Anna Müller" || snap.ShotNumber != 2 {
		t.Fatalf("archived snap = %+v", snap)
	}
	live := ls.Snapshot()[0]
	if live.ShooterName != "Jonas Becker" || live.ShotNumber != 1 {
		t.Fatalf("live after change = %+v", live)
	}
}

func TestSessionArchive_WarmupToCompDoesNotArchive(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{DecValue: 10, FullValue: 10, IsWarmup: true, Shooter: testShooter("Anna", "Müller", "")})
	ls.ApplyShot(1, &ShotPayload{DecValue: 10, FullValue: 10, IsWarmup: false, Shooter: testShooter("Anna", "Müller", "")})
	list := ls.SessionResults()
	if len(list) != 1 || !list[0].Live {
		t.Fatalf("warmup→comp should keep one live result, got %+v", list)
	}
}

func TestSessionArchive_CompToWarmupArchives(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{DecValue: 10, FullValue: 10, IsWarmup: false, Shooter: testShooter("Anna", "Müller", "")})
	ls.ApplyShot(1, &ShotPayload{DecValue: 9, FullValue: 9, IsWarmup: true, Shooter: testShooter("Anna", "Müller", "")})
	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("comp→warmup should archive Wertung, got %+v", list)
	}
	if !list[0].Live || !list[0].IsWarmup {
		t.Fatalf("live probe = %+v", list[0])
	}
	if list[1].Live || list[1].IsWarmup || list[1].ShotNumber != 1 {
		t.Fatalf("archived wertung = %+v", list[1])
	}
}

func TestSessionArchive_ResetRangeKeepsResult(t *testing.T) {
	ls := NewLiveState(1)
	ls.ApplyShot(1, &ShotPayload{DecValue: 10.1, FullValue: 10, Shooter: testShooter("Anna", "Müller", "")})
	if !ls.ResetRange(1) {
		t.Fatal("ResetRange")
	}
	list := ls.SessionResults()
	if len(list) != 1 || list[0].Live || list[0].ShooterName != "Anna Müller" {
		t.Fatalf("reset should leave archived Anna, got %+v", list)
	}
	live := ls.Snapshot()[0]
	if live.ShotNumber != 0 || live.ShooterName != "" {
		t.Fatalf("live after reset = %+v", live)
	}
}

func TestSessionArchive_SameShooterNewStartKeepsOwnID(t *testing.T) {
	ls := NewLiveState(1)
	sh := testShooter("Anna", "Müller", "")
	menu := &struct {
		MenuPointName string `json:"MenuPointName"`
		MenuItemName  string `json:"MenuItemName"`
	}{MenuItemName: "LG 2 Schuss"}
	ls.ApplyShot(1, &ShotPayload{DecValue: 10.1, FullValue: 10, Shooter: sh, MenuItem: menu})
	ls.ApplyShot(1, &ShotPayload{DecValue: 9.2, FullValue: 9, Shooter: sh, MenuItem: menu})
	first := ls.SessionResults()
	if len(first) != 1 || !first[0].Live || first[0].ShotNumber != 2 {
		t.Fatalf("first start = %+v", first)
	}
	oldID := first[0].ID

	ls.ApplyShot(1, &ShotPayload{DecValue: 8.0, FullValue: 8, Shooter: sh, MenuItem: menu})
	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("same shooter new start should archive, got %+v", list)
	}
	if !list[0].Live || list[0].ID == oldID || list[0].ShotNumber != 1 || list[0].ShooterName != "Anna Müller" {
		t.Fatalf("new live = %+v (oldID %s)", list[0], oldID)
	}
	if list[1].Live || list[1].ID != oldID || list[1].ShotNumber != 2 {
		t.Fatalf("archived first start = %+v, want id %s", list[1], oldID)
	}
	_, snap, ok := ls.SessionResult(oldID)
	if !ok || snap.ShotNumber != 2 || snap.ShooterName != "Anna Müller" {
		t.Fatalf("lookup old id = ok=%v snap=%+v", ok, snap)
	}
}

func TestSessionArchive_SameShooterAfterResetNewID(t *testing.T) {
	ls := NewLiveState(1)
	sh := testShooter("Anna", "Müller", "")
	ls.ApplyShot(1, &ShotPayload{DecValue: 10.4, FullValue: 10, Shooter: sh})
	oldID := ls.SessionResults()[0].ID
	if !ls.ResetRange(1) {
		t.Fatal("ResetRange")
	}
	ls.ApplyShot(1, &ShotPayload{DecValue: 9.0, FullValue: 9, Shooter: sh})
	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("reset then same shooter: %+v", list)
	}
	if !list[0].Live || list[0].ID == oldID || list[0].ShotNumber != 1 {
		t.Fatalf("new live after reset = %+v (old %s)", list[0], oldID)
	}
	if list[1].ID != oldID || list[1].Live || list[1].ShotNumber != 1 {
		t.Fatalf("archived after reset = %+v, want %s", list[1], oldID)
	}
	info, _, ok := ls.SessionResult("live-1")
	if !ok || !info.Live || info.ID == oldID {
		t.Fatalf("live-1 alias should be the new start, got %+v", info)
	}
}

func TestSessionArchive_UnbegrenztHundredThenNewStart(t *testing.T) {
	ls := NewLiveState(1)
	sh := testShooter("Anna", "Müller", "")
	menu := menuItem("Luftgewehr", "unbegrenzt")
	applyNamedShots(ls, unbegrenztDisplayShots, sh, menu)
	first := ls.SessionResults()
	if len(first) != 1 || !first[0].Live || first[0].ShotNumber != unbegrenztDisplayShots {
		t.Fatalf("100 unbegrenzt = %+v", first)
	}
	if first[0].TotalShotsToFire != unbegrenztDisplayShots {
		t.Fatalf("totalShotsToFire = %d, want %d", first[0].TotalShotsToFire, unbegrenztDisplayShots)
	}
	oldID := first[0].ID

	ls.ApplyShot(1, &ShotPayload{DecValue: 9.0, FullValue: 9, Shooter: sh, MenuItem: menu})
	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("101st unbegrenzt shot should start a new session, got %+v", list)
	}
	if !list[0].Live || list[0].ID == oldID || list[0].ShotNumber != 1 {
		t.Fatalf("live after 101 = %+v (old %s)", list[0], oldID)
	}
	if list[1].Live || list[1].ID != oldID || list[1].ShotNumber != unbegrenztDisplayShots {
		t.Fatalf("archived 100 = %+v, want id %s", list[1], oldID)
	}
	live := ls.Snapshot()[0]
	if live.ShotNumber != 1 || live.ResultID == oldID {
		t.Fatalf("hall after 101 = %+v", live)
	}
}

func TestSessionArchive_UnfinishedSameProgramKeepsOneID(t *testing.T) {
	ls := NewLiveState(1)
	sh := testShooter("Anna", "Müller", "")
	menu := menuItem("Sportordnung", "LG 40 Schuss")
	applyNamedShots(ls, 12, sh, menu)
	oldID := ls.SessionResults()[0].ID
	ls.ApplyShot(1, &ShotPayload{DecValue: 8.0, FullValue: 8, Shooter: sh, MenuItem: menu})
	list := ls.SessionResults()
	if len(list) != 1 || !list[0].Live || list[0].ID != oldID || list[0].ShotNumber != 13 {
		t.Fatalf("unfinished LG 40 must stay one start, got %+v (old %s)", list, oldID)
	}
}

func TestSessionArchive_ProgramLengthChangeNewStart(t *testing.T) {
	ls := NewLiveState(1)
	sh := testShooter("Anna", "Müller", "")
	applyNamedShots(ls, 5, sh, menuItem("Sportordnung", "LG 20 Schuss"))
	oldID := ls.SessionResults()[0].ID
	ls.ApplyShot(1, &ShotPayload{
		DecValue: 8.0, FullValue: 8, Shooter: sh,
		MenuItem: menuItem("Sportordnung", "LG 40 Schuss"),
	})
	list := ls.SessionResults()
	if len(list) != 2 {
		t.Fatalf("20→40 should archive the short start, got %+v", list)
	}
	if !list[0].Live || list[0].ID == oldID || list[0].ShotNumber != 1 || list[0].TotalShotsToFire != 40 {
		t.Fatalf("live LG 40 = %+v (old %s)", list[0], oldID)
	}
	if list[1].ID != oldID || list[1].Live || list[1].ShotNumber != 5 {
		t.Fatalf("archived LG 20 = %+v, want id %s", list[1], oldID)
	}
}

func applyNamedShots(ls *LiveState, n int, sh *ShotShooter, menu *struct {
	MenuPointName string `json:"MenuPointName"`
	MenuItemName  string `json:"MenuItemName"`
}) {
	for i := 0; i < n; i++ {
		ls.ApplyShot(1, &ShotPayload{DecValue: 10.0, FullValue: 10, Shooter: sh, MenuItem: menu})
	}
}

func TestSessionResult_LiveID(t *testing.T) {
	ls := NewLiveState(2)
	ls.ApplyShot(2, &ShotPayload{DecValue: 9.5, FullValue: 9, Shooter: testShooter("A", "B", "")})
	info, snap, ok := ls.SessionResult("live-2")
	if !ok || !info.Live || snap.RangeNum != 2 {
		t.Fatalf("live-2 = ok=%v info=%+v snap.range=%d", ok, info, snap.RangeNum)
	}
	if _, _, ok := ls.SessionResult("live-1"); ok {
		t.Fatal("empty live-1 should be missing")
	}
	if _, _, ok := ls.SessionResult("999"); ok {
		t.Fatal("unknown archive id")
	}
}
