package munitionstest

import (
	"math"
	"testing"

	"srdashboard/host/logicapi"
)

// Luftgewehr: one ring is 2.5 mm from the centre shot, and 100 DSG are 1 mm.
// 0.7 rings is therefore 1.750 mm and Teiler 175.0. A good lot sits near 0.2.
const maxSpreadRings = 0.7

func TestRealisticSpreadWithin07Rings(t *testing.T) {
	t.Run("four lots", func(t *testing.T) {
		l := newLogic(t)
		sess := mustInit(t, l)
		// Centres sit off the bull, the way a real zero does. Offsets are the
		// group around that centring shot, kept inside 0.7 rings.
		lots := []struct {
			name   string
			cx, cy int
			group  [][2]int
		}{
			{"R50", 210, -80, [][2]int{
				{0, 0}, {14, -8}, {-18, 12}, {22, 16}, {-26, -14},
				{10, 32}, {-30, 20}, {28, -24}, {-12, -40}, {34, 32},
			}},
			{"R10", -160, 250, [][2]int{
				{0, 0}, {20, -15}, {-28, 24}, {36, 30}, {-40, -22},
				{18, 48}, {-44, 36}, {50, -40}, {-22, -60}, {64, 60},
			}},
			{"Club", 90, 180, [][2]int{
				{0, 0}, {30, -20}, {-40, 36}, {55, 48}, {-60, -44},
				{24, 70}, {-66, 50}, {80, -60}, {-30, -90}, {100, 90},
			}},
			{"Training", -300, -140, [][2]int{
				{0, 0}, {40, -30}, {-55, 48}, {70, 60}, {-80, -50},
				{36, 96}, {-90, 70}, {110, -80}, {-40, -120}, {175, 0},
			}},
		}
		for _, lot := range lots {
			sess = mustControl(t, l, sess, "addLot", map[string]any{"name": lot.name, "select": true})
			for _, off := range lot.group {
				sess = mustShot(t, l, sess, lot.cx+off[0], lot.cy+off[1])
			}
			a := active(t, l, sess)
			assertGroupWithin(t, lot.name, a, lot.cx, lot.cy)
			if a["shotCount"] != 10 || a["full"] != true || a["hasAbort"] != false {
				t.Fatalf("%s after 10 shots: %#v", lot.name, a)
			}
			t.Logf("%s Streukreis %0.3f mm, Teiler %0.1f (%.3f Ringe)",
				lot.name, a["teiler"].(float64)/dsgPerMm, a["teiler"].(float64), a["teiler"].(float64)/(lgRingWidthMm*dsgPerMm))
		}
	})

	t.Run("abort stays inside the band", func(t *testing.T) {
		l := newLogic(t)
		sess := mustInit(t, l)
		// Fein ends at Teiler 40 (0.16 rings). Grob opens past it in steps
		// and finishes at Teiler 172, still under 0.7 rings.
		sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "Fein", "select": true})
		fein := [][2]int{{0, 0}, {12, -6}, {-16, 10}, {18, 14}, {-8, 22}, {20, -16}, {-14, -18}, {10, 26}, {-22, 12}, {0, 40}}
		for _, off := range fein {
			sess = mustShot(t, l, sess, 400+off[0], -200+off[1])
		}
		feinView := resultByName(t, l, sess, "Fein")
		if feinView["teiler"] != 40.0 {
			t.Fatalf("Fein teiler = %v", feinView["teiler"])
		}

		sess = mustControl(t, l, sess, "addLot", map[string]any{"name": "Grob", "select": true})
		grob := [][2]int{
			{0, 0},
			{24, 10},  // still inside Fein
			{45, 30},  // first time worse
			{15, -10}, // inside the circle already shot
			{120, 90}, // 0.60 rings, several tenths worse
			{-20, 16},
			{30, -24},
			{-18, -12},
			{22, 8},
			{96, 142}, // hypot 171.5, just under 0.7 rings
		}
		var steps, scale float64
		for i, off := range grob {
			sess = mustShot(t, l, sess, -250+off[0], 300+off[1])
			a := active(t, l, sess)
			assertGroupWithin(t, "Grob", a, -250, 300)
			if i+1 < 2 {
				continue
			}
			gotSteps := asInt(a["abortSteps"])
			gotScale := a["abortScale"].(float64)
			wantSteps := stepsFromDiameters(a["diameterMm"].(float64), feinView["diameterMm"].(float64))
			if gotSteps != wantSteps {
				t.Fatalf("after shot %d teiler %v: steps %d want %d", i+1, a["teiler"], gotSteps, wantSteps)
			}
			wantScale := 1.0
			if wantSteps >= 1 {
				wantScale = 1 + float64(wantSteps)*0.5
				if wantScale > 4 {
					wantScale = 4
				}
			}
			if math.Abs(gotScale-wantScale) > 1e-9 {
				t.Fatalf("after shot %d: scale %v want %v", i+1, gotScale, wantScale)
			}
			steps, scale = float64(gotSteps), gotScale
			if i == 1 && gotSteps != 0 {
				t.Fatalf("shot still inside Fein grew the button: %#v", a)
			}
			if i == 2 && gotSteps != 1 {
				t.Fatalf("first worsening: %#v", a)
			}
			if i == 3 && gotSteps != 1 {
				t.Fatalf("shot inside the circle grew the button: %#v", a)
			}
		}
		if steps < 2 || scale <= 1 {
			t.Fatalf("group that opened toward 0.7 rings did not grow the button: steps %v scale %v", steps, scale)
		}
		a := active(t, l, sess)
		t.Logf("Grob Streukreis %0.3f mm, Teiler %0.1f (%.3f Ringe), Abbruch steps %v scale %v",
			a["teiler"].(float64)/dsgPerMm, a["teiler"].(float64), a["teiler"].(float64)/(lgRingWidthMm*dsgPerMm), a["abortSteps"], a["abortScale"])
	})
}

