// sim-f1-balance: 10 F1 logics with DRS/overtake variables.
//
// Key finding: flat DRS boost helps leaders reclaim, so beginners rarely hold P1.
// Variants explore underdog DRS stacking: extra DRS pace only when the chaser's
// running mean Dec is worse than the car ahead (catch-up without lottery).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
)

const (
	decMax    = 10.9
	stintSize = 10
	nShots    = 40
	nRanges   = 6
	pivot     = 8.0
)

type shooter struct {
	name     string
	lo       float64
	advanced bool
}

var field = []shooter{
	{"Adv-Anna", 7.0, true},
	{"Adv-Max", 7.0, true},
	{"Adv-Lena", 7.0, true},
	{"Beg-Tom", 4.0, false},
	{"Beg-Mia", 4.0, false},
	{"Beg-Jonas", 4.0, false},
}

type logicVariant struct {
	ID               int
	Name             string
	OvertakeRatio    float64
	DRSSections      []int
	PaceCompress     float64
	DRSStackPerPlace float64
	UnderdogOnly     bool
	Note             string
}

type car struct {
	rn, pos          int
	lastSpeed        float64
	shots            int
	highStreak       int
	streakBoostUntil int
	meanSum          float64
	advanced         bool
	name             string
}

func (c *car) mean() float64 {
	if c.shots == 0 {
		return 0
	}
	return c.meanSum / float64(c.shots)
}

type raceResult struct {
	BegWin         bool
	BegPodium      bool
	AvgAdvPos      float64
	AvgBegPos      float64
	SkillUpset     bool
	WinnerMeanDec  float64
	BestMeanDec    float64
	Order          []string
}

type variantStats struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	OvertakeRatio    float64  `json:"overtakeRatio"`
	DRSSections      []int    `json:"drsSections"`
	DRSCoveragePct   float64  `json:"drsCoveragePct"`
	PaceCompress     float64  `json:"paceCompress"`
	DRSStackPerPlace float64  `json:"drsStackPerPlace"`
	UnderdogOnly     bool     `json:"underdogOnly"`
	Note             string   `json:"note"`
	Races            int      `json:"races"`
	BegWinRate       float64  `json:"begWinRate"`
	BegPodiumRate    float64  `json:"begPodiumRate"`
	AdvWinRate       float64  `json:"advWinRate"`
	AvgAdvPos        float64  `json:"avgAdvPos"`
	AvgBegPos        float64  `json:"avgBegPos"`
	UpsetRate        float64  `json:"upsetRate"`
	SkillAlignRate   float64  `json:"skillAlignRate"`
	Verdict          string   `json:"verdict"`
	SampleOrder      []string `json:"sampleOrder"`
}

func variants() []logicVariant {
	spa := []int{3, 8, 9}
	wide := []int{2, 3, 4, 7, 8}
	return []logicVariant{
		{1, "Live baseline", 1.20, spa, 1.00, 0.00, false, "Current live: pace=Dec, ratio 1.2, Spa DRS"},
		{2, "Soft overtake only", 1.08, spa, 1.00, 0.00, false, "Easier non-DRS passes; no compress/stack"},
		{3, "Wide DRS only", 1.20, wide, 1.00, 0.00, false, "More DRS sections; live pass math"},
		{4, "Underdog mild", 1.25, spa, 0.90, 0.08, true, "Small underdog DRS stack"},
		{5, "Underdog medium", 1.28, spa, 0.75, 0.11, true, "Medium underdog stack + compress"},
		{6, "Underdog strong", 1.30, wide, 0.65, 0.12, true, "Strong underdog aid — watch upset rate"},
		{7, "Sweet spot A", 1.12, wide, 0.50, 0.12, false, "Soft ratio + compress + flat stack"},
		{8, "Sweet spot B", 1.05, wide, 0.45, 0.14, false, "Prior balanced hit (flat stack)"},
		{9, "Skill-leaning", 1.35, spa, 0.70, 0.08, true, "Harder passes; advanced should dominate"},
		{10, "Lottery", 1.02, wide, 0.35, 0.16, false, "Very open — expect coin-flip wins"},
	}
}

func pickDec(rng *rand.Rand, lo float64) float64 {
	steps := int(math.Round((decMax - lo) * 10))
	if steps < 0 {
		steps = 0
	}
	return math.Round((lo+float64(rng.Intn(steps+1))/10)*10) / 10
}

