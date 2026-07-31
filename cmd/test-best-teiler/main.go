// test-best-teiler bursts random competition shots to every range, tracks the
// true best Teiler locally, then verifies GET /api/live reports the same Bester.
//
// Shot DecValue and Distance are correlated (OpticScore-style) so ring position
// matches the scored value and Summe matches what you see on the target/series.
//
//	go run ./cmd/test-best-teiler
//	go run ./cmd/test-best-teiler -ranges 6 -shots 40 -interval 0
package main

import (
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

const (
	decMax    = 10.9
	rifleBand = 25.0 // LG teilerBandDsg
)

type plannedShot struct {
	rangeNum int
	dist     float64
	dec      float64
	x, y     int
}

type expectedBest struct {
	teiler float64
	shot   int
	sumDec float64
	sumInt int
}

func pickDec(rng *rand.Rand, lo float64) float64 {
	steps := int(math.Round((decMax - lo) * 10))
	if steps < 0 {
		steps = 0
	}
	return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
}

func fullValue(dec float64) int {
	full := int(math.Floor(dec))
	if full > 10 {
		return 10
	}
	if full < 0 {
		return 0
	}
	return full
}

// pickShotCoords derives X/Y/Distance from DecValue so position matches score.
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

func main() {
	host := flag.String("host", "127.0.0.1", "Dashboard host")
	udpPort := flag.Int("udp-port", 30169, "UDP port")
	httpBase := flag.String("http", "http://127.0.0.1:8080", "HTTP base URL for /api/live")
	numRanges := flag.Int("ranges", 6, "Number of ranges")
	nShots := flag.Int("shots", 40, "Shots per range")
	decLo := flag.Float64("dec-lo", 8.0, "Minimum random DecValue")
	interval := flag.Duration("interval", 0, "Delay between UDP packets (0 = full burst)")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	skipVerify := flag.Bool("skip-verify", false, "Only send shots; do not call /api/live")
	flag.Parse()

	s := *seed
	if s == 0 {
		s = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(s))
	addr := fmt.Sprintf("%s:%d", *host, *udpPort)

	var queue []plannedShot
	for rangeNum := 1; rangeNum <= *numRanges; rangeNum++ {
		for i := 0; i < *nShots; i++ {
			dec := pickDec(rng, *decLo)
			x, y, dist := pickShotCoords(rng, dec, rifleBand)
			queue = append(queue, plannedShot{
				rangeNum: rangeNum,
				dist:     dist,
				dec:      dec,
				x:        x,
				y:        y,
			})
		}
	}

	// Burst in random order across all ranges (not sequential per stand).
	rng.Shuffle(len(queue), func(i, j int) { queue[i], queue[j] = queue[j], queue[i] })

	expect := make(map[int]*expectedBest, *numRanges)
	sentOnRange := make(map[int]int, *numRanges)
	for rangeNum := 1; rangeNum <= *numRanges; rangeNum++ {
		expect[rangeNum] = &expectedBest{}
	}

	log.Printf("burst %d shots (%d ranges × %d) to %s seed=%d interval=%v dec=%.1f–%.1f",
		len(queue), *numRanges, *nShots, addr, s, *interval, *decLo, decMax)

	base := time.Now()
	for n, sh := range queue {
		// Match udp.BuildShotPacket: Distance 0 with DecValue >= 10 becomes 0.1.
		dist := sh.dist
		if dist == 0 && sh.dec >= 10 {
			dist = 0.1
		}

		sentOnRange[sh.rangeNum]++
		shotNum := sentOnRange[sh.rangeNum]
		eb := expect[sh.rangeNum]
		if eb.shot == 0 || dist < eb.teiler {
			eb.teiler = dist
			eb.shot = shotNum
		}
		eb.sumInt += fullValue(sh.dec)
		eb.sumDec += sh.dec

		shooter := fmt.Sprintf("B%d-%d", s%100000, sh.rangeNum)
		if err := udp.SendShotPacket(addr, udp.ShotPacketOpts{
			Range:    sh.rangeNum,
			X:        sh.x,
			Y:        sh.y,
			Distance: sh.dist,
			DecValue: sh.dec,
			Shooter:  shooter,
			ShotAt:   base.Add(time.Duration(n) * time.Millisecond),
			MenuItem: "LG 40 Schuss",
		}); err != nil {
			log.Fatalf("range %d shot #%d: %v", sh.rangeNum, shotNum, err)
		}
		if *interval > 0 {
			time.Sleep(*interval)
		}
	}

	log.Printf("sent %d packets", len(queue))
	for rangeNum := 1; rangeNum <= *numRanges; rangeNum++ {
		eb := expect[rangeNum]
		log.Printf("  expect range %d: best=%.1f #%d sum=%.1f/%d",
			rangeNum, eb.teiler, eb.shot, eb.sumDec, eb.sumInt)
	}

	if *skipVerify {
		log.Println("done (verify skipped)")
		return
	}

	time.Sleep(500 * time.Millisecond)

	resp, err := http.Get(*httpBase + "/api/live")
	if err != nil {
		log.Fatalf("GET /api/live: %v (is the dashboard running on %s?)", err, *httpBase)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("GET /api/live: %s\n%s", resp.Status, body)
	}

	var live struct {
		Ranges []struct {
			RangeNum       int       `json:"rangeNum"`
			ShotNumber     int       `json:"shotNumber"`
			BestTeiler     float64   `json:"bestTeiler"`
			BestTeilerShot int       `json:"bestTeilerShot"`
			OverallSumInt  int       `json:"overallSumInt"`
			OverallSumDec  float64   `json:"overallSumDecimal"`
			SeriesSums     []float64 `json:"seriesSums"`
			ShooterName    string    `json:"shooterName"`
			Shots          []struct {
				DecValue float64 `json:"decValue"`
			} `json:"shots"`
		} `json:"ranges"`
	}
	if err := json.Unmarshal(body, &live); err != nil {
		log.Fatalf("decode live: %v", err)
	}

	failed := 0
	for rangeNum := 1; rangeNum <= *numRanges; rangeNum++ {
		want := expect[rangeNum]
		var found bool
		for _, rs := range live.Ranges {
			if rs.RangeNum != rangeNum {
				continue
			}
			found = true
			var seriesSum float64
			for _, v := range rs.SeriesSums {
				seriesSum += v
			}
			var targetSum float64
			for _, sh := range rs.Shots {
				targetSum += sh.DecValue
			}
			okBest := rs.ShotNumber == *nShots &&
				rs.BestTeilerShot == want.shot &&
				math.Abs(rs.BestTeiler-want.teiler) < 0.05
			okSum := rs.OverallSumInt == want.sumInt &&
				math.Abs(rs.OverallSumDec-want.sumDec) < 0.05 &&
				math.Abs(seriesSum-want.sumDec) < 0.15
			ok := okBest && okSum
			status := "OK"
			if !ok {
				status = "FAIL"
				failed++
			}
			log.Printf("[%s] range %d shots=%d best=%.1f #%d sum=%.1f/%d (want best=%.1f #%d sum=%.1f/%d; seriesΣ=%.1f targetΣ=%.1f [%d on scheibe])",
				status, rangeNum, rs.ShotNumber,
				rs.BestTeiler, rs.BestTeilerShot, rs.OverallSumDec, rs.OverallSumInt,
				want.teiler, want.shot, want.sumDec, want.sumInt,
				seriesSum, targetSum, len(rs.Shots))
			break
		}
		if !found {
			log.Printf("[FAIL] range %d missing from /api/live", rangeNum)
			failed++
		}
	}

	if failed > 0 {
		log.Printf("%d range(s) failed check", failed)
		os.Exit(1)
	}
	fmt.Println("all ranges: random-burst Bester + Summe OK")
}
