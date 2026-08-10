// sim-classic-stress sends 30 warmup + 100 competition shots per range
// using a synthetic field with representative mean scores.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"time"

	"srdashboard/udp"
)

var field = []struct {
	First, Last string
	Mean        float64
	Range       int
}{
	{"Alex", "Alpha", 10.34, 1},
	{"Blake", "Bravo", 10.08, 2},
	{"Casey", "Charlie", 8.57, 3},
	{"Dana", "Delta", 8.49, 4},
	{"Eden", "Echo", 7.01, 5},
	{"Finn", "Foxtrot", 6.67, 6},
}

func main() {
	httpBase := flag.String("http", "http://127.0.0.1:8080", "HTTP base")
	host := flag.String("host", "127.0.0.1", "UDP host")
	port := flag.Int("udp-port", 30169, "UDP port")
	warmupN := flag.Int("warmup", 30, "Warmup shots per range")
	compN := flag.Int("shots", 100, "Competition shots per range")
	interval := flag.Duration("interval", 80*time.Millisecond, "Delay between shots")
	seed := flag.Int64("seed", 42, "RNG seed")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	base := *httpBase
	addr := fmt.Sprintf("%s:%d", *host, *port)

	must(postJSON(base+"/api/plugins/activate", map[string]any{"id": "classic-range"}))
	time.Sleep(200 * time.Millisecond)
	for r := 1; r <= 6; r++ {
		resp, err := http.Post(fmt.Sprintf("%s/api/live/reset?range=%d", base, r), "application/json", nil)
		if err == nil {
			resp.Body.Close()
		}
	}
	// classic-range is display-only — no /api/plugins/control; names/discipline come from UDP.

	sent := 0
	log.Printf("warmup %d × %d ranges…", *warmupN, len(field))
	for i := 1; i <= *warmupN; i++ {
		for _, sh := range field {
			dec := sample(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			must(udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				IsWarmup: true, Shooter: sh.First + " " + sh.Last, ShotAt: time.Now(), MenuItem: "LG 100 Schuss",
			}))
			sent++
			time.Sleep(*interval)
		}
		if i%10 == 0 {
			log.Printf("  warmup %d/%d sent=%d", i, *warmupN, sent)
		}
	}

	log.Printf("competition %d × %d ranges…", *compN, len(field))
	for i := 1; i <= *compN; i++ {
		for _, sh := range field {
			dec := sample(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			must(udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				IsWarmup: false, Shooter: sh.First + " " + sh.Last, ShotAt: time.Now(), MenuItem: "LG 100 Schuss",
			}))
			sent++
			time.Sleep(*interval)
		}
		if i%20 == 0 {
			log.Printf("  competition %d/%d sent=%d", i, *compN, sent)
		}
	}

	time.Sleep(400 * time.Millisecond)
	live, err := getJSON(base + "/api/live")
	if err != nil {
		log.Fatal(err)
	}
	out := map[string]any{
		"generatedAt": time.Now().Format(time.RFC3339),
		"warmupPerRange": *warmupN,
		"competitionPerRange": *compN,
		"totalSent": sent,
		"live": live,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	_ = os.MkdirAll("testdata", 0o755)
	must(os.WriteFile("testdata/classic-stress-100.json", b, 0o644))
	log.Printf("done totalSent=%d → testdata/classic-stress-100.json", sent)
	printLive(live)
}

func printLive(live map[string]any) {
	// /api/live may be {ranges:[...]} or map keyed by range
	if arr, ok := live["ranges"].([]any); ok {
		for _, it := range arr {
			m, _ := it.(map[string]any)
			log.Printf("R%v %-20v shots=%v warmup=%v last=%v sumDec=%v",
				m["rangeNum"], m["shooterName"], m["shotNumber"], m["isWarmup"], m["currentShotValue"], m["overallSumDecimal"])
		}
		return
	}
	for r := 1; r <= 6; r++ {
		key := fmt.Sprintf("%d", r)
		m, _ := live[key].(map[string]any)
		if m == nil {
			continue
		}
		log.Printf("R%d %-20v shots=%v last=%v sumDec=%v",
			r, m["shooterName"], m["shotNumber"], m["currentShotValue"], m["overallSumDecimal"])
	}
}

const (
	decMax       = 10.9
	rifleBandDSG = 25.0 // LG 10m: 0.25 mm per tenth → 25 DSG per 0.1 ring
)

func sample(rng *rand.Rand, mean float64) float64 {
	// Capability mean → score floor: strong shooters still spray into the 9s.
	// Uniform on [lo, 10.9] matches cmd/sim-f1-match pickDec (not a top-clipped normal).
	lo := mean - 1.8
	if mean < 8.5 {
		lo = mean - 2.5
	}
	if mean < 7.5 {
		lo = 0
	}
	if lo < 0 {
		lo = 0
	}
	if lo > decMax {
		lo = decMax
	}
	steps := int(math.Round((decMax - lo) * 10))
	if steps < 0 {
		steps = 0
	}
	return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
}

// coords places the pellet in the correct teiler band for dec (LG air-rifle geometry).
func coords(rng *rand.Rand, dec float64) (x, y int, distance float64) {
	if dec > decMax {
		dec = decMax
	}
	band := int(math.Round((decMax - dec) * 10))
	lo := float64(band) * rifleBandDSG
	hi := lo + rifleBandDSG
	if dec >= decMax {
		distance = rng.Float64() * rifleBandDSG * 0.4
	} else {
		distance = lo + rng.Float64()*(hi-lo)
	}
	distance = math.Round(distance*10) / 10
	ang := rng.Float64() * 2 * math.Pi
	x = int(math.Round(math.Cos(ang) * distance))
	y = int(math.Round(math.Sin(ang) * distance))
	distance = math.Round(math.Hypot(float64(x), float64(y))*10) / 10
	return
}

func postJSON(url string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s %s", url, resp.Status, string(b))
	}
	return nil
}

func getJSON(url string) (map[string]any, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
}
