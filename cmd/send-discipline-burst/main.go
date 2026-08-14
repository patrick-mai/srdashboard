// send-discipline-burst sends OpticScore-shaped UDP shots across ranges with
// different DSB-style programs (LG/LP/KK × freistehend/Auflage), including Probe.
//
//	go run ./cmd/send-discipline-burst
//	go run ./cmd/send-discipline-burst -already 20   # finish programs already in progress
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const decMax = 10.9

type program struct {
	Range      int
	DiscType   string
	MenuPoint  string
	MenuItem   string // concrete label → Ring Reader DSB code
	Firstname  string
	Lastname   string
	Club       string
	TeilerBand float64
	DecLo      float64
	ProgramN   int // Wertung shots in the DSB program
}

func main() {
	addr := flag.String("addr", "127.0.0.1:30169", "UDP host:port")
	warmupN := flag.Int("warmup", 5, "Probe shots per range")
	compN := flag.Int("comp", 0, "Competition shots per range (0 = full ProgramN)")
	already := flag.Int("already", 0, "Wertung shots already on each range; send the rest of the program")
	gap := flag.Duration("gap", 8*time.Millisecond, "Delay between shots")
	flag.Parse()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	programs := []program{
		{1, "LG", "Sportordnung", "LG 30 Schuss", "Anna", "Müller", "SV Adler", 25, 9.2, 30},
		{2, "LG", "Sportordnung", "LG 30 Schuss Auflage", "Max", "Schmidt", "KSG Mitte", 25, 9.7, 30},
		{3, "LP", "Sportordnung", "LP 40 Schuss", "Lena", "Fischer", "SSC West", 80, 7.0, 40},
		{4, "LP", "Sportordnung", "LP 40 Schuss Auflage", "Tom", "Weber", "BSV Ost", 80, 8.4, 40},
		{5, "KK", "Sportordnung", "KK 40 Schuss", "Sarah", "Meyer", "SG Süd", 80, 4.5, 40},
		{6, "KK", "Sportordnung", "KK Sportgewehr 40 Schuss Auflage", "Felix", "Wagner", "SV Adler", 80, 7.8, 40},
	}

	if *already > 0 {
		*warmupN = 0
	}
	remain := make([]int, len(programs))
	maxRemain := 0
	for i, p := range programs {
		want := *compN
		if want <= 0 {
			want = p.ProgramN
		}
		r := want - *already
		if r < 0 {
			r = 0
		}
		remain[i] = r
		if r > maxRemain {
			maxRemain = r
		}
	}

	log.Printf("burst → %s  (warmup=%d, already=%d)", *addr, *warmupN, *already)
	for i, p := range programs {
		log.Printf("range %d: %s %s — %s [%s] floor=%.1f remain=%d/%d → expect DSB %s",
			p.Range, p.Firstname, p.Lastname, p.MenuItem, p.DiscType, p.DecLo, remain[i], p.ProgramN, expectDSB(p.DiscType, p.MenuItem))
	}

	bases := make([]time.Time, len(programs))
	shotIdx := make([]int, len(programs))
	now := time.Now()
	for i := range programs {
		bases[i] = now
	}

	sendOne := func(i int, warmup bool) {
		p := programs[i]
		dec := pickDec(rng, p.DecLo)
		x, y, dist := pickShotCoords(rng, dec, p.TeilerBand)
		full := int(math.Floor(dec))
		if full > 10 {
			full = 10
		}
		at := bases[i].Add(time.Duration(shotIdx[i]) * 900 * time.Millisecond)
		shotIdx[i]++
		msg := map[string]any{
			"MessageType": "Event",
			"MessageVerb": "Shot",
			"Ranges":      p.Range,
			"Objects": []any{
				map[string]any{
					"X":            x,
					"Y":            y,
					"Distance":     dist,
					"FullValue":    full,
					"DecValue":     dec,
					"Range":        p.Range,
					"IsWarmup":     warmup,
					"IsHot":        !warmup,
					"IsValid":      true,
					"DiscType":     p.DiscType,
					"DiscTypeRaw":  p.DiscType,
					"ShotDateTime": at.Format("2006-01-02 15:04:05.000"),
					"Shooter": map[string]any{
						"Firstname": p.Firstname,
						"Lastname":  p.Lastname,
						"Club":      map[string]any{"Name": p.Club},
					},
					"MenuItem": map[string]any{
						"MenuPointName": p.MenuPoint,
						"MenuItemName":  p.MenuItem,
					},
				},
			},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Fatal(err)
		}
		if err := sendUDP(*addr, data); err != nil {
			log.Fatal(err)
		}
		time.Sleep(*gap)
	}

	log.Printf("warmup %d × %d ranges…", *warmupN, len(programs))
	for n := 0; n < *warmupN; n++ {
		for i := range programs {
			sendOne(i, true)
		}
	}
	log.Printf("competition remaining (max %d)…", maxRemain)
	for n := 0; n < maxRemain; n++ {
		for i := range programs {
			if n < remain[i] {
				sendOne(i, false)
			}
		}
		if (n+1)%10 == 0 {
			log.Printf("  competition +%d", n+1)
		}
	}
	fmt.Println("done — open each Bahn QR (Ring Reader): LG 1.10/1.11, LP 2.10/2.11, KK 1.40/1.41")
}

func expectDSB(discType, menu string) string {
	auflage := strings.Contains(strings.ToLower(menu), "auflage") ||
		strings.Contains(strings.ToLower(menu), "aufgelegt")
	switch discType {
	case "LG":
		if auflage {
			return "1.11"
		}
		return "1.10"
	case "LP":
		if auflage {
			return "2.11"
		}
		return "2.10"
	case "KK":
		if auflage {
			return "1.41"
		}
		return "1.40"
	default:
		return "?"
	}
}

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

func sendUDP(addr string, data []byte) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(data)
	return err
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}
