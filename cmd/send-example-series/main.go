// One-off helper: send demo shot series to ranges over UDP (OpticScore-shaped payloads).
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

const decMax = 10.9

func main() {
	addr := "127.0.0.1:30169"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	first := []string{"Anna", "Max", "Lena", "Tom", "Sarah", "Felix", "Mia", "Jonas", "Emma", "Luca", "Sophie", "Paul", "Laura", "Tim", "Nina", "Ben"}
	last := []string{"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker", "Hoffmann", "Schulz", "Koch", "Bauer", "Richter", "Klein", "Wolf", "Schröder"}
	clubs := []string{"SV Adler", "Schützenverein Nord", "KSG Mitte", "SSC West", "BSV Ost", "SG Süd"}

	pick := func(ss []string) string { return ss[rng.Intn(len(ss))] }

	pickDec := func(lo float64) float64 {
		steps := int(math.Round((decMax - lo) * 10))
		if steps < 0 {
			steps = 0
		}
		return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
	}

	// Teiler (Distance) in DISAG units: one 0.1 DecValue step spans teilerBandDsg.
	pickShotCoords := func(dec float64, teilerBandDsg float64) (x, y int, distance float64) {
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
		// Keep Distance = sqrt(X²+Y²) as in real OpticScore logs.
		distance = math.Round(math.Hypot(float64(x), float64(y))*10) / 10
		return x, y, distance
	}

	sendSeries := func(rangeNum, nShots int, menu string, lo float64, teilerBandDsg float64) {
		fn, ln, club := pick(first), pick(last), pick(clubs)
		log.Printf("range %d: %s %s (%s) — %s, %d shots, %.1f–%.1f (teiler band %.0f DSG)",
			rangeNum, fn, ln, club, menu, nShots, lo, decMax, teilerBandDsg)
		base := time.Now()
		for i := 0; i < nShots; i++ {
			dec := pickDec(lo)
			x, y, dist := pickShotCoords(dec, teilerBandDsg)
			full := int(math.Floor(dec))
			if full > 10 {
				full = 10
			}
			if full < 0 {
				full = 0
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
				log.Printf("  range %d: %d/%d (last=%.1f teiler=%.1f)", rangeNum, i+1, nShots, dec, dist)
			}
			time.Sleep(12 * time.Millisecond)
		}
	}

	const rifleBand = 25.0
	const pistolBand = 72.0

	sendSeries(1, 30, "LG Aufgelegt", 10.0, rifleBand)
	for r := 2; r <= 3; r++ {
		sendSeries(r, 40, "LG 40 Schuss", 8.5, rifleBand)
	}
	for r := 4; r <= 6; r++ {
		sendSeries(r, 40, "LP 40 Schuss", 6.0, pistolBand)
	}
	fmt.Println("done: range1=30 LG Aufgelegt (10.0–10.9); ranges 2–3=40 LG (8.5–10.9); ranges 4–6=40 LP (6.0–10.9)")
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