func toPace(dec, compress float64) float64 {
	return pivot + (dec-pivot)*compress
}

func sectionOfShot(shotNum int) int {
	if shotNum < 1 {
		shotNum = 1
	}
	return ((shotNum - 1) % stintSize) + 1
}

func inDRS(section int, sections []int) bool {
	for _, s := range sections {
		if s == section {
			return true
		}
	}
	return false
}

func tryPass(cars []*car, me *car, speed float64, section int, v logicVariant) {
	if me.pos <= 1 {
		return
	}
	var ahead *car
	for _, c := range cars {
		if c.pos == me.pos-1 {
			ahead = c
			break
		}
	}
	if ahead == nil {
		return
	}
	can := false
	if inDRS(section, v.DRSSections) {
		boost := 1.0
		underdog := me.mean()+0.05 < ahead.mean()
		if v.DRSStackPerPlace > 0 && (!v.UnderdogOnly || underdog) {
			boost = 1.0 + float64(me.pos-1)*v.DRSStackPerPlace
		}
		if speed*boost >= ahead.lastSpeed {
			can = true
		}
	}
	if !can && speed >= ahead.lastSpeed*v.OvertakeRatio {
		can = true
	}
	if can {
		me.pos, ahead.pos = ahead.pos, me.pos
	}
}

func runRace(v logicVariant, rng *rand.Rand) raceResult {
	cars := make([]*car, nRanges)
	for i, sh := range field {
		cars[i] = &car{rn: i + 1, name: sh.name, advanced: sh.advanced}
	}

	type scored struct {
		c *car
		s float64
	}
	grid := make([]scored, 0, nRanges)
	order := []int{0, 1, 2, 3, 4, 5}
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	for _, ix := range order {
		dec := pickDec(rng, field[ix].lo)
		cars[ix].meanSum += dec
		cars[ix].shots = 1
		pace := toPace(dec, v.PaceCompress)
		cars[ix].lastSpeed = pace
		if dec >= 9.0 {
			cars[ix].highStreak = 1
		}
		grid = append(grid, scored{cars[ix], pace})
	}
	sort.Slice(grid, func(i, j int) bool {
		if grid[i].s != grid[j].s {
			return grid[i].s > grid[j].s
		}
		return grid[i].c.rn < grid[j].c.rn
	})
	for i, g := range grid {
		g.c.pos = i + 1
	}

	for round := 2; round <= nShots; round++ {
		section := sectionOfShot(round)
		isPit := section == stintSize
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, ix := range order {
			c := cars[ix]
			dec := pickDec(rng, field[ix].lo)
			c.meanSum += dec
			c.shots++

			boost := 1.0
			if c.streakBoostUntil >= c.shots {
				boost = 1.15
			}
			pace := toPace(dec, v.PaceCompress)
			speed := pace * boost
			if isPit {
				speed = pace * 2.0
			}
			c.lastSpeed = speed
			tryPass(cars, c, speed, section, v)

			if !isPit {
				if dec >= 9.0 {
					c.highStreak++
					if c.highStreak >= 3 {
						c.streakBoostUntil = c.shots + 3
						c.highStreak = 0
					}
				} else {
					c.highStreak = 0
				}
			}
		}
	}

	sort.Slice(cars, func(i, j int) bool { return cars[i].pos < cars[j].pos })
	bestMean := -1.0
	avgAdv, avgBeg := 0.0, 0.0
	nAdv, nBeg := 0, 0
	for _, c := range cars {
		m := c.mean()
		if m > bestMean {
			bestMean = m
		}
		if c.advanced {
			avgAdv += float64(c.pos)
			nAdv++
		} else {
			avgBeg += float64(c.pos)
			nBeg++
		}
	}
	avgAdv /= float64(nAdv)
	avgBeg /= float64(nBeg)
	win := cars[0]
	winMean := win.mean()
	begPodium := false
	for i := 0; i < 3; i++ {
		if !cars[i].advanced {
			begPodium = true
			break
		}
	}
	orderNames := make([]string, nRanges)
	for i, c := range cars {
		tag := "B"
		if c.advanced {
			tag = "A"
		}
		orderNames[i] = fmt.Sprintf("P%d %s(%s μ=%.1f)", c.pos, c.name, tag, c.mean())
	}
	return raceResult{
		BegWin:        !win.advanced,
		BegPodium:     begPodium,
		AvgAdvPos:     avgAdv,
		AvgBegPos:     avgBeg,
		SkillUpset:    winMean+0.15 < bestMean,
		WinnerMeanDec: winMean,
		BestMeanDec:   bestMean,
		Order:         orderNames,
	}
}

