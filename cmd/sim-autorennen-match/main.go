// sim-f1-match sends a warmup + 40-shot F1 race simulation over UDP.
//
// Ranges 1–2: advanced  (DecValue 7.5–10.9)
// Ranges 3–4: midrange  (DecValue 4.5–10.9)
// Ranges 5–6: beginners (DecValue 0.0–10.9)
//
// Timing: ~7s between round opens. Each shooter gets an independent
// reaction offset of ±2s (skill-independent). Pit rounds (every 10th shot)
// fire relative to the shared pit cue instead of waiting a full 7s first,
// so reaction times land inside / around the 5s pit window.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"time"

	"srdashboard/udp"
)

const (
	decMax       = 10.9
	rifleBand    = 25.0
	stintSize    = 10
	warmupShots  = 3
	reactCenter  = 2 * time.Second // nominal reaction / fire delay
	reactJitter  = 2 * time.Second // ±2s, skill-independent
	roundSpacing = 7 * time.Second
)

func pickDec(rng *rand.Rand, lo float64) float64 {
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

func pickShotCoords(rng *rand.Rand, dec, teilerBandDsg float64) (x, y int, distance float64) {
	if dec > decMax {
		dec = decMax
	}
	band := int(math.Round((decMax - dec) * 10))
	lo := float64(band) * teilerBandDsg
	hi := lo + teilerBandDsg
	if dec >= decMax {
		distance = rng.Float64() * teilerBandDsg * 0.4
	} else {
		distance = lo + rng.Float64()*(hi-lo)
	}
	distance = math.Round(distance*10) / 10
	angle := rng.Float64() * 2 * math.Pi
	x = int(math.Round(math.Cos(angle) * distance))
	y = int(math.Round(math.Sin(angle) * distance))
	distance = math.Round(math.Hypot(float64(x), float64(y))*10) / 10
	return x, y, distance
}

// reactionOffset returns a skill-independent delay in [0, 4s] centered at 2s ±2s.
func reactionOffset(rng *rand.Rand) time.Duration {
	// uniform in [-2s, +2s]
	j := time.Duration((rng.Float64()*2 - 1) * float64(reactJitter))
	d := reactCenter + j
	if d < 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	return d
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
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return nil
}

func sleepUntil(t time.Time) {
	d := time.Until(t)
	if d > 0 {
		time.Sleep(d)
	}
}

type shooter struct {
	name string
	lo   float64
	tier string
}

func main() {
	host := flag.String("host", "127.0.0.1", "Dashboard host")
	udpPort := flag.Int("udp-port", 30169, "UDP port")
	httpBase := flag.String("http", "http://127.0.0.1:8080", "HTTP base URL")
	nShots := flag.Int("shots", 40, "Competition shots per range")
	nWarmup := flag.Int("warmup", warmupShots, "Warmup shots per range before race start")
	interval := flag.Duration("interval", roundSpacing, "Target spacing between round opens")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	skipStart := flag.Bool("skip-start", false, "Do not reset/start the F1 plugin session")
	flag.Parse()

	s := *seed
	if s == 0 {
		s = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(s))
	addr := fmt.Sprintf("%s:%d", *host, *udpPort)

	shooters := map[int]shooter{
		1: {"Adv-Anna", 7.5, "advanced"},
		2: {"Adv-Max", 7.5, "advanced"},
		3: {"Mid-Lena", 4.5, "midrange"},
		4: {"Mid-Tom", 4.5, "midrange"},
		5: {"Beg-Mia", 0.0, "beginner"},
		6: {"Beg-Jonas", 0.0, "beginner"},
	}
	ranges := []int{1, 2, 3, 4, 5, 6}

	if !*skipStart {
		log.Printf("resetting live ranges + F1 session…")
		for _, r := range ranges {
			u := fmt.Sprintf("%s/api/live/reset?range=%d", *httpBase, r)
			resp, err := http.Post(u, "application/json", nil)
			if err != nil {
				log.Fatalf("live reset range %d: %v", r, err)
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Fatalf("live reset range %d: %s", r, resp.Status)
			}
		}
		if err := postJSON(*httpBase+"/api/plugins/control", map[string]any{"action": "reset"}); err != nil {
			log.Fatalf("plugin reset: %v", err)
		}

		liveWarm := map[string]any{}
		for _, r := range ranges {
			liveWarm[fmt.Sprintf("%d", r)] = map[string]any{
				"totalShotsToFire": *nShots,
				"discipline":       "LG 40 Schuss",
				"isWarmup":         true,
				"shooterName":      shooters[r].name,
			}
		}
		if err := postJSON(*httpBase+"/api/plugins/control", map[string]any{
			"action": "sync_live",
			"params": map[string]any{"live": liveWarm},
		}); err != nil {
			log.Fatalf("sync_live warmup: %v", err)
		}

		log.Printf("warmup: %d shots × %d ranges", *nWarmup, len(ranges))
		for w := 1; w <= *nWarmup; w++ {
			order := append([]int(nil), ranges...)
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
			log.Printf("  warmup round %d/%d order=%v", w, *nWarmup, order)
			for _, rangeNum := range order {
				sh := shooters[rangeNum]
				dec := pickDec(rng, sh.lo)
				x, y, dist := pickShotCoords(rng, dec, rifleBand)
				if err := udp.SendShotPacket(addr, udp.ShotPacketOpts{
					Range:    rangeNum,
					X:        x,
					Y:        y,
					Distance: dist,
					DecValue: dec,
					IsWarmup: true,
					Shooter:  sh.name,
					ShotAt:   time.Now(),
					MenuItem: "LG 40 Schuss",
				}); err != nil {
					log.Fatalf("warmup range %d: %v", rangeNum, err)
				}
				time.Sleep(80 * time.Millisecond)
			}
			time.Sleep(400 * time.Millisecond)
		}

		liveRace := map[string]any{}
		for _, r := range ranges {
			liveRace[fmt.Sprintf("%d", r)] = map[string]any{
				"totalShotsToFire": *nShots,
				"discipline":       "LG 40 Schuss",
				"isWarmup":         false,
				"shooterName":      shooters[r].name,
			}
		}
		if err := postJSON(*httpBase+"/api/plugins/control", map[string]any{
			"action": "start",
			"params": map[string]any{"live": liveRace},
		}); err != nil {
			log.Fatalf("plugin start: %v", err)
		}
		log.Printf("race started (%d shots, interval=%v, reaction=±%v)", *nShots, *interval, reactJitter)
		time.Sleep(400 * time.Millisecond)
	}

	log.Printf("simulating %d competition rounds on ranges 1–6 seed=%d", *nShots, s)
	log.Printf("tiers: 1–2 advanced(7.5–10.9)  3–4 mid(4.5–10.9)  5–6 beg(0.0–10.9)")

	for shotNum := 1; shotNum <= *nShots; shotNum++ {
		isPit := stintSize > 0 && shotNum%stintSize == 0
		isGrid := shotNum == 1
		tag := "power"
		if isGrid {
			tag = "GRID"
		} else if isPit {
			tag = "PIT"
		}

		roundOpen := time.Now()
		// Schedule each shooter's fire time from round open with ±2s reaction.
		type sched struct {
			rangeNum int
			at       time.Time
			react    time.Duration
		}
		plan := make([]sched, 0, len(ranges))
		for _, r := range ranges {
			react := reactionOffset(rng)
			plan = append(plan, sched{rangeNum: r, at: roundOpen.Add(react), react: react})
		}
		sort.Slice(plan, func(i, j int) bool { return plan[i].at.Before(plan[j].at) })

		orderLog := make([]int, len(plan))
		for i, p := range plan {
			orderLog[i] = p.rangeNum
		}
		log.Printf("shot %2d/%d [%s] open=%s order=%v",
			shotNum, *nShots, tag, roundOpen.Format("15:04:05.0"), orderLog)

		for _, p := range plan {
			sleepUntil(p.at)
			sh := shooters[p.rangeNum]
			dec := pickDec(rng, sh.lo)
			x, y, dist := pickShotCoords(rng, dec, rifleBand)
			if err := udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range:    p.rangeNum,
				X:        x,
				Y:        y,
				Distance: dist,
				DecValue: dec,
				IsWarmup: false,
				Shooter:  sh.name,
				ShotAt:   time.Now(),
				MenuItem: "LG 40 Schuss",
			}); err != nil {
				log.Fatalf("shot %d range %d: %v", shotNum, p.rangeNum, err)
			}
			log.Printf("    R%d %-10s %s dec=%4.1f react=%4.0fms",
				p.rangeNum, sh.name, sh.tier, dec, float64(p.react)/float64(time.Millisecond))
		}

		if shotNum >= *nShots {
			break
		}

		nextIsPit := stintSize > 0 && (shotNum+1)%stintSize == 0
		if nextIsPit {
			// Pit cue arms as soon as this round closes (last shot above).
			// Open the pit round immediately so reactions are measured from the cue.
			log.Printf("  → next is PIT — opening immediately (cue armed)")
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Hold ~7s from round open before the next power/grid follow-up.
		sleepUntil(roundOpen.Add(*interval))
	}

	log.Printf("done: %d competition shots × %d ranges (+ %d warmup)", *nShots, len(ranges), *nWarmup)
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}
