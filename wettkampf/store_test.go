package wettkampf

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"srdashboard/state"
)

func wertungSnap(name, club, team string, rng, shot, total, sumInt int, sumDec float64) state.RangeSnapshot {
	predInt, predDec := 0, 0.0
	if shot > 0 && total > 0 {
		if shot >= total {
			predInt, predDec = sumInt, sumDec
		} else {
			predInt = int(float64(sumInt)/float64(shot)*float64(total) + 0.5)
			predDec = sumDec / float64(shot) * float64(total)
		}
	}
	return state.RangeSnapshot{
		RangeNum:         rng,
		ShooterName:      name,
		ClubName:         club,
		TeamName:         team,
		IsWarmup:         false,
		ShotNumber:       shot,
		OverallSumInt:    sumInt,
		OverallSumDec:    sumDec,
		PredictionInt:    predInt,
		PredictionDec:    predDec,
		TotalShotsToFire: total,
	}
}

func TestObserveUpsertAndTeamHint(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 10, 40, 98, 98.4))
	v := s.View()
	if len(v.Teams) != 1 || v.Teams[0].Name != "SV Adler" {
		t.Fatalf("teams = %#v", v.Teams)
	}
	if v.Teams[0].SumInt != 98 || v.Teams[0].Count != 1 {
		t.Fatalf("team totals = %#v", v.Teams[0])
	}
	if v.Teams[0].Club != "SV Adler" {
		t.Fatalf("team club = %q", v.Teams[0].Club)
	}
	if len(v.Roster) != 1 || v.Roster[0].TeamID != v.Teams[0].ID {
		t.Fatalf("roster = %#v", v.Roster)
	}
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 20, 40, 196, 196.8))
	v = s.View()
	if v.Teams[0].SumInt != 196 {
		t.Fatalf("upsert sum = %d", v.Teams[0].SumInt)
	}
}

func TestUDPTeamNamePreferredOverClub(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("Jonas Becker", "SV Adler", "Adler I", 2, 5, 40, 49, 49.2))
	v := s.View()
	if len(v.Teams) != 1 || v.Teams[0].Name != "Adler I" {
		t.Fatalf("teams = %#v", v.Teams)
	}
	if v.Teams[0].Club != "SV Adler" {
		t.Fatalf("club from members = %q", v.Teams[0].Club)
	}
}

func TestAuflageTeamUsesDecimal(t *testing.T) {
	s, _ := Open("")
	snap := wertungSnap("Anna Müller", "SV Adler", "Adler I", 1, 30, 30, 284, 294.5)
	snap.Discipline = "LG 30 Schuss Auflage"
	s.Observe(snap)
	v := s.View()
	if len(v.Teams) != 1 || !v.Teams[0].Decimal {
		t.Fatalf("auflage team decimal = %#v", v.Teams)
	}
	if v.Teams[0].SumDec != 294.5 {
		t.Fatalf("sumDec = %v", v.Teams[0].SumDec)
	}
	if v.Teams[0].Members[0].Discipline != "LG 30 Schuss Auflage" {
		t.Fatalf("member discipline = %#v", v.Teams[0].Members[0])
	}
}

func TestProbeDoesNotClobberWertung(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 40, 40, 387, 387.5))
	probe := wertungSnap("Anna Müller", "SV Adler", "", 3, 2, 40, 19, 19.1)
	probe.IsWarmup = true
	s.Observe(probe)
	v := s.View()
	if v.Teams[0].SumInt != 387 {
		t.Fatalf("probe overwrote wertung: %d", v.Teams[0].SumInt)
	}
	if !v.Roster[0].Locked {
		t.Fatal("complete program should lock")
	}
}

func TestLockedIgnoresLaterSession(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 40, 40, 387, 387.5))
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 4, 1, 40, 10, 10.2))
	v := s.View()
	if v.Teams[0].SumInt != 387 {
		t.Fatalf("locked overwrite = %d", v.Teams[0].SumInt)
	}
}

func TestEmptyObserveDoesNotDropRoster(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 10, 40, 98, 98.4))
	s.Observe(state.RangeSnapshot{RangeNum: 1})
	v := s.View()
	if len(v.Roster) != 1 {
		t.Fatalf("roster dropped on empty lane: %#v", v.Roster)
	}
}

func TestTeamSumAndPrognose(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("A One", "Heim", "", 1, 10, 40, 100, 100.0))
	s.Observe(wertungSnap("B Two", "Heim", "", 2, 20, 40, 200, 200.0))
	v := s.View()
	if len(v.Teams) != 1 {
		t.Fatalf("teams = %#v", v.Teams)
	}
	if v.Teams[0].SumInt != 300 {
		t.Fatalf("sumInt = %d", v.Teams[0].SumInt)
	}
	// A: (100/10)*40 = 400; B: (200/20)*40 = 400
	if v.Teams[0].PredInt != 800 {
		t.Fatalf("predInt = %d", v.Teams[0].PredInt)
	}
	if v.ExpectedPerTeam != 4 || v.Teams[0].Count != 2 {
		t.Fatalf("completeness %d/%d", v.Teams[0].Count, v.ExpectedPerTeam)
	}
}

