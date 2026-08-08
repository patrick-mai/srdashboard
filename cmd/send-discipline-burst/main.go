// send-discipline-burst sends OpticScore-shaped UDP shots across ranges with
// different DSB-style programs (LG/LP/KK × freistehend/Auflage), including Probe.
//
//	go run ./cmd/send-discipline-burst
//	go run ./cmd/send-discipline-burst -addr 127.0.0.1:30169 -warmup 5 -comp 20
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
}

func main() {
	addr := flag.String("addr", "127.0.0.1:30169", "UDP host:port")
	warmupN := flag.Int("warmup", 5, "Probe shots per range")
	compN := flag.Int("comp", 20, "Competition shots per range (open series ok)")
	gap := flag.Duration("gap", 8*time.Millisecond, "Delay between shots")
	flag.Parse()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	programs := []program{
		{1, "LG", "Sportordnung", "LG 30 Schuss", "Anna", "Müller", "SV Adler", 25, 9.0},
		{2, "LG", "Sportordnung", "LG 30 Schuss Auflage", "Max", "Schmidt", "KSG Mitte", 25, 9.5},
		{3, "LP", "Sportordnung", "LP 40 Schuss", "Lena", "Fischer", "SSC West", 80, 8.0},
		{4, "LP", "Sportordnung", "LP 40 Schuss Auflage", "Tom", "Weber", "BSV Ost", 80, 8.5},
		{5, "KK", "Sportordnung", "KK 40 Schuss", "Sarah", "Meyer", "SG Süd", 80, 8.0},
		{6, "KK", "Sportordnung", "KK Sportgewehr Auflage", "Felix", "Wagner", "SV Adler", 80, 8.5},
	}

	log.Printf("burst → %s  (warmup=%d comp=%d per range)", *addr, *warmupN, *compN)
	for _, p := range programs {
		log.Printf("range %d: %s %s — %s [%s] → expect DSB %s",
			p.Range, p.Firstname, p.Lastname, p.MenuItem, p.DiscType, expectDSB(p.DiscType, p.MenuItem))
		base := time.Now()
		shotIdx := 0
		sendBlock := func(n int, warmup bool) {
			for i := 0; i < n; i++ {
				dec := pickDec(rng, p.DecLo)
				x, y, dist := pickShotCoords(rng, dec, p.TeilerBand)
				full := int(math.Floor(dec))
				if full > 10 {
					full = 10
				}
				at := base.Add(time.Duration(shotIdx) * 900 * time.Millisecond)
				shotIdx++
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
		}
		sendBlock(*warmupN, true)
		sendBlock(*compN, false)
		log.Printf("  range %d done (%d probe + %d wertung)", p.Range, *warmupN, *compN)
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
