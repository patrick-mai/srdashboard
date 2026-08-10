package tannebaum

import (
	"math"
	"strconv"
)

const (
	StageA = "A"
	StageB = "B"
	StageC = "C"
)

var stageOrder = []string{StageA, StageB, StageC}

// StageSpec describes one band of the Tannebaum.
type StageSpec struct {
	ID     string
	Label  string
	Min    float64
	Max    float64
	Step   float64 // 1, 0.5, or 0.1
	Leaves map[string]int // canonical value key → remaining
}

func stageSpecs() []StageSpec {
	return []StageSpec{
		{
			ID: StageA, Label: "Stufe A: 5-10",
			Min: 5, Max: 10, Step: 1,
			Leaves: map[string]int{"5.0": 1, "6.0": 1, "7.0": 1, "8.0": 1, "9.0": 1, "10.0": 1},
		},
		{
			ID: StageB, Label: "Stufe B: 8-10 (1/2)",
			Min: 8, Max: 10, Step: 0.5,
			Leaves: map[string]int{"8.0": 1, "8.5": 1, "9.0": 1, "9.5": 1, "10.0": 1},
		},
		{
			ID: StageC, Label: "Stufe C: 10.5-10.9",
			Min: 10.5, Max: 10.9, Step: 0.1,
			Leaves: map[string]int{"10.5": 1, "10.6": 1, "10.7": 1, "10.8": 1, "10.9": 1},
		},
	}
}

func cloneLeaves(src map[string]int) map[string]int {
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func leafKey(v float64) string {
	return fmtFloat1(v)
}

func parseLeafKey(k string) float64 {
	f, err := strconv.ParseFloat(k, 64)
	if err != nil {
		return 0
	}
	return f
}

func floorInt(v float64) float64 {
	return math.Floor(v + 1e-9)
}

func floorHalf(v float64) float64 {
	return math.Floor(v*2+1e-9) / 2
}

func floorTenth(v float64) float64 {
	return math.Floor(v*10+1e-9) / 10
}

// mapShotToStage down-rounds raw into a leaf value for the given stage.
// Returns ok=false when the shot falls outside that stage's band.
func mapShotToStage(raw float64, stageID string) (float64, bool) {
	switch stageID {
	case StageA:
		v := floorInt(raw)
		if v < 5 || v > 10 {
			return 0, false
		}
		return v, true
	case StageB:
		v := floorHalf(raw)
		if v < 8 || v > 10 {
			return 0, false
		}
		return v, true
	case StageC:
		v := floorTenth(raw)
		if v < 10.5 || v > 10.9 {
			return 0, false
		}
		return v, true
	default:
		return 0, false
	}
}

// shotCandidate is a down-rounded leaf a raw shot can strike.
type shotCandidate struct {
	StageID string
	Value   float64
}

// mapShotCandidates lists possible stage hits for a raw decimal, most precise first (C→B→A).
func mapShotCandidates(raw float64) []shotCandidate {
	out := make([]shotCandidate, 0, 3)
	if v, ok := mapShotToStage(raw, StageC); ok {
		out = append(out, shotCandidate{StageID: StageC, Value: v})
	}
	if v, ok := mapShotToStage(raw, StageB); ok {
		out = append(out, shotCandidate{StageID: StageB, Value: v})
	}
	if v, ok := mapShotToStage(raw, StageA); ok {
		out = append(out, shotCandidate{StageID: StageA, Value: v})
	}
	return out
}

func stageRemaining(leaves map[string]int) int {
	n := 0
	for _, c := range leaves {
		n += c
	}
	return n
}

func stageCleared(leaves map[string]int) bool {
	return stageRemaining(leaves) == 0
}

func strikeLeaf(leaves map[string]int, value float64) bool {
	if leaves == nil {
		return false
	}
	k := leafKey(value)
	if leaves[k] <= 0 {
		return false
	}
	leaves[k]--
	if leaves[k] <= 0 {
		delete(leaves, k)
	}
	return true
}

func hasLeaf(leaves map[string]int, value float64) bool {
	return leaves != nil && leaves[leafKey(value)] > 0
}

func rawShotValue(dec float64, full int) float64 {
	if dec > 0 {
		return dec
	}
	if full > 0 {
		return float64(full)
	}
	return 0
}

func fmtFloat1(v float64) string {
	r := math.Round(v*10) / 10
	i := int(math.Floor(r + 1e-9))
	frac := int(math.Round((r - float64(i)) * 10))
	if frac < 0 {
		frac = 0
	}
	if frac > 9 {
		frac = 9
	}
	return itoa(i) + "." + itoa(frac)
}
