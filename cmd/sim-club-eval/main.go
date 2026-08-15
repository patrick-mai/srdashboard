// sim-club-eval runs accelerated matches for all games using a synthetic
// shooter field with representative means, then writes
// testdata/club-eval-results.json for post-run statistics.
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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"srdashboard/udp"
)

// Synthetic field with representative mean DecValues (spread across skill levels).
var clubField = []shooter{
	{Name: "Alex Alpha", Mean: 10.34, Range: 1},
	{Name: "Blake Bravo", Mean: 10.08, Range: 2},
	{Name: "Casey Charlie", Mean: 8.57, Range: 3},
	{Name: "Dana Delta", Mean: 8.49, Range: 4},
	{Name: "Eden Echo", Mean: 7.01, Range: 5},
	{Name: "Finn Foxtrot", Mean: 6.67, Range: 6},
}

type shooter struct {
	Name  string
	Mean  float64
	Range int
}

type gameResult struct {
	Plugin     string         `json:"plugin"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt time.Time      `json:"finishedAt"`
	DurationMs int64          `json:"durationMs"`
	ShotsSent  int            `json:"shotsSent"`
	Phase      string         `json:"phase"`
	OK         bool           `json:"ok"`
	Error      string         `json:"error,omitempty"`
	Session    map[string]any `json:"session,omitempty"`
	Notes      []string       `json:"notes,omitempty"`
}

func main() {
	httpBase := flag.String("http", "http://127.0.0.1:8080", "HTTP base")
	udpHost := flag.String("host", "127.0.0.1", "UDP host")
	udpPort := flag.Int("udp-port", 30169, "UDP port")
	outPath := flag.String("out", "testdata/club-eval-results.json", "results JSON path")
	seed := flag.Int64("seed", 42, "RNG seed")
	only := flag.String("only", "", "comma list: classic-range,autorennen,fox-on-the-run,tannebaum-einzel,tannebaum-team")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	addr := fmt.Sprintf("%s:%d", *udpHost, *udpPort)
	base := strings.TrimRight(*httpBase, "/")

	games := []string{"classic-range", "autorennen", "fox-on-the-run", "tannebaum-einzel", "tannebaum-team"}
	if *only != "" {
		games = nil
		for _, p := range strings.Split(*only, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				games = append(games, p)
			}
		}
	}

	results := make([]gameResult, 0, len(games))
	for _, plugin := range games {
		log.Printf("======== %s ========", plugin)
		res := runGame(base, addr, plugin, rng)
		results = append(results, res)
		if res.Error != "" {
			log.Printf("FAIL %s: %s", plugin, res.Error)
		} else {
			log.Printf("OK %s phase=%s shots=%d dur=%dms", plugin, res.Phase, res.ShotsSent, res.DurationMs)
		}
		time.Sleep(400 * time.Millisecond)
	}

	payload := map[string]any{
		"generatedAt": time.Now().Format(time.RFC3339),
		"seed":        *seed,
		"shooters":    clubField,
		"sourceLog":   "synthetic",
		"games":       results,
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatal(err)
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", *outPath)
}

func runGame(base, addr, plugin string, rng *rand.Rand) gameResult {
	res := gameResult{Plugin: plugin, StartedAt: time.Now()}
	defer func() {
		res.FinishedAt = time.Now()
		res.DurationMs = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
	}()

	if err := postJSON(base+"/api/plugins/activate", map[string]any{"id": plugin}); err != nil {
		res.Error = "activate: " + err.Error()
		return res
	}
	time.Sleep(200 * time.Millisecond)

	switch plugin {
	case "classic-range":
		res.ShotsSent, res.Phase, res.Error = runClassic(base, addr, rng)
	case "autorennen":
		res.ShotsSent, res.Phase, res.Error = runAutorennen(base, addr, rng)
	case "fox-on-the-run":
		_ = putPluginConfig(base, plugin, map[string]any{
			"calibrateShots": 2,
			"openingShots":   2,
			"escapeTarget":   21.0,
			"maxChaseShots":  24,
		})
		time.Sleep(150 * time.Millisecond)
		_ = postJSON(base+"/api/plugins/activate", map[string]any{"id": plugin})
		time.Sleep(200 * time.Millisecond)
		res.ShotsSent, res.Phase, res.Error = runFox(base, addr, rng)
		// restore fox pacing defaults after short-night eval
		_ = putPluginConfig(base, plugin, map[string]any{})
	case "tannebaum-einzel":
		res.ShotsSent, res.Phase, res.Error = runTannebaum(base, addr, rng, false)
	case "tannebaum-team":
		res.ShotsSent, res.Phase, res.Error = runTannebaum(base, addr, rng, true)
	default:
		res.Error = "unknown plugin"
	}

	sess, _ := getJSON(base + "/api/plugins/session")
	res.Session = sess
	if res.Phase == "" {
		res.Phase = extractPhase(sess)
	}
	res.OK = res.Error == ""
	return res
}

func runClassic(base, addr string, rng *rand.Rand) (shots int, phase string, errMsg string) {
	resetAll(base)
	for _, sh := range clubField {
		_ = postJSON(base+"/api/plugins/control", map[string]any{
			"action": "sync_live",
			"params": map[string]any{"live": map[string]any{
				fmt.Sprintf("%d", sh.Range): map[string]any{
					"shooterName":      sh.Name,
					"discipline":       "LG 20 Schuss",
					"isWarmup":         false,
					"totalShotsToFire": 20,
				},
			}},
		})
	}
	// 8 shots each for Scheibe density / last-10 readability
	for round := 0; round < 8; round++ {
		for _, sh := range clubField {
			dec := sampleDec(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			if err := udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 20 Schuss",
			}); err != nil {
				return shots, "live", err.Error()
			}
			shots++
			time.Sleep(40 * time.Millisecond)
		}
	}
	return shots, "classic-live", ""
}

func runAutorennen(base, addr string, rng *rand.Rand) (shots int, phase string, errMsg string) {
	resetAll(base)
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "reset"})
	nShots := 10
	liveWarm := map[string]any{}
	for _, sh := range clubField {
		liveWarm[fmt.Sprintf("%d", sh.Range)] = map[string]any{
			"totalShotsToFire": nShots,
			"discipline":       "LG 10 Schuss",
			"isWarmup":         true,
			"shooterName":      sh.Name,
		}
	}
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "sync_live", "params": map[string]any{"live": liveWarm}})

	for w := 0; w < 2; w++ {
		for _, sh := range clubField {
			dec := sampleDec(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				IsWarmup: true, Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 10 Schuss",
			})
			shots++
			time.Sleep(30 * time.Millisecond)
		}
	}

	liveRace := map[string]any{}
	for _, sh := range clubField {
		liveRace[fmt.Sprintf("%d", sh.Range)] = map[string]any{
			"totalShotsToFire": nShots,
			"discipline":       "LG 10 Schuss",
			"isWarmup":         false,
			"shooterName":      sh.Name,
		}
	}
	if err := postJSON(base+"/api/plugins/control", map[string]any{
		"action": "start", "params": map[string]any{"live": liveRace},
	}); err != nil {
		return shots, "arming", err.Error()
	}
	time.Sleep(300 * time.Millisecond)

	for shotNum := 1; shotNum <= nShots; shotNum++ {
		order := append([]shooter(nil), clubField...)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, sh := range order {
			dec := sampleDec(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 10 Schuss",
			})
			shots++
			time.Sleep(60 * time.Millisecond)
		}
		time.Sleep(120 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	sess, _ := getJSON(base + "/api/plugins/session?range=1")
	return shots, extractPhase(sess), ""
}

func runFox(base, addr string, rng *rand.Rand) (shots int, phase string, errMsg string) {
	resetAll(base)
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "reset"})
	// Use 5 shooters (plan concurrent field); leave range 6 inactive via no sync... but shared games use all ranges.
	// Keep all 6 for consistency with config ranges=6.
	live := map[string]any{}
	for _, sh := range clubField {
		live[fmt.Sprintf("%d", sh.Range)] = map[string]any{
			"shooterName": sh.Name,
			"discipline":  "LG 40 Schuss",
			"isWarmup":    false,
		}
	}
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "sync_live", "params": map[string]any{"live": live}})

	// calibration: 2 shots each
	for c := 0; c < 2; c++ {
		for _, sh := range clubField {
			dec := sampleDec(rng, sh.Mean)
			x, y, d := coords(rng, dec)
			_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 40 Schuss",
			})
			shots++
			time.Sleep(35 * time.Millisecond)
		}
	}
	time.Sleep(200 * time.Millisecond)
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "start", "params": map[string]any{"live": live}})
	time.Sleep(250 * time.Millisecond)

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		sess, err := getJSON(base + "/api/plugins/session?range=1")
		if err != nil {
			return shots, phase, err.Error()
		}
		phase = extractPhase(sess)
		if phase == "finished" {
			return shots, phase, ""
		}
		vm := viewModel(sess)
		hunt, _ := vm["hunt"].(map[string]any)
		if hunt == nil {
			hunt = vm
		}
		turn := intFrom(hunt["turnRange"])
		fox := intFrom(hunt["currentFox"])
		if turn <= 0 {
			if phase == "opening" && fox > 0 {
				turn = fox
			} else {
				time.Sleep(80 * time.Millisecond)
				continue
			}
		}
		sh := shooterByRange(turn)
		dec := sampleDec(rng, sh.Mean)
		// Bias fox opening/chase slightly high so rounds resolve under escapeTarget=21
		if turn == fox {
			dec = math.Min(10.9, dec+0.4)
			dec = math.Round(dec*10) / 10
		}
		x, y, d := coords(rng, dec)
		_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
			Range: turn, X: x, Y: y, Distance: d, DecValue: dec,
			Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 40 Schuss",
		})
		shots++
		time.Sleep(70 * time.Millisecond)
	}
	return shots, phase, "fox timeout"
}

func runTannebaum(base, addr string, rng *rand.Rand, team bool) (shots int, phase string, errMsg string) {
	resetAll(base)
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "reset"})
	liveWarm := map[string]any{}
	for _, sh := range clubField {
		liveWarm[fmt.Sprintf("%d", sh.Range)] = map[string]any{
			"shooterName": sh.Name,
			"discipline":  "LG 40 Schuss",
			"isWarmup":    true,
		}
	}
	_ = postJSON(base+"/api/plugins/control", map[string]any{"action": "sync_live", "params": map[string]any{"live": liveWarm}})
	for _, sh := range clubField {
		dec := sampleDec(rng, sh.Mean)
		x, y, d := coords(rng, dec)
		_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
			Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
			IsWarmup: true, Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 40 Schuss",
		})
		shots++
		time.Sleep(30 * time.Millisecond)
	}
	live := map[string]any{}
	for _, sh := range clubField {
		live[fmt.Sprintf("%d", sh.Range)] = map[string]any{
			"shooterName": sh.Name,
			"discipline":  "LG 40 Schuss",
			"isWarmup":    false,
		}
	}
	if err := postJSON(base+"/api/plugins/control", map[string]any{
		"action": "start", "params": map[string]any{"live": live},
	}); err != nil {
		return shots, "arming", err.Error()
	}
	time.Sleep(250 * time.Millisecond)

	// Scripted leaf-clear sequence mixed with capability noise so trees finish.
	// Stage A ints 5-10, B halves 8-10, C tenths 10.5-10.9
	targets := []float64{
		5, 6, 7, 8, 9, 10,
		8.0, 8.5, 9.0, 9.5, 10.0,
		10.5, 10.6, 10.7, 10.8, 10.9,
	}
	deadline := time.Now().Add(75 * time.Second)
	ti := 0
	for time.Now().Before(deadline) {
		sess, err := getJSON(base + "/api/plugins/session?range=1")
		if err != nil {
			return shots, phase, err.Error()
		}
		phase = extractPhase(sess)
		if phase == "finished" {
			_ = team // silence
			return shots, phase, ""
		}
		for _, sh := range clubField {
			var dec float64
			if ti < len(targets) {
				dec = targets[ti%len(targets)]
				// weaker shooters sometimes miss the intended leaf
				if sh.Mean < 8.0 && rng.Float64() < 0.35 {
					dec = sampleDec(rng, sh.Mean)
				} else if sh.Mean < 9.0 && dec >= 10.5 && rng.Float64() < 0.25 {
					dec = sampleDec(rng, sh.Mean)
				}
				ti++
			} else {
				dec = sampleDec(rng, sh.Mean)
			}
			x, y, d := coords(rng, dec)
			_ = udp.SendShotPacket(addr, udp.ShotPacketOpts{
				Range: sh.Range, X: x, Y: y, Distance: d, DecValue: dec,
				Shooter: sh.Name, ShotAt: time.Now(), MenuItem: "LG 40 Schuss",
			})
			shots++
			time.Sleep(45 * time.Millisecond)
		}
		time.Sleep(80 * time.Millisecond)
		if shots > 400 {
			break
		}
	}
	return shots, phase, "tannebaum timeout"
}

func resetAll(base string) {
	for r := 1; r <= 6; r++ {
		resp, err := http.Post(fmt.Sprintf("%s/api/live/reset?range=%d", base, r), "application/json", nil)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func sampleDec(rng *rand.Rand, mean float64) float64 {
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
	if lo > 10.9 {
		lo = 10.9
	}
	steps := int(math.Round((10.9 - lo) * 10))
	if steps < 0 {
		steps = 0
	}
	return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
}

func coords(rng *rand.Rand, dec float64) (x, y int, distance float64) {
	const decMax, rifleBand = 10.9, 25.0
	if dec > decMax {
		dec = decMax
	}
	band := int(math.Round((decMax - dec) * 10))
	lo := float64(band) * rifleBand
	hi := lo + rifleBand
	if dec >= decMax {
		distance = rng.Float64() * rifleBand * 0.4
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

func shooterByRange(r int) shooter {
	for _, sh := range clubField {
		if sh.Range == r {
			return sh
		}
	}
	return shooter{Name: "UDP", Mean: 8, Range: r}
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

func putPluginConfig(base, id string, overrides map[string]any) error {
	data, err := json.Marshal(map[string]any{"overrides": overrides})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, base+"/api/plugins/"+id+"/config", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put config: %s %s", resp.Status, string(b))
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
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func viewModel(sess map[string]any) map[string]any {
	if sess == nil {
		return map[string]any{}
	}
	if vm, ok := sess["viewModel"].(map[string]any); ok {
		return vm
	}
	if vm, ok := sess["ViewModel"].(map[string]any); ok {
		return vm
	}
	// SnapshotRange may nest under match/game
	for _, k := range []string{"match", "game", "session"} {
		if m, ok := sess[k].(map[string]any); ok {
			if vm, ok := m["viewModel"].(map[string]any); ok {
				return vm
			}
		}
	}
	return sess
}

func extractPhase(sess map[string]any) string {
	vm := viewModel(sess)
	for _, nest := range []string{"race", "hunt", "tree"} {
		if m, ok := vm[nest].(map[string]any); ok {
			if s, ok := m["phase"].(string); ok && s != "" {
				return s
			}
		}
	}
	for _, k := range []string{"phase", "Phase"} {
		if s, ok := vm[k].(string); ok && s != "" {
			return s
		}
	}
	// walk shallow
	var phases []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if s, ok := t["phase"].(string); ok && s != "" {
				phases = append(phases, s)
			}
			for _, vv := range t {
				walk(vv)
			}
		case []any:
			for _, vv := range t {
				walk(vv)
			}
		}
	}
	walk(sess)
	if len(phases) > 0 {
		sort.Strings(phases)
		return phases[len(phases)-1]
	}
	return ""
}

func intFrom(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		return 0
	}
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
}