func assertGroupWithin(t *testing.T, name string, view map[string]any, cx, cy int) {
	t.Helper()
	shots, _ := view["shots"].([]map[string]any)
	if len(shots) == 0 {
		t.Fatalf("%s has no shots", name)
	}
	maxTeiler := 0.0
	for i, s := range shots {
		x := asInt(s["x"])
		y := asInt(s["y"])
		dx := float64(x - cx)
		dy := float64(y - cy)
		wantTeiler := 0.0
		if i > 0 {
			wantTeiler = math.Round(math.Hypot(dx, dy)*10) / 10
		}
		if math.Abs(s["dxMm"].(float64)-dx/dsgPerMm) > 1e-9 || math.Abs(s["dyMm"].(float64)-dy/dsgPerMm) > 1e-9 {
			t.Fatalf("%s shot %d mm offset = %v %v, want %.3f %.3f", name, i+1, s["dxMm"], s["dyMm"], dx/dsgPerMm, dy/dsgPerMm)
		}
		if s["teiler"] != wantTeiler {
			t.Fatalf("%s shot %d teiler = %v want %v (raw bull teiler would be %.1f)", name, i+1, s["teiler"], wantTeiler, math.Hypot(float64(x), float64(y)))
		}
		rings := wantTeiler / (lgRingWidthMm * dsgPerMm)
		if rings > maxSpreadRings+1e-9 {
			t.Fatalf("%s shot %d is %.3f rings, limit is %.1f", name, i+1, rings, maxSpreadRings)
		}
		if wantTeiler > maxTeiler {
			maxTeiler = wantTeiler
		}
	}
	if len(shots) < 2 {
		return
	}
	if view["teiler"] != maxTeiler {
		t.Fatalf("%s group teiler %v, farthest shot %v", name, view["teiler"], maxTeiler)
	}
	if rings := maxTeiler / (lgRingWidthMm * dsgPerMm); rings > maxSpreadRings+1e-9 {
		t.Fatalf("%s group %.3f rings", name, rings)
	}
}

func stepsFromDiameters(diameter, best float64) int {
	excess := (diameter/2)/lgRingWidthMm - (best/2)/lgRingWidthMm
	if excess <= 1e-9 {
		return 0
	}
	return int(math.Floor(excess/ringStep+1e-9)) + 1
}

func resultByName(t *testing.T, l *Logic, sess logicapi.SessionState, name string) map[string]any {
	t.Helper()
	vm, err := l.ViewModel(sess, 1)
	if err != nil {
		t.Fatal(err)
	}
	results, _ := vm["results"].([]map[string]any)
	for _, r := range results {
		if r["name"] == name {
			return r
		}
	}
	t.Fatalf("lot %s missing", name)
	return nil
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
