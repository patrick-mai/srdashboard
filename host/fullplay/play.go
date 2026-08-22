package fullplay

import (
	"testing"
)

func playClassicRange(t *testing.T, h *Host) {
	t.Helper()
	for i := 1; i <= h.NumRanges; i++ {
		h.Fire(i, 10.4)
	}
	h.requireBothRangesPaint()
	vm := h.VM(1)
	if vm["kind"] != "display" {
		t.Fatalf("classic-range kind=%v", vm["kind"])
	}
}

func playLudo(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	game := nest(h.VM(1), "game")
	if game == nil {
		t.Fatal("missing game view model")
	}
	arms := asInt(game["boardArms"])
	ring := asInt(game["ringSize"])
	if h.NumRanges < 4 {
		if arms != 4 {
			t.Fatalf("2/3-player Ludo boardArms=%d want 4 (classic cross, not a 2-point star)", arms)
		}
		if ring < 40 {
			t.Fatalf("2-player Ludo ringSize=%d want >= 40 fields", ring)
		}
	} else if ring < h.NumRanges*8 {
		t.Fatalf("Ludo ringSize=%d too small for %d players", ring, h.NumRanges)
	}
	h.requireBothRangesPaint()

	h.Fire(1, 10.0)
	for i := 0; i < 80 && h.phase([]string{"game", "phase"}) != "finished"; i++ {
		h.Fire(1, 10.5)
	}
	h.requirePhase([]string{"game", "phase"}, "finished")
	if asInt(nest(h.VM(1), "game")["winnerRange"]) != 1 {
		t.Fatalf("winner=%v want stand 1", nest(h.VM(1), "game")["winnerRange"])
	}
}

func playBarrikade(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	h.requireBothRangesPaint()
	prev := -1
	for i := 0; i < 80 && h.phase([]string{"game", "phase"}) != "finished"; i++ {
		game := nest(h.VM(1), "game")
		cell := 0
		for _, p := range asSlice(game["players"]) {
			pm, _ := p.(map[string]any)
			if asInt(pm["rangeNum"]) == 1 {
				cell = asInt(pm["cell"])
				break
			}
		}
		if cell == prev {
			h.Fire(1, 10.0)
		} else {
			h.Fire(1, 10.5)
		}
		prev = cell
	}
	h.requirePhase([]string{"game", "phase"}, "finished")
}

func playBingo(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	h.requireBothRangesPaint()
	vals := bingoCardValues(nest(h.VM(1), "game"))
	if len(vals) < 5 {
		t.Fatalf("bingo card values=%d want 25", len(vals))
	}
	for _, v := range vals[:5] {
		h.Fire(1, v)
	}
	h.requirePhase([]string{"game", "phase"}, "finished")
}

func bingoCardValues(game map[string]any) []float64 {
	if game == nil {
		return nil
	}
	switch t := game["cardValues"].(type) {
	case []float64:
		return t
	case []any:
		out := make([]float64, 0, len(t))
		for _, x := range t {
			switch n := x.(type) {
			case float64:
				out = append(out, n)
			case int:
				out = append(out, float64(n))
			}
		}
		return out
	default:
		return nil
	}
}

var tannebaumClear = []float64{
	5, 6, 7, 8.0, 8.5, 9.0, 9.5, 10.0, 8.1, 9.1, 10.1, 10.5, 10.6, 10.7, 10.8, 10.9,
}

func playTannebaumEinzel(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	tree := nest(h.VM(1), "tree")
	if tree == nil {
		t.Fatal("missing tree view model")
	}
	if n := len(asSlice(tree["contenders"])); n != h.NumRanges {
		t.Fatalf("einzel contenders=%d want %d (hall gallery needs every stand)", n, h.NumRanges)
	}
	h.requireBothRangesPaint()
	for _, v := range tannebaumClear {
		h.Fire(1, v)
	}
	h.requirePhase([]string{"tree", "phase"}, "finished")
}

func playTannebaumTeam(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	tree := nest(h.VM(1), "tree")
	if n := len(asSlice(tree["contenders"])); n != 2 {
		t.Fatalf("team contenders=%d want 2", n)
	}
	h.requireBothRangesPaint()
	for _, v := range tannebaumClear {
		h.Fire(1, v)
	}
	h.requirePhase([]string{"tree", "phase"}, "finished")
}

func playFox(t *testing.T, h *Host) {
	t.Helper()
	for i := 0; i < 5; i++ {
		for r := 1; r <= h.NumRanges; r++ {
			h.Fire(r, 8.0)
		}
	}
	if h.phase([]string{"hunt", "phase"}) != "opening" && h.phase([]string{"hunt", "phase"}) != "chase" {
		h.Start()
	}
	h.requireBothRangesPaint()
	for i := 0; i < 400 && h.phase([]string{"hunt", "phase"}) != "finished"; i++ {
		hunt := nest(h.VM(1), "hunt")
		turn := asInt(hunt["turnRange"])
		fox := asInt(hunt["currentFox"])
		if turn < 1 {
			h.Start()
			continue
		}
		val := 10.0
		if turn == fox {
			val = 1.2
		}
		h.Fire(turn, val)
	}
	h.requirePhase([]string{"hunt", "phase"}, "finished")
	shots := asSlice(nest(h.VM(1), "hunt")["recentShots"])
	if len(shots) == 0 {
		t.Fatal("fox finished without shots on the disc")
	}
	last, _ := shots[len(shots)-1].(map[string]any)
	if last == nil || (asInt(last["x"]) == 0 && asInt(last["y"]) == 0) {
		t.Fatalf("fox disc shots missing coordinates: %#v", last)
	}
}

func playAutorennen(t *testing.T, h *Host) {
	t.Helper()
	h.Start()
	if p := h.phase([]string{"race", "phase"}); p != "racing" && p != "finished" {
		t.Fatalf("autorennen after start phase=%q", p)
	}
	h.requireBothRangesPaint()
	budget := h.totalShots
	if budget < 1 {
		budget = 10
	}
	for n := 0; n < budget; n++ {
		if h.phase([]string{"race", "phase"}) == "finished" {
			break
		}
		for r := 1; r <= h.NumRanges; r++ {
			h.Fire(r, 10.3)
		}
	}
	h.requirePhase([]string{"race", "phase"}, "finished")
}
