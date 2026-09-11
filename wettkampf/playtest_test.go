package wettkampf

import (
	"math"
	"testing"

	"srdashboard/state"
	"srdashboard/udp"
)

type simShooter struct {
	First, Last, Club, Team string
	Range                   int
	Skill                   float64 // typical DecValue
}

func (p simShooter) name() string {
	return p.First + " " + p.Last
}

func payload(p simShooter, warmup bool, dec float64, seed int, menu, disc string) *state.ShotPayload {
	x, y, dist := udp.PlaceShotForDisc(disc, dec, seed)
	sp := &state.ShotPayload{
		X: x, Y: y, Distance: dist,
		FullValue: udp.FullValueOf(dec),
		DecValue:  dec,
		Range:     p.Range,
		IsWarmup:  warmup,
		DiscType:  disc,
		Shooter: &state.ShotShooter{
			Firstname: p.First,
			Lastname:  p.Last,
			Club:      &state.ShotClub{Name: p.Club},
			Team:      &state.ShotTeam{Name: p.Team},
		},
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuItemName: menu},
	}
	return sp
}

func fireShot(ls *state.LiveState, wk *Store, sp *state.ShotPayload) {
	if !ls.ApplyShot(sp.Range, sp) {
		panic("apply shot")
	}
	snap, ok := ls.RangeSnapshot(sp.Range)
	if !ok {
		panic("snapshot")
	}
	wk.Observe(snap)
}

type laneSpec struct {
	p               simShooter
	warmup, wertung int
	menu, disc      string
}

func spec(p simShooter, warmup, wertung int, menu, disc string) laneSpec {
	return laneSpec{p: p, warmup: warmup, wertung: wertung, menu: menu, disc: disc}
}

type shotSum struct {
	Int int
	Dec float64
}

// fireHall round-robins one shot per occupied Bahn so the hall fires together,
// not 40 on Bahn 1 then 40 on Bahn 2. Shorter programs drop out when finished.
func fireHall(ls *state.LiveState, wk *Store, specs []laneSpec) (map[string]shotSum, []int) {
	type run struct {
		laneSpec
		warmLeft, wertLeft int
		warmIdx, wertIdx   int
		sumInt             int
		sumDec             float64
	}
	runs := make([]run, len(specs))
	for i, s := range specs {
		runs[i] = run{laneSpec: s, warmLeft: s.warmup, wertLeft: s.wertung}
	}
	order := make([]int, 0)
	start := 0
	for {
		progressed := false
		n := len(runs)
		for k := 0; k < n; k++ {
			r := &runs[(start+k)%n]
			seed := r.p.Range * 100
			var sp *state.ShotPayload
			if r.warmLeft > 0 {
				sp = payload(r.p, true, clampDec(r.p.Skill-0.2), seed+r.warmIdx, r.menu, r.disc)
				r.warmIdx++
				r.warmLeft--
			} else if r.wertLeft > 0 {
				sp = payload(r.p, false, clampDec(r.p.Skill-0.1*float64(r.wertIdx%7)), seed+50+r.wertIdx, r.menu, r.disc)
				r.sumInt += sp.FullValue
				r.sumDec += sp.DecValue
				r.wertIdx++
				r.wertLeft--
			} else {
				continue
			}
			fireShot(ls, wk, sp)
			order = append(order, r.p.Range)
			progressed = true
		}
		if !progressed {
			break
		}
		start++
	}
	sums := make(map[string]shotSum, len(runs))
	for _, r := range runs {
		sums[r.p.name()] = shotSum{Int: r.sumInt, Dec: r.sumDec}
	}
	return sums, order
}

func firstVolleyCoversLanes(order []int, n int) bool {
	if n <= 0 || len(order) < n {
		return false
	}
	seen := make(map[int]bool, n)
	for _, rng := range order[:n] {
		if seen[rng] {
			return false
		}
		seen[rng] = true
	}
	return len(seen) == n
}

func fireProgram(ls *state.LiveState, wk *Store, p simShooter, warmupN, wertungN int, menu, disc string) (sumInt int, sumDec float64) {
	sums, _ := fireHall(ls, wk, []laneSpec{spec(p, warmupN, wertungN, menu, disc)})
	s := sums[p.name()]
	return s.Int, s.Dec
}

