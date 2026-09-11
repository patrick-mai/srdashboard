// sim-wettkampf-eval fires full LG 40 programs for hall Wettkampf playtests.
//
//	go run ./cmd/sim-wettkampf-eval -http http://127.0.0.1:8080
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"srdashboard/udp"
)

type shooter struct {
	First, Last, Club, Team string
	Range                   int
	Skill                   float64
}

func main() {
	httpBase := flag.String("http", "http://127.0.0.1:8080", "dashboard HTTP")
	udpAddr := flag.String("udp", "127.0.0.1:30169", "dashboard UDP")
	pause := flag.Duration("pause", 20*time.Millisecond, "delay between shots")
	scenario := flag.String("scenario", "full", "full, club5, or fourteam (14 shooters / 4 Mannschaften)")
	kind := flag.String("program", "LG40", "club5 program: LG40 or LGA30")
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	must(postJSON(*httpBase+"/api/plugins/activate", map[string]any{"id": "classic-range-condensed"}))
	if *scenario == "club5" {
		menu, disc, n := "LG 40 Schuss", "LG", 40
		if strings.EqualFold(strings.TrimSpace(*kind), "LGA30") {
			menu, n = "LG 30 Schuss Auflage", 30
		}
		runClub5(*httpBase, *udpAddr, *pause, menu, disc, n)
		return
	}
	if *scenario == "fourteam" {
		runFourTeam(*httpBase, *udpAddr, *pause)
		return
	}
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"reset": true}))
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"expectedPerTeam": 3}))
	clearHall(*httpBase)

	log.Println("=== dual meet: 6 Bahnen, Adler I vs Mitte I, 5 Probe + 40 Wertung ===")
	adler := []shooter{
		{"Anna", "Müller", "SV Adler", "Adler I", 1, 10.2},
		{"Peter", "Klein", "SV Adler", "Adler I", 2, 9.7},
		{"Lisa", "Wolf", "SV Adler", "Adler I", 3, 9.3},
	}
	mitte := []shooter{
		{"Jonas", "Becker", "KSG Mitte", "Mitte I", 4, 10.0},
		{"Sara", "Lang", "KSG Mitte", "Mitte I", 5, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Mitte I", 6, 9.1},
	}
	fireHall(*httpBase, *udpAddr, programsLG(append(append([]shooter{}, adler...), mitte...), 5, 40), *pause)
	dump(*httpBase, "after dual meet")
	requireTeamCount(*httpBase, "Adler I", 3)
	requireTeamCount(*httpBase, "Mitte I", 3)
	for _, name := range []string{"Anna Müller", "Peter Klein", "Lisa Wolf", "Jonas Becker", "Sara Lang", "Tim Koch"} {
		requireShots(*httpBase, name, 40, true)
	}
	adlerSum := team(getJSON(*httpBase+"/api/wettkampf"), "Adler I")["sumInt"]
	clearHall(*httpBase)
	dump(*httpBase, "after Bahn clear (results must remain)")
	if team(getJSON(*httpBase+"/api/wettkampf"), "Adler I")["sumInt"] != adlerSum {
		log.Fatal("Bahn clear wiped dual-meet Wettkampf totals")
	}

	log.Println("=== 4 teams / 12 shooters, two waves, then rearrange ===")
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"reset": true, "expectedPerTeam": 3}))
	clearHall(*httpBase)
	wave1 := []shooter{
		{"Anna", "Müller", "SV Adler", "Adler I", 1, 10.1},
		{"Peter", "Klein", "SV Adler", "Adler I", 2, 9.6},
		{"Jonas", "Becker", "KSG Mitte", "Mitte I", 3, 9.9},
		{"Uwe", "Hart", "SV West", "West II", 4, 9.4},
		{"Ina", "Berg", "SV West", "West II", 5, 9.8},
		{"Otto", "See", "SG Ost", "Ost I", 6, 9.2},
	}
	wave2 := []shooter{
		{"Lisa", "Wolf", "SV Adler", "Adler I", 1, 9.3},
		{"Sara", "Lang", "KSG Mitte", "Mitte I", 2, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Mitte I", 3, 9.0},
		{"Mia", "Feld", "SV West", "West II", 4, 10.0},
		{"Eva", "Horn", "SG Ost", "Ost I", 5, 9.7},
		{"Jan", "Moos", "SG Ost", "Ost I", 6, 9.1},
	}
	fireHall(*httpBase, *udpAddr, []program{
		lgProg(wave1[0], 5, 40),
		lgProg(wave1[1], 5, 40),
		lgProg(wave1[2], 3, 12),
		lgProg(wave1[3], 5, 40),
		lgProg(wave1[4], 5, 40),
		lgProg(wave1[5], 2, 20),
	}, *pause)
	resetRange(*httpBase, 3)
	otto := roster(getJSON(*httpBase+"/api/wettkampf"), "Otto See")
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{
		"roster": []map[string]any{{"key": otto["key"], "teamId": otto["teamId"], "locked": true}},
	}))
	dump(*httpBase, "wave 1 (2 Adler done, Jonas 12/40, 2 West done, Otto locked 20/40)")
	clearHall(*httpBase)
	fireHall(*httpBase, *udpAddr, programsLG(wave2, 5, 40), *pause)
	dump(*httpBase, "wave 2 (12 shooters on 4 teams)")
	requireShots(*httpBase, "Jonas Becker", 12, false)
	requireShots(*httpBase, "Otto See", 20, true)

	ottoShooter := wave1[5]
	ottoShooter.Range = 1
	ottoBefore := roster(getJSON(*httpBase+"/api/wettkampf"), "Otto See")["sumInt"]
	fireProgram(*httpBase, *udpAddr, ottoShooter, 0, 5, "LG 40 Schuss", "LG", *pause)
	if roster(getJSON(*httpBase+"/api/wettkampf"), "Otto See")["sumInt"] != ottoBefore {
		log.Fatal("locked Otto overwritten by later Bahn 1 session")
	}

	snap := getJSON(*httpBase + "/api/wettkampf")
	lisa := roster(snap, "Lisa Wolf")
	jonas := roster(snap, "Jonas Becker")
	ost := team(snap, "Ost I")
	west := team(snap, "West II")
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{
		"roster": []map[string]any{
			{"key": lisa["key"], "teamId": ost["id"], "locked": lisa["locked"]},
			{"key": jonas["key"], "teamId": west["id"], "locked": jonas["locked"]},
		},
	}))
	dump(*httpBase, "after moving Lisa → Ost I and Jonas → West II")

	jonasShooter := wave1[2]
	jonasShooter.Range = 6
	fireProgram(*httpBase, *udpAddr, jonasShooter, 0, 40, "LG 40 Schuss", "LG", *pause)
	requireShots(*httpBase, "Jonas Becker", 40, true)
	dump(*httpBase, "Jonas finished unlocked reshoot on Bahn 6")

	log.Println("=== competition 2: reset roster, keep teams, LP 40 on all 6 Bahnen ===")
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"reset": true}))
	clearHall(*httpBase)
	lp := []shooter{
		{"Carla", "Pist", "SV Adler", "Adler I", 1, 9.8},
		{"Ben", "Pist", "SV Adler", "Adler I", 2, 9.4},
		{"Nia", "Pist", "SV Adler", "Adler I", 3, 10.1},
		{"Omar", "Pist", "KSG Mitte", "Mitte I", 4, 9.6},
		{"Pia", "Pist", "KSG Mitte", "Mitte I", 5, 9.2},
		{"Rico", "Pist", "KSG Mitte", "Mitte I", 6, 9.9},
	}
	fireHall(*httpBase, *udpAddr, programsLP(lp, 5, 40), *pause)
	dump(*httpBase, "after LP 40 (Adler I vs Mitte I; West/Ost idle)")
	requireTeamCount(*httpBase, "Adler I", 3)
	requireTeamCount(*httpBase, "Mitte I", 3)
	requireTeamCount(*httpBase, "West II", 0)
	requireTeamCount(*httpBase, "Ost I", 0)
	for _, name := range []string{"Carla Pist", "Ben Pist", "Nia Pist", "Omar Pist", "Pia Pist", "Rico Pist"} {
		requireShots(*httpBase, name, 40, true)
	}

	snap = getJSON(*httpBase + "/api/wettkampf")
	ost = team(snap, "Ost I")
	park := make([]map[string]any, 0)
	for _, raw := range asSlice(snap["roster"]) {
		m, _ := raw.(map[string]any)
		park = append(park, map[string]any{"key": m["key"], "teamId": ost["id"], "locked": false})
	}
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"roster": park}))
	dump(*httpBase, "all LP shooters parked on Ost I")
	must(putJSON(*httpBase+"/api/wettkampf", map[string]any{"action": "assignByClub", "assignByClub": true}))
	dump(*httpBase, "after Nach Verein zuordnen")
	requireTeamCount(*httpBase, "Adler I", 3)
	requireTeamCount(*httpBase, "Mitte I", 3)
}

