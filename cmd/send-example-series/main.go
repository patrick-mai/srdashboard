// One-off helper: send demo shot series to all ranges over UDP.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"time"
)

func main() {
	addr := "127.0.0.1:30169"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	first := []string{"Anna", "Max", "Lena", "Tom", "Sarah", "Felix", "Mia", "Jonas", "Emma", "Luca", "Sophie", "Paul", "Laura", "Tim", "Nina", "Ben"}
	last := []string{"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker", "Hoffmann", "Schulz", "Koch", "Bauer", "Richter", "Klein", "Wolf", "Schröder"}
	clubs := []string{"SV Adler", "Schützenverein Nord", "KSG Mitte", "SSC West", "BSV Ost", "SG Süd"}

	pick := func(ss []string) string { return ss[rng.Intn(len(ss))] }

	// DecValue from lo..10.9 in 0.1 steps
	pickDec := func(lo float64) float64 {
		steps := int(math.Round((10.9 - lo) * 10))
		return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
	}

	teiler := func(dec float64) float64 {
		var d float64
		switch {
		case dec >= 10.9:
			d = 0.05 + rng.Float64()*0.08
		case dec >= 10.0:
			d = 0.15 + (10.9-dec)*0.45 + rng.Float64()*0.15
		default:
			d = 1.8 + (10.0-dec)*3.2 + rng.Float64()*1.2
		}
		return math.Round(d*100) / 100
	}

	sendSeries := func(rangeNum, nShots int, menu string, lo float64) {
		fn, ln, club := pick(first), pick(last), pick(clubs)
		log.Printf("range %d: %s %s (%s) — %s, %d shots, %.1f–10.9", rangeNum, fn, ln, club, menu, nShots, lo)
		base := time.Now()
		for i := 0; i < nShots; i++ {
			dec := pickDec(lo)
			dist := teiler(dec)
			angle := rng.Float64() * 2 * math.Pi
			radius := dist * 90
			x := int(math.Cos(angle) * radius)
			y := int(math.Sin(angle) * radius)
			full := int(math.Floor(dec))
			if full > 10 {
				full = 10
			}
			msg := map[string]any{
				"MessageType": "Event",
				"MessageVerb": "Shot",
				"Ranges":      rangeNum,
				"Objects": []any{
					map[string]any{
						"X":            x,
						"Y":            y,
						"Distance":     dist,
						"FullValue":    full,
						"DecValue":     dec,
						"Range":        rangeNum,
						"IsWarmup":     false,
						"ShotDateTime": base.Add(time.Duration(i) * 700 * time.Millisecond).Format("2006-01-02 15:04:05.000"),
						"Shooter": map[string]any{
							"Firstname": fn,
							"Lastname":  ln,
							"Club":      map[string]any{"Name": club},
						},
						"MenuItem": map[string]any{
							"MenuItemName": menu,
						},
					},
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				log.Fatal(err)
			}
			if err := sendUDP(addr, data); err != nil {
				log.Fatal(err)
			}
			if (i+1)%10 == 0 || i+1 == nShots {
				log.Printf("  range %d: %d/%d (last=%.1f)", rangeNum, i+1, nShots, dec)
			}
			time.Sleep(12 * time.Millisecond)
		}
	}

	sendSeries(1, 30, "LG Auflage 30 Schuss", 10.0)
	for r := 2; r <= 6; r++ {
		sendSeries(r, 40, "LG 40 Schuss", 8.5)
	}
	fmt.Println("done: range1=30 LG Auflage (10.0–10.9), ranges 2–6=40 shots (8.5–10.9)")
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