func TestAssignByClubAndResetKeepsTeams(t *testing.T) {
	s, _ := Open("")
	s.Observe(wertungSnap("A One", "Heim", "", 1, 10, 40, 100, 100.0))
	s.Apply(Patch{Action: "addTeam", Name: "Gast"})
	unlocked := false
	s.Apply(Patch{Roster: []RosterPatch{{Key: s.View().Roster[0].Key, TeamID: "", Locked: &unlocked}}})
	v := s.Apply(Patch{Action: "assignByClub"})
	if v.Roster[0].TeamID == "" {
		t.Fatalf("not reassigned: %#v", v.Roster[0])
	}
	v = s.Apply(Patch{Reset: true})
	if len(v.Roster) != 0 {
		t.Fatalf("reset roster = %#v", v.Roster)
	}
	if len(v.Teams) == 0 {
		t.Fatal("reset dropped teams")
	}
}

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wettkampf.xml")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "", 1, 10, 40, 98, 98.4))
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v := s2.View()
	if len(v.Roster) != 1 || v.Roster[0].Name != "Anna Müller" || v.Teams[0].SumInt != 98 {
		t.Fatalf("reload = %#v", v)
	}
}

func TestOpenSessionStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wettkampf.xml")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Observe(wertungSnap("Anna Müller", "SV Adler", "Adler I", 1, 10, 40, 98, 98.4))
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	v := s2.View()
	if len(v.Roster) != 0 || len(v.Teams) != 0 {
		t.Fatalf("new session reused prior Wettkampf: %#v", v)
	}
}

func TestExcludeShooterFromTeamKeepsRoster(t *testing.T) {
	s, _ := Open("")
	three := 3
	s.Apply(Patch{ExpectedPerTeam: &three})
	club := "SV Adler"
	names := []string{"A One", "B Two", "C Three", "D Four", "E Five"}
	for i, name := range names {
		s.Observe(wertungSnap(name, club, "Adler I", i+1, 10, 40, 90+i, 90.1+float64(i)))
	}
	v := s.View()
	if len(v.Roster) != 5 || teamByNameView(v, "Adler I").Count != 5 {
		t.Fatalf("before exclude roster=%d count=%d", len(v.Roster), teamByNameView(v, "Adler I").Count)
	}
	yes := true
	d := rosterByNameView(v, "D Four")
	e := rosterByNameView(v, "E Five")
	s.Apply(Patch{Roster: []RosterPatch{
		{Key: d.Key, TeamID: d.TeamID, Excluded: &yes},
		{Key: e.Key, TeamID: e.TeamID, Excluded: &yes},
	}})
	v = s.View()
	adler := teamByNameView(v, "Adler I")
	if adler.Count != 3 {
		t.Fatalf("team count %d want 3 members=%#v", adler.Count, adler.Members)
	}
	if len(v.Roster) != 5 {
		t.Fatalf("excluded shooters must stay on roster: %d", len(v.Roster))
	}
	if !rosterByNameView(v, "D Four").Excluded || rosterByNameView(v, "D Four").TeamID != d.TeamID {
		t.Fatalf("D not excluded under team: %#v", rosterByNameView(v, "D Four"))
	}
	before := adler.SumInt
	s.Observe(wertungSnap("D Four", club, "Adler I", 4, 20, 40, 200, 200.0))
	v = s.View()
	if teamByNameView(v, "Adler I").SumInt != before {
		t.Fatalf("excluded D counted after later shots: %d vs %d", teamByNameView(v, "Adler I").SumInt, before)
	}
	if rosterByNameView(v, "D Four").SumInt != 200 {
		t.Fatalf("excluded D individual score lost: %#v", rosterByNameView(v, "D Four"))
	}
	v = s.Apply(Patch{Action: "assignByClub"})
	if !rosterByNameView(v, "E Five").Excluded || rosterByNameView(v, "E Five").TeamID != e.TeamID {
		t.Fatalf("assignByClub must not pull excluded back: %#v", rosterByNameView(v, "E Five"))
	}
	if teamByNameView(v, "Adler I").Count != 3 {
		t.Fatalf("after assignByClub count %d", teamByNameView(v, "Adler I").Count)
	}
}

func teamByNameView(v Snapshot, name string) TeamView {
	for _, t := range v.Teams {
		if t.Name == name {
			return t
		}
	}
	return TeamView{}
}

func rosterByNameView(v Snapshot, name string) RosterView {
	for _, r := range v.Roster {
		if r.Name == name {
			return r
		}
	}
	return RosterView{}
}

func TestShotPayloadUnmarshalsTeam(t *testing.T) {
	raw := []byte(`{
		"X":1,"Y":1,"Distance":1.4,"FullValue":10,"DecValue":10.2,
		"Shooter":{
			"Firstname":"Anna","Lastname":"Müller",
			"Club":{"Name":"SV Adler"},
			"Team":{"Name":"Adler I","ShortName":"AI"}
		}
	}`)
	var sp state.ShotPayload
	if err := json.Unmarshal(raw, &sp); err != nil {
		t.Fatal(err)
	}
	if sp.Shooter == nil || sp.Shooter.Team == nil || sp.Shooter.Team.Name != "Adler I" {
		t.Fatalf("team = %#v", sp.Shooter)
	}
	ls := state.NewLiveState(1)
	if !ls.ApplyShot(1, &sp) {
		t.Fatal("apply")
	}
	got := ls.Snapshot()[0]
	if got.TeamName != "Adler I" || got.ClubName != "SV Adler" {
		t.Fatalf("snapshot team=%q club=%q", got.TeamName, got.ClubName)
	}
}
