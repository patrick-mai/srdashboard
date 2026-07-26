// send-shot sends synthetic DISAG OpticScore Shot JSON over UDP for local testing
// without live range hardware.
//
// Examples:
//
//	go run ./cmd/send-shot -range 1 -dec 10
//	go run ./cmd/send-shot -ranges 1,2,3 -dec 9.5 -interval 1s
//	go run ./cmd/send-shot -range 1 -dec 10 -offset-ms 250   # ShotDateTime = now+250ms
//
// Replay real shots from an OpticScore log:
//
//	go run ./cmd/replay-log -log "C:\ProgramData\DisagOpticScore\OUT-JSONInterface.log.txt"
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"srdashboard/state"
	"srdashboard/udp"
)

func main() {
	host := flag.String("host", "127.0.0.1", "Viewer UDP host")
	port := flag.Int("port", 30169, "Viewer UDP port")
	rangeNum := flag.Int("range", 0, "Single range number (1..N)")
	rangesFlag := flag.String("ranges", "", "Comma-separated ranges (e.g. 1,2,3); overrides -range")
	dec := flag.Float64("dec", 9.0, "Shot DecValue (ring score)")
	x := flag.Int("x", 80, "Shot X (−9000..9000)")
	y := flag.Int("y", 120, "Shot Y (−9000..9000)")
	distance := flag.Float64("distance", 0, "Teiler Distance (0 = auto for ring tens)")
	shooter := flag.String("shooter", "UDP Test", "Shooter first name in payload")
	atFlag := flag.String("at", "", "ShotDateTime (yyyy-MM-dd HH:mm:ss.fff); default now when -offset-ms set, else server receive time")
	offsetMs := flag.Int("offset-ms", 0, "ShotDateTime = now + this many ms (for reaction tests)")
	interval := flag.Duration("interval", 500*time.Millisecond, "Delay between shots in multi-range / scenario mode")
	delay := flag.Duration("delay", 0, "Wait before first shot (e.g. 8s for F1 lights)")
	rounds := flag.Int("rounds", 1, "Repeat the full sequence this many times")
	warmup := flag.Bool("warmup", false, "Set IsWarmup on shots")
	scenario := flag.String("scenario", "", "Preset: f1-party (one shot per range after -delay)")
	dryRun := flag.Bool("dry-run", false, "Print JSON only; do not send UDP")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	ranges, err := resolveRanges(*rangeNum, *rangesFlag, *scenario)
	if err != nil {
		log.Fatal(err)
	}
	if len(ranges) == 0 {
		log.Fatal("no ranges: use -range N or -ranges 1,2,3")
	}

	if *scenario == "f1-party" && *delay == 0 {
		*delay = 8 * time.Second
		log.Printf("scenario f1-party: default -delay 8s (wait for lights out)")
	}

	if *delay > 0 {
		log.Printf("waiting %v before sending…", *delay)
		time.Sleep(*delay)
	}

	sent := 0
	for r := 0; r < *rounds; r++ {
		if *rounds > 1 {
			log.Printf("round %d/%d", r+1, *rounds)
		}
		for i, rng := range ranges {
			opts := udp.ShotPacketOpts{
				Range:    rng,
				X:        *x,
				Y:        *y,
				Distance: *distance,
				DecValue: *dec,
				IsWarmup: *warmup,
				Shooter:  *shooter,
			}
			if shotAt, ok := resolveShotAt(*atFlag, *offsetMs); ok {
				opts.ShotAt = shotAt
			}
			data, err := udp.BuildShotPacket(opts)
			if err != nil {
				log.Fatalf("build packet: %v", err)
			}
			if *dryRun {
				fmt.Println(string(data))
			} else {
				if err := udp.SendRawPacket(addr, data); err != nil {
					log.Fatalf("send range %d: %v", rng, err)
				}
				log.Printf("sent range %d dec=%.1f bytes=%d → %s", rng, *dec, len(data), addr)
			}
			sent++
			if i < len(ranges)-1 || r < *rounds-1 {
				time.Sleep(*interval)
			}
		}
	}
	log.Printf("done: %d packet(s)", sent)
}

func resolveRanges(single int, list, scenario string) ([]int, error) {
	if list != "" {
		return parseRangesList(list)
	}
	if single > 0 {
		return []int{single}, nil
	}
	if scenario == "f1-party" {
		return []int{1, 2, 3}, nil
	}
	return nil, nil
}

func parseRangesList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid range %q", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty -ranges")
	}
	return out, nil
}

func resolveShotAt(atFlag string, offsetMs int) (time.Time, bool) {
	if atFlag != "" {
		t, ok := state.ParseOpticScoreTime(atFlag)
		if !ok {
			log.Fatalf("invalid -at %q (use yyyy-MM-dd HH:mm:ss.fff)", atFlag)
		}
		return t, true
	}
	if offsetMs != 0 {
		return time.Now().Add(time.Duration(offsetMs) * time.Millisecond), true
	}
	return time.Time{}, false
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}
