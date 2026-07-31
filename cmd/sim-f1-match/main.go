// sim-f1-match sends a round-based 40-shot F1 race simulation over UDP.
//
// Ranges 1–3: advanced (DecValue 7.0–10.9)
// Ranges 4–6: beginners (DecValue 4.0–10.9)
// 3s pause between rounds except pit rounds (every 10th shot).
// Firing order is shuffled each round, including after pit exit.
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
	"time"

	"srdashboard/udp"
)

const (
	decMax    = 10.9
	rifleBand = 25.0
	stintSize = 10
)

func pickDec(rng *rand.Rand, lo float64) float64 {
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

func main() {
	host := flag.String("host", "127.0.0.1", "Dashboard host")
	udpPort := flag.Int("udp-port", 30169, "UDP port")
	httpBase := flag.String("http", "http://127.0.0.1:8080", "HTTP base URL")
	nShots := flag.Int("shots", 40, "Shots per range")
	roundDelay := flag.Duration("round-delay", 3*time.Second, "Delay after non-pit rounds")
	intraDelay := flag.Duration("intra-delay", 80*time.Millisecond, "Delay between shots within a round")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	skipStart := flag.Bool("skip-start", false, "Do not reset/start the F1 plugin session")
	flag.Parse()

	s := *seed
	if s == 0 {
		s = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(s))
	addr := fmt.Sprintf("%s:%d", *host, *udpPort)

	shooters := map[int]struct {
		name string
		lo   float64
	}{
		1: {"Adv-Anna", 7.0},
		2: {"Adv-Max", 7.0},
		3: {"Adv-Lena", 7.0},
		4: {"Beg-Tom", 4.0},
		5: {"Beg-Mia", 4.0},
		6: {"Beg-Jonas", 4.0},
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
		live := map[string]any{}
		for _, r := range ranges {
			live[fmt.Sprintf("%d", r)] = map[string]any{
				"totalShotsToFire": *nShots,
				"discipline":       "LG 40 Schuss",
				"isWarmup":         false,
				"shooterName":      shooters[r].name,
			}
		}
		if err := postJSON(*httpBase+"/api/plugins/control", map[string]any{
			"action": "start",
			"params": map[string]any{"live": live},
		}); err != nil {
			log.Fatalf("plugin start: %v", err)
		}
		log.Printf("race started (%d shots)", *nShots)
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("simulating %d rounds on ranges 1–6 seed=%d roundDelay=%v (skip on pit)", *nShots, s, *roundDelay)
	base := time.Now()

	for round := 1; round <= *nShots; round++ {
		order := append([]int(nil), ranges...)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		isPit := stintSize > 0 && round%stintSize == 0
		tag := "power"
		if isPit {
			tag = "PIT"
		}
		log.Printf("round %2d/%d [%s] order=%v", round, *nShots, tag, order)

		for i, rangeNum := range order {
			sh := shooters[rangeNum]
			dec := pickDec(rng, sh.lo)
			x, y, dist := pickShotCoords(rng, dec, rifleBand)
			if err := udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range:    rangeNum,
				X:        x,
				Y:        y,
				Distance: dist,
				DecValue: dec,
				IsWarmup: false,
				Shooter:  sh.name,
				ShotAt:   base.Add(time.Duration(round)*time.Second + time.Duration(i)*50*time.Millisecond),
				MenuItem: "LG 40 Schuss",
			}); err != nil {
				log.Fatalf("round %d range %d: %v", round, rangeNum, err)
			}
			if i < len(order)-1 {
				time.Sleep(*intraDelay)
			}
		}

		if round < *nShots && !isPit {
			time.Sleep(*roundDelay)
		} else if round < *nShots && isPit {
			log.Printf("  pit exit — shuffling next stint immediately")
		}
	}

	log.Printf("done: %d rounds × %d ranges", *nShots, len(ranges))
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}