func runClub5(httpBase, udpAddr string, pause time.Duration, menu, disc string, wertung int) {
	must(putJSON(httpBase+"/api/wettkampf", map[string]any{"reset": true, "expectedPerTeam": 3}))
	clearHall(httpBase)

	adler := []shooter{
		{"Anna", "Müller", "SV Adler", "Adler I", 0, 10.2},
		{"Peter", "Klein", "SV Adler", "Adler I", 0, 9.7},
		{"Lisa", "Wolf", "SV Adler", "Adler I", 0, 9.3},
		{"Uwe", "Hart", "SV Adler", "Adler I", 0, 9.4},
		{"Ina", "Berg", "SV Adler", "Adler I", 0, 9.8},
	}
	mitte := []shooter{
		{"Jonas", "Becker", "KSG Mitte", "Mitte I", 0, 10.0},
		{"Sara", "Lang", "KSG Mitte", "Mitte I", 0, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Mitte I", 0, 9.1},
		{"Mia", "Feld", "KSG Mitte", "Mitte I", 0, 10.0},
		{"Otto", "See", "KSG Mitte", "Mitte I", 0, 9.2},
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(adler), func(i, j int) { adler[i], adler[j] = adler[j], adler[i] })
	rng.Shuffle(len(mitte), func(i, j int) { mitte[i], mitte[j] = mitte[j], mitte[i] })
	teamA, extraA := adler[:3], adler[3:]
	teamM, extraM := mitte[:3], mitte[3:]

	// Mixed hall: Adler on odd Bahnen, Mitte on even — headers alternate club color.
	hall := make([]shooter, 6)
	for i := 0; i < 3; i++ {
		a := teamA[i]
		a.Range = i*2 + 1
		m := teamM[i]
		m.Range = i*2 + 2
		hall[i*2], hall[i*2+1] = a, m
	}

	log.Printf("=== club5 wave 1 mixed %s: team Adler %s / %s / %s on 1,3,5; team Mitte %s / %s / %s on 2,4,6 ===",
		menu,
		hall[0].First+" "+hall[0].Last, hall[2].First+" "+hall[2].Last, hall[4].First+" "+hall[4].Last,
		hall[1].First+" "+hall[1].Last, hall[3].First+" "+hall[3].Last, hall[5].First+" "+hall[5].Last)
	fireHall(httpBase, udpAddr, programsMenu(hall, 5, wertung, menu, disc), pause)
	dump(httpBase, "mixed 3+3 on all 6 Bahnen")
	requireTeamCount(httpBase, "Adler I", 3)
	requireTeamCount(httpBase, "Mitte I", 3)

	// Extras take the other club's Bahnen so lane headers change color in place.
	extraM[0].Range = 1
	extraA[0].Range = 2
	extraM[1].Range = 3
	extraA[1].Range = 4
	log.Printf("=== club5 wave 2: extras swap clubs on 1–4 (%s/%s Mitte, %s/%s Adler) ===",
		extraM[0].First+" "+extraM[0].Last, extraM[1].First+" "+extraM[1].Last,
		extraA[0].First+" "+extraA[0].Last, extraA[1].First+" "+extraA[1].Last)
	time.Sleep(8 * time.Second)
	fireHall(httpBase, udpAddr, programsMenu(append(append([]shooter{}, extraA...), extraM...), 5, wertung, menu, disc), pause)
	excludeNames(httpBase, []string{
		extraA[0].First + " " + extraA[0].Last,
		extraA[1].First + " " + extraA[1].Last,
		extraM[0].First + " " + extraM[0].Last,
		extraM[1].First + " " + extraM[1].Last,
	})
	dump(httpBase, "after extras excluded (3+3 remain on teams)")
	requireTeamCount(httpBase, "Adler I", 3)
	requireTeamCount(httpBase, "Mitte I", 3)
	snap := getJSON(httpBase + "/api/wettkampf")
	if len(asSlice(snap["roster"])) != 10 {
		log.Fatalf("roster %d want 10", len(asSlice(snap["roster"])))
	}
	for _, p := range append(append([]shooter{}, extraA...), extraM...) {
		name := p.First + " " + p.Last
		r := roster(snap, name)
		if r["excluded"] != true {
			log.Fatalf("%s should be ohne Mannschaft: %#v", name, r)
		}
	}
	for _, p := range append(append([]shooter{}, teamA...), teamM...) {
		requireShots(httpBase, p.First+" "+p.Last, wertung, true)
	}
}

func runFourTeam(httpBase, udpAddr string, pause time.Duration) {
	must(putJSON(httpBase+"/api/wettkampf", map[string]any{
		"reset": true, "expectedPerTeam": 3,
		"teams": []map[string]string{{"name": "A"}, {"name": "B"}, {"name": "C"}, {"name": "Z"}},
	}))
	clearHall(httpBase)

	a := []shooter{
		{"Anna", "Müller", "SV Adler", "A", 0, 10.2},
		{"Peter", "Klein", "SV Adler", "A", 0, 9.7},
		{"Lisa", "Wolf", "SV Adler", "A", 0, 9.3},
	}
	aExtra := shooter{"Uwe", "Hart", "SV Adler", "A", 0, 9.4}
	b := []shooter{
		{"Ina", "Berg", "SV Adler", "B", 0, 9.8},
		{"Eva", "Horn", "SV Adler", "B", 0, 9.5},
		{"Jan", "Moos", "SV Adler", "B", 0, 9.1},
	}
	bExtra := shooter{"Mia", "Feld", "SV Adler", "B", 0, 10.0}
	c := []shooter{
		{"Carla", "Pist", "SV Adler", "C", 0, 9.6},
		{"Ben", "Pist", "SV Adler", "C", 0, 9.2},
		{"Nia", "Pist", "SV Adler", "C", 0, 9.9},
	}
	z := []shooter{
		{"Jonas", "Becker", "KSG Mitte", "Z", 0, 10.0},
		{"Sara", "Lang", "KSG Mitte", "Z", 0, 9.5},
		{"Tim", "Koch", "KSG Mitte", "Z", 0, 9.1},
	}

	wave1 := []shooter{
		onRange(a[0], 1), onRange(z[0], 2), onRange(b[0], 3),
		onRange(c[0], 4), onRange(a[1], 5), onRange(z[1], 6),
	}
	log.Println("=== fourteam wave 1: A/Z/B/C/A/Z mixed, LGA30 + LG40 ===")
	fireHall(httpBase, udpAddr, discPrograms(wave1), pause)

	wave2 := []shooter{
		onRange(a[2], 1), onRange(z[2], 2), onRange(b[1], 3),
		onRange(c[1], 4), onRange(aExtra, 5), onRange(b[2], 6),
	}
	log.Println("=== fourteam wave 2: remaining A/B/C/Z + A extra ===")
	fireHall(httpBase, udpAddr, discPrograms(wave2), pause)
	excludeNames(httpBase, []string{aExtra.First + " " + aExtra.Last})

	wave3 := []shooter{
		onRange(bExtra, 1), onRange(c[2], 2),
	}
	log.Println("=== fourteam wave 3: B extra + last C ===")
	fireHall(httpBase, udpAddr, discPrograms(wave3), pause)
	excludeNames(httpBase, []string{bExtra.First + " " + bExtra.Last})

	dump(httpBase, "14 shooters / 4 teams (A B LGA30, C Z LG40)")
	for _, name := range []string{"A", "B", "C", "Z"} {
		requireTeamCount(httpBase, name, 3)
	}
	snap := getJSON(httpBase + "/api/wettkampf")
	if len(asSlice(snap["roster"])) != 14 {
		log.Fatalf("roster %d want 14", len(asSlice(snap["roster"])))
	}
	for _, name := range []string{"Uwe Hart", "Mia Feld"} {
		r := roster(snap, name)
		if r["excluded"] != true {
			log.Fatalf("%s should be ohne Mannschaft: %#v", name, r)
		}
	}
	for _, p := range append(append([]shooter{}, a...), b...) {
		requireShots(httpBase, p.First+" "+p.Last, 30, true)
	}
	requireShots(httpBase, aExtra.First+" "+aExtra.Last, 30, true)
	requireShots(httpBase, bExtra.First+" "+bExtra.Last, 30, true)
	for _, p := range append(append([]shooter{}, c...), z...) {
		requireShots(httpBase, p.First+" "+p.Last, 40, true)
	}
}

func onRange(p shooter, rng int) shooter {
	p.Range = rng
	return p
}

func discPrograms(people []shooter) []program {
	out := make([]program, len(people))
	for i, p := range people {
		if p.Team == "A" || p.Team == "B" {
			out[i] = menuProg(p, 5, 30, "LG 30 Schuss Auflage", "LG")
			continue
		}
		out[i] = menuProg(p, 5, 40, "LG 40 Schuss", "LG")
	}
	return out
}

func excludeNames(base string, names []string) {
	snap := getJSON(base + "/api/wettkampf")
	patch := make([]map[string]any, 0, len(names))
	for _, name := range names {
		r := roster(snap, name)
		patch = append(patch, map[string]any{
			"key": r["key"], "teamId": r["teamId"], "excluded": true, "locked": r["locked"],
		})
	}
	must(putJSON(base+"/api/wettkampf", map[string]any{"roster": patch}))
}

func fireProgram(httpBase, addr string, p shooter, warmup, wertung int, menu, disc string, pause time.Duration) {
	fireHall(httpBase, addr, []program{{
		shooter: p, Warmup: warmup, Wertung: wertung, Menu: menu, Disc: disc,
	}}, pause)
}

type program struct {
	shooter
	Warmup, Wertung int
	Menu, Disc      string
}

func lgProg(p shooter, warmup, wertung int) program {
	return menuProg(p, warmup, wertung, "LG 40 Schuss", "LG")
}

func lpProg(p shooter, warmup, wertung int) program {
	return menuProg(p, warmup, wertung, "LP 40 Schuss", "LP")
}

func menuProg(p shooter, warmup, wertung int, menu, disc string) program {
	return program{shooter: p, Warmup: warmup, Wertung: wertung, Menu: menu, Disc: disc}
}

func programsMenu(people []shooter, warmup, wertung int, menu, disc string) []program {
	out := make([]program, len(people))
	for i, p := range people {
		out[i] = menuProg(p, warmup, wertung, menu, disc)
	}
	return out
}

func programsLG(people []shooter, warmup, wertung int) []program {
	out := make([]program, len(people))
	for i, p := range people {
		out[i] = lgProg(p, warmup, wertung)
	}
	return out
}

func programsLP(people []shooter, warmup, wertung int) []program {
	out := make([]program, len(people))
	for i, p := range people {
		out[i] = lpProg(p, warmup, wertung)
	}
	return out
}

func fireHall(httpBase, addr string, progs []program, pause time.Duration) {
	type run struct {
		program
		warmLeft, wertLeft int
		warmIdx, wertIdx   int
	}
	runs := make([]run, len(progs))
	for i, p := range progs {
		runs[i] = run{program: p, warmLeft: p.Warmup, wertLeft: p.Wertung}
	}
	start := 0
	for {
		progressed := false
		n := len(runs)
		for k := 0; k < n; k++ {
			r := &runs[(start+k)%n]
			if r.warmLeft == 0 && r.wertLeft == 0 {
				continue
			}
			warmup := r.warmLeft > 0
			idx := r.warmIdx
			if !warmup {
				idx = 50 + r.wertIdx
			}
			dec := clamp(r.Skill - 0.2)
			if !warmup {
				dec = clamp(r.Skill - 0.1*float64(r.wertIdx%7))
			}
			sendShot(addr, r.shooter, warmup, dec, idx, r.Menu, r.Disc)
			if warmup {
				r.warmIdx++
				r.warmLeft--
			} else {
				r.wertIdx++
				r.wertLeft--
			}
			progressed = true
			if pause > 0 {
				time.Sleep(pause)
			}
		}
		if !progressed {
			break
		}
		start++
	}
	for _, p := range progs {
		waitLiveWertung(httpBase, p.Range, p.Wertung)
		log.Printf("fired %s %s  probe=%d wertung=%d  Bahn %d", p.First, p.Last, p.Warmup, p.Wertung, p.Range)
	}
}

func sendShot(addr string, p shooter, warmup bool, dec float64, n int, menu, disc string) {
	opts := udp.ShotPacketOpts{
		Range: p.Range, DecValue: dec, IsWarmup: warmup,
		Shooter: p.First, Lastname: p.Last, Club: p.Club, Team: p.Team,
		MenuItem: menu, DiscType: disc,
	}
	x, y, dist := udp.PlaceShotForDisc(disc, dec, p.Range*100+n)
	opts.X, opts.Y, opts.Distance = x, y, dist
	if err := udp.SendShotPacket(addr, opts); err != nil {
		log.Fatalf("udp %s: %v", p.First, err)
	}
}

func clamp(d float64) float64 {
	if d > 10.9 {
		return 10.9
	}
	if d < 8 {
		return 8
	}
	return float64(int(d*10+0.5)) / 10
}

func clearHall(base string) {
	for n := 1; n <= 6; n++ {
		resetRange(base, n)
	}
}

func resetRange(base string, n int) {
	resp, err := http.Post(fmt.Sprintf("%s/api/live/reset?range=%d", base, n), "application/json", nil)
	if err != nil {
		log.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatalf("reset %d: %s", n, resp.Status)
	}
}

func dump(base, title string) {
	snap := getJSON(base + "/api/wettkampf")
	log.Printf("--- %s ---", title)
	teams, _ := snap["teams"].([]any)
	for _, raw := range teams {
		t, _ := raw.(map[string]any)
		log.Printf("  %-8s  %v/%v  sum %v / %v  prog %v / %v  members %d",
			t["name"], t["count"], t["expected"], t["sumInt"], t["sumDec"],
			t["predInt"], t["predDec"], len(asSlice(t["members"])))
		for _, m := range asSlice(t["members"]) {
			mm, _ := m.(map[string]any)
			log.Printf("      %-16s  shots %v/%v  %v  locked=%v", mm["name"], mm["shotsFired"], mm["totalShots"], mm["sumInt"], mm["locked"])
		}
	}
	log.Printf("  roster %d", len(asSlice(snap["roster"])))
	for _, raw := range asSlice(snap["roster"]) {
		m, _ := raw.(map[string]any)
		if m["excluded"] == true {
			log.Printf("      ohne Mannschaft  %-16s  shots %v  %v", m["name"], m["shotsFired"], m["sumInt"])
		}
	}
}

func waitLiveWertung(base string, rangeNum, want int) {
	deadline := time.Now().Add(8 * time.Second)
	got := 0
	for time.Now().Before(deadline) {
		live := getJSON(base + "/api/live")
		for _, raw := range asSlice(live["ranges"]) {
			m, _ := raw.(map[string]any)
			if int(num(m["rangeNum"])) != rangeNum {
				continue
			}
			got = int(num(m["shotNumber"]))
			if got >= want {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	log.Fatalf("Bahn %d wertung %d want %d (UDP drop or still draining)", rangeNum, got, want)
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func requireShots(base, name string, shots int, locked bool) {
	r := roster(getJSON(base+"/api/wettkampf"), name)
	got, _ := r["shotsFired"].(float64)
	if int(got) != shots {
		log.Fatalf("%s shots %v want %d (UDP drop?)", name, r["shotsFired"], shots)
	}
	if r["locked"] != locked {
		log.Fatalf("%s locked=%v want %v", name, r["locked"], locked)
	}
}

func requireTeamCount(base, name string, n int) {
	t := team(getJSON(base+"/api/wettkampf"), name)
	got, _ := t["count"].(float64)
	if int(got) != n {
		log.Fatalf("%s count %v want %d", name, t["count"], n)
	}
}

func roster(snap map[string]any, name string) map[string]any {
	for _, raw := range asSlice(snap["roster"]) {
		m, _ := raw.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	log.Fatalf("missing %s", name)
	return nil
}

func team(snap map[string]any, name string) map[string]any {
	for _, raw := range asSlice(snap["teams"]) {
		m, _ := raw.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	log.Fatalf("missing team %s", name)
	return nil
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func postJSON(url string, body any) error {
	return doJSON(http.MethodPost, url, body)
}

func putJSON(url string, body any) error {
	return doJSON(http.MethodPut, url, body)
}

func doJSON(method, url string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, url, resp.Status)
	}
	return nil
}

func getJSON(url string) map[string]any {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Fatal(err)
	}
	return out
}