func clampDec(d float64) float64 {
	if d > 10.9 {
		return 10.9
	}
	if d < 8.0 {
		return 8.0
	}
	return math.Round(d*10) / 10
}

func pred(sumInt, n, total int, sumDec float64) (int, float64) {
	if n == 0 || total == 0 {
		return 0, 0
	}
	if n >= total {
		return sumInt, sumDec
	}
	return int(math.Round(float64(sumInt) / float64(n) * float64(total))), (sumDec / float64(n)) * float64(total)
}

func teamByName(v Snapshot, name string) TeamView {
	for _, t := range v.Teams {
		if t.Name == name {
			return t
		}
	}
	return TeamView{}
}

func rosterByName(v Snapshot, name string) RosterView {
	for _, r := range v.Roster {
		if r.Name == name {
			return r
		}
	}
	return RosterView{}
}

func TestDualMeetSixLanesFullPrograms(t *testing.T) {
	ls := state.NewLiveState(6)
	wk, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	three := 3
	wk.Apply(Patch{ExpectedPerTeam: &three})

	adler := []simShooter{
		{"Anna", "Müller", "SV Adler", "Adler I", 1, 10.2},
		{"Peter", "Klein", "SV Adler", "Adler I", 2, 9.7},
		{"Lisa", "Wolf", "SV Adler", "Adler I", 3, 9.3},
	}
	mitte := []simShooter{
		{"Jonas", "Becker", "KSG Mitte", "Mitte I", 4, 10.0},
		{"Sara", "Lang", "KSG Mitte", "Mitte I", 5, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Mitte I", 6, 9.1},
	}

	lg := "LG 40 Schuss"
	hall := make([]laneSpec, 0, 6)
	for _, p := range adler {
		hall = append(hall, spec(p, 5, 40, lg, "LG"))
	}
	for _, p := range mitte {
		hall = append(hall, spec(p, 5, 40, lg, "LG"))
	}
	sums, order := fireHall(ls, wk, hall)
	if !firstVolleyCoversLanes(order, 6) {
		t.Fatalf("first volley must hit all 6 Bahnen, got %v", order[:min(12, len(order))])
	}
	wantAdlerInt, wantAdlerDec := 0, 0.0
	wantMitteInt, wantMitteDec := 0, 0.0
	wantAdlerPred, wantMittePred := 0, 0
	for _, p := range adler {
		s := sums[p.name()]
		wantAdlerInt += s.Int
		wantAdlerDec += s.Dec
		pi, _ := pred(s.Int, 40, 40, s.Dec)
		wantAdlerPred += pi
	}
	for _, p := range mitte {
		s := sums[p.name()]
		wantMitteInt += s.Int
		wantMitteDec += s.Dec
		pi, _ := pred(s.Int, 40, 40, s.Dec)
		wantMittePred += pi
	}

	v := wk.View()
	if len(v.Roster) != 6 {
		t.Fatalf("roster %d want 6: %#v", len(v.Roster), v.Roster)
	}
	if len(v.Teams) != 2 {
		t.Fatalf("teams %d want 2: %#v", len(v.Teams), v.Teams)
	}
	a := teamByName(v, "Adler I")
	m := teamByName(v, "Mitte I")
	if a.Count != 3 || m.Count != 3 || a.Expected != 3 {
		t.Fatalf("completeness Adler %d/%d Mitte %d/%d", a.Count, a.Expected, m.Count, m.Expected)
	}
	if len(a.Members) != 3 || len(m.Members) != 3 {
		t.Fatalf("members Adler %d Mitte %d", len(a.Members), len(m.Members))
	}
	if a.SumInt != wantAdlerInt || m.SumInt != wantMitteInt {
		t.Fatalf("sums Adler %d want %d; Mitte %d want %d", a.SumInt, wantAdlerInt, m.SumInt, wantMitteInt)
	}
	if math.Abs(a.SumDec-wantAdlerDec) > 0.15 || math.Abs(m.SumDec-wantMitteDec) > 0.15 {
		t.Fatalf("dec Adler %v want %v; Mitte %v want %v", a.SumDec, wantAdlerDec, m.SumDec, wantMitteDec)
	}
	if a.PredInt != wantAdlerPred || m.PredInt != wantMittePred {
		t.Fatalf("pred Adler %d want %d; Mitte %d want %d", a.PredInt, wantAdlerPred, m.PredInt, wantMittePred)
	}
	for _, r := range v.Roster {
		if !r.Locked || r.ShotsFired != 40 || r.TotalShots != 40 {
			t.Fatalf("not finished: %#v", r)
		}
	}

	for rng := 1; rng <= 6; rng++ {
		if !ls.ResetRange(rng) {
			t.Fatalf("reset %d", rng)
		}
	}
	v = wk.View()
	a = teamByName(v, "Adler I")
	if a.SumInt != wantAdlerInt || a.Count != 3 {
		t.Fatalf("ResetRange wiped Wettkampf: %#v", a)
	}

	probe := payload(adler[0], true, 9.0, 999, "LG 40 Schuss", "LG")
	fireShot(ls, wk, probe)
	v = wk.View()
	if teamByName(v, "Adler I").SumInt != wantAdlerInt {
		t.Fatal("probe after finish overwrote locked result")
	}
}

func TestFourTeamAsyncTwelveShootersAndRearrange(t *testing.T) {
	ls := state.NewLiveState(6)
	wk, _ := Open("")
	three := 3
	wk.Apply(Patch{ExpectedPerTeam: &three})

	// 4 Mannschaften × 3 = 12 Schützen. Only 6 Bahnen, so two waves.
	wave1 := []simShooter{
		{"Anna", "Müller", "SV Adler", "Adler I", 1, 10.1},
		{"Peter", "Klein", "SV Adler", "Adler I", 2, 9.6},
		{"Jonas", "Becker", "KSG Mitte", "Mitte I", 3, 9.9},
		{"Uwe", "Hart", "SV West", "West II", 4, 9.4},
		{"Ina", "Berg", "SV West", "West II", 5, 9.8},
		{"Otto", "See", "SG Ost", "Ost I", 6, 9.2},
	}
	wave2 := []simShooter{
		{"Lisa", "Wolf", "SV Adler", "Adler I", 1, 9.3},
		{"Sara", "Lang", "KSG Mitte", "Mitte I", 2, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Mitte I", 3, 9.0},
		{"Mia", "Feld", "SV West", "West II", 4, 10.0},
		{"Eva", "Horn", "SG Ost", "Ost I", 5, 9.7},
		{"Jan", "Moos", "SG Ost", "Ost I", 6, 9.1},
	}

	lg := "LG 40 Schuss"
	j := wave1[2]
	o := wave1[5]
	w1, order := fireHall(ls, wk, []laneSpec{
		spec(wave1[0], 5, 40, lg, "LG"),
		spec(wave1[1], 5, 40, lg, "LG"),
		spec(j, 3, 12, lg, "LG"),
		spec(wave1[3], 5, 40, lg, "LG"),
		spec(wave1[4], 5, 40, lg, "LG"),
		spec(o, 2, 20, lg, "LG"),
	})
	if !firstVolleyCoversLanes(order, 6) {
		t.Fatalf("wave1 first volley must hit all 6 Bahnen, got %v", order[:min(12, len(order))])
	}
	if !ls.ResetRange(j.Range) {
		t.Fatal("clear Jonas")
	}
	v := wk.View()
	jr := rosterByName(v, j.name())
	js, jd := w1[j.name()].Int, w1[j.name()].Dec
	if jr.Locked || jr.ShotsFired != 12 || jr.SumInt != js {
		t.Fatalf("partial after clear: %#v want sum %d", jr, js)
	}
	jp, _ := pred(js, 12, 40, jd)
	if jr.PredInt != jp {
		t.Fatalf("partial prognose %d want %d", jr.PredInt, jp)
	}
	os := w1[o.name()].Int
	if rosterByName(wk.View(), o.name()).ShotsFired != 20 {
		t.Fatalf("Otto shots %d", rosterByName(wk.View(), o.name()).ShotsFired)
	}

	// Lock Otto's partial so a later session cannot clobber it.
	locked := true
	wk.Apply(Patch{Roster: []RosterPatch{{Key: rosterByName(wk.View(), o.name()).Key, TeamID: rosterByName(wk.View(), o.name()).TeamID, Locked: &locked}}})

	// Clear hall for wave 2 (async: next 6 come in).
	for rng := 1; rng <= 6; rng++ {
		ls.ResetRange(rng)
	}
	v = wk.View()
	if teamByName(v, "Adler I").Count != 2 {
		t.Fatalf("Adler after wave1 want 2 finished, got %d", teamByName(v, "Adler I").Count)
	}
	if teamByName(v, "Mitte I").Count != 1 || teamByName(v, "West II").Count != 2 || teamByName(v, "Ost I").Count != 1 {
		t.Fatalf("wave1 counts %#v", v.Teams)
	}
	if teamByName(v, "Adler I").SumInt == 0 || teamByName(v, "West II").SumInt == 0 {
		t.Fatal("wave1 totals lost after hall clear")
	}

	// Wave 2: remaining 6 shoot full programs on the same 6 Bahnen.
	w2specs := make([]laneSpec, 0, len(wave2))
	for _, p := range wave2 {
		w2specs = append(w2specs, spec(p, 5, 40, lg, "LG"))
	}
	_, order2 := fireHall(ls, wk, w2specs)
	if !firstVolleyCoversLanes(order2, 6) {
		t.Fatalf("wave2 first volley must hit all 6 Bahnen, got %v", order2[:min(12, len(order2))])
	}

	v = wk.View()
	if len(v.Roster) != 12 {
		t.Fatalf("roster %d want 12", len(v.Roster))
	}
	if len(v.Teams) != 4 {
		t.Fatalf("teams %d want 4: %#v", len(v.Teams), v.Teams)
	}
	if teamByName(v, "Adler I").Count != 3 || teamByName(v, "Mitte I").Count != 3 ||
		teamByName(v, "West II").Count != 3 || teamByName(v, "Ost I").Count != 3 {
		t.Fatalf("after wave2 counts A%d M%d W%d O%d",
			teamByName(v, "Adler I").Count, teamByName(v, "Mitte I").Count,
			teamByName(v, "West II").Count, teamByName(v, "Ost I").Count)
	}

	// Unlocked Jonas still sitting at 12; locked Otto still 20; everyone else 40.
	if rosterByName(v, j.name()).ShotsFired != 12 || rosterByName(v, j.name()).Locked {
		t.Fatalf("Jonas should stay partial unlocked: %#v", rosterByName(v, j.name()))
	}
	if rosterByName(v, o.name()).ShotsFired != 20 || !rosterByName(v, o.name()).Locked {
		t.Fatalf("Otto lock failed: %#v", rosterByName(v, o.name()))
	}

	// Same person on a new Bahn after lock: must not overwrite.
	o.Range = 1
	fireProgram(ls, wk, o, 0, 5, "LG 40 Schuss", "LG")
	if rosterByName(wk.View(), o.name()).SumInt != os {
		t.Fatalf("locked Otto overwritten: %#v", rosterByName(wk.View(), o.name()))
	}

	// Rearrange: move finished Lisa Wolf (Adler) to Ost, move partial Jonas to West.
	v = wk.View()
	lisa := rosterByName(v, "Lisa Wolf")
	jonas := rosterByName(v, j.name())
	ostID := teamByName(v, "Ost I").ID
	westID := teamByName(v, "West II").ID
	adlerBefore := teamByName(v, "Adler I").SumInt
	ostBefore := teamByName(v, "Ost I").SumInt
	westBefore := teamByName(v, "West II").SumInt
	lisaSum := lisa.SumInt
	jonasSum := jonas.SumInt

	wk.Apply(Patch{Roster: []RosterPatch{
		{Key: lisa.Key, TeamID: ostID, Locked: &locked},
		{Key: jonas.Key, TeamID: westID},
	}})
	v = wk.View()
	if teamByName(v, "Adler I").Count != 2 {
		t.Fatalf("Adler after move Lisa: %d", teamByName(v, "Adler I").Count)
	}
	if teamByName(v, "Ost I").Count != 4 {
		t.Fatalf("Ost after Lisa: %d", teamByName(v, "Ost I").Count)
	}
	if teamByName(v, "Adler I").SumInt != adlerBefore-lisaSum {
		t.Fatalf("Adler sum after rearrange %d want %d", teamByName(v, "Adler I").SumInt, adlerBefore-lisaSum)
	}
	if teamByName(v, "Ost I").SumInt != ostBefore+lisaSum {
		t.Fatalf("Ost sum after rearrange %d want %d", teamByName(v, "Ost I").SumInt, ostBefore+lisaSum)
	}
	if teamByName(v, "West II").SumInt != westBefore+jonasSum {
		t.Fatalf("West sum after Jonas move %d want %d", teamByName(v, "West II").SumInt, westBefore+jonasSum)
	}

	// Unlocked continuation overwrites a partial (Jonas finishes on Bahn 6).
	j.Range = 6
	beforeJonas := rosterByName(wk.View(), j.name()).SumInt
	si, _ := fireProgram(ls, wk, j, 0, 40, "LG 40 Schuss", "LG")
	after := rosterByName(wk.View(), j.name())
	if after.SumInt == beforeJonas || after.ShotsFired != 40 || !after.Locked {
		t.Fatalf("unlocked reshoot should replace partial: before %d after %#v new %d", beforeJonas, after, si)
	}

	// Competition 2: reset roster, keep team names, LP 40 on all 6 lanes (2 teams of 3).
	v = wk.Apply(Patch{Reset: true})
	if len(v.Roster) != 0 {
		t.Fatalf("reset roster %#v", v.Roster)
	}
	if len(v.Teams) < 4 {
		t.Fatalf("reset dropped teams: %#v", v.Teams)
	}
	lp := []simShooter{
		{"Carla", "Pist", "SV Adler", "Adler I", 1, 9.8},
		{"Ben", "Pist", "SV Adler", "Adler I", 2, 9.4},
		{"Nia", "Pist", "SV Adler", "Adler I", 3, 10.1},
		{"Omar", "Pist", "KSG Mitte", "Mitte I", 4, 9.6},
		{"Pia", "Pist", "KSG Mitte", "Mitte I", 5, 9.2},
		{"Rico", "Pist", "KSG Mitte", "Mitte I", 6, 9.9},
	}
	for rng := 1; rng <= 6; rng++ {
		ls.ResetRange(rng)
	}
	lpSpecs := make([]laneSpec, 0, len(lp))
	for _, p := range lp {
		lpSpecs = append(lpSpecs, spec(p, 5, 40, "LP 40 Schuss", "LP"))
	}
	_, lpOrder := fireHall(ls, wk, lpSpecs)
	if !firstVolleyCoversLanes(lpOrder, 6) {
		t.Fatalf("LP first volley must hit all 6 Bahnen, got %v", lpOrder[:min(12, len(lpOrder))])
	}
	v = wk.View()
	if len(v.Roster) != 6 {
		t.Fatalf("LP competition roster %d", len(v.Roster))
	}
	if teamByName(v, "Adler I").Count != 3 || teamByName(v, "Mitte I").Count != 3 {
		t.Fatalf("LP counts A%d M%d", teamByName(v, "Adler I").Count, teamByName(v, "Mitte I").Count)
	}
	if teamByName(v, "West II").Count != 0 || teamByName(v, "Ost I").Count != 0 {
		t.Fatalf("idle teams should stay at 0 after reset: W%d O%d", teamByName(v, "West II").Count, teamByName(v, "Ost I").Count)
	}
	for _, r := range v.Roster {
		if r.Discipline != "LP 40 Schuss" || !r.Locked || r.ShotsFired != 40 {
			t.Fatalf("LP program %#v", r)
		}
	}

	// Nach Verein zuordnen after a manual mix: park everyone on Ost, then re-hint.
	v = wk.View()
	ostID = teamByName(v, "Ost I").ID
	patches := make([]RosterPatch, 0, len(v.Roster))
	unlocked := false
	for _, r := range v.Roster {
		patches = append(patches, RosterPatch{Key: r.Key, TeamID: ostID, Locked: &unlocked})
	}
	wk.Apply(Patch{Roster: patches})
	if teamByName(wk.View(), "Ost I").Count != 6 {
		t.Fatalf("all parked on Ost: %d", teamByName(wk.View(), "Ost I").Count)
	}
	v = wk.Apply(Patch{Action: "assignByClub"})
	if teamByName(v, "Adler I").Count != 3 || teamByName(v, "Mitte I").Count != 3 {
		t.Fatalf("assignByClub A%d M%d", teamByName(v, "Adler I").Count, teamByName(v, "Mitte I").Count)
	}
}