func verdictOf(s variantStats) string {
	begOK := s.BegWinRate >= 0.11 && s.BegWinRate <= 0.38
	podiumOK := s.BegPodiumRate >= 0.40 && s.BegPodiumRate <= 0.82
	skillOK := s.UpsetRate <= 0.43 && s.SkillAlignRate >= 0.48
	advLead := s.AvgAdvPos+0.10 < s.AvgBegPos
	switch {
	case begOK && podiumOK && skillOK && advLead:
		return "balanced"
	case s.BegWinRate < 0.08 && s.BegPodiumRate < 0.35:
		return "adv-locked"
	case s.BegWinRate > 0.42 || s.UpsetRate > 0.50:
		return "too-lucky"
	case !advLead && s.BegWinRate >= 0.18:
		return "beg-favored"
	default:
		return "skewed"
	}
}

func main() {
	nRaces := flag.Int("races", 300, "Races per logic variant")
	seed := flag.Int64("seed", 42, "RNG seed")
	flag.Parse()

	base := rand.New(rand.NewSource(*seed))
	out := make([]variantStats, 0, len(variants()))

	for _, v := range variants() {
		begWins, begPods, upsets, skillAlign := 0, 0, 0, 0
		sumAdv, sumBeg := 0.0, 0.0
		var sample []string
		for i := 0; i < *nRaces; i++ {
			rng := rand.New(rand.NewSource(base.Int63()))
			res := runRace(v, rng)
			if res.BegWin {
				begWins++
			}
			if res.BegPodium {
				begPods++
			}
			if res.SkillUpset {
				upsets++
			}
			if res.WinnerMeanDec+0.15 >= res.BestMeanDec {
				skillAlign++
			}
			sumAdv += res.AvgAdvPos
			sumBeg += res.AvgBegPos
			if i == 0 {
				sample = res.Order
			}
		}
		n := float64(*nRaces)
		cov := 100.0 * float64(len(v.DRSSections)) / float64(stintSize-1)
		st := variantStats{
			ID:               v.ID,
			Name:             v.Name,
			OvertakeRatio:    v.OvertakeRatio,
			DRSSections:      append([]int(nil), v.DRSSections...),
			DRSCoveragePct:   math.Round(cov*10) / 10,
			PaceCompress:     v.PaceCompress,
			DRSStackPerPlace: v.DRSStackPerPlace,
			UnderdogOnly:     v.UnderdogOnly,
			Note:             v.Note,
			Races:            *nRaces,
			BegWinRate:       math.Round(1000*float64(begWins)/n) / 1000,
			BegPodiumRate:    math.Round(1000*float64(begPods)/n) / 1000,
			AdvWinRate:       math.Round(1000*float64(*nRaces-begWins)/n) / 1000,
			AvgAdvPos:        math.Round(100*sumAdv/n) / 100,
			AvgBegPos:        math.Round(100*sumBeg/n) / 100,
			UpsetRate:        math.Round(1000*float64(upsets)/n) / 1000,
			SkillAlignRate:   math.Round(1000*float64(skillAlign)/n) / 1000,
			SampleOrder:      sample,
		}
		st.Verdict = verdictOf(st)
		out = append(out, st)
		fmt.Fprintf(os.Stderr, "#%2d %-22s ratio=%.2f cmp=%.2f stack=%.2f und=%v  win=%5.1f%% pod=%5.1f%% upset=%5.1f%% → %s\n",
			st.ID, st.Name, st.OvertakeRatio, st.PaceCompress, st.DRSStackPerPlace, st.UnderdogOnly,
			st.BegWinRate*100, st.BegPodiumRate*100, st.UpsetRate*100, st.Verdict)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"seed":            *seed,
		"racesPerVariant": *nRaces,
		"shots":           nShots,
		"advancedDec":     "7.0–10.9 uniform",
		"beginnerDec":     "4.0–10.9 uniform",
		"paceFormula":     "pace = 8.0 + (dec-8.0)*paceCompress",
		"drsBoostFormula": "in DRS: chase *= 1+(pos-1)*stack; underdogOnly limits stack to lower mean Dec",
		"target":          "beginner wins 12–38%, podium 40–80%, upset ≤42%, advanced better avg position",
		"variants":        out,
	})
}
