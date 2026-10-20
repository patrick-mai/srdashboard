package main

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"srdashboard/rwkbericht"
)

// Word is one OCR token with its box in bitmap pixels.
type Word struct {
	Text       string
	X, Y, W, H float64
}

var (
	reDate = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)
	reDec  = regexp.MustCompile(`^\d{1,4}[.,]\d$`)
	rePass = regexp.MustCompile(`^\d{6,12}$`)
	reVS   = regexp.MustCompile(`(?i)^vs\.?$`)
	reAK   = regexp.MustCompile(`(?i)\(AK\)`)
)

// ParseSheet reads an OpticScore results page from OCR words.
// Passnummern are filled. Jahrgang, Disziplin, Durchgang, Gruppe and Mannschaftsnummer are not on the page.
func ParseSheet(words []Word, pageW float64) rwkbericht.Bericht {
	var b rwkbericht.Bericht
	if pageW <= 0 {
		pageW = 1000
	}
	mid := pageW / 2
	if heim, gast, ok := pairLabels(words, pageW); ok {
		b.Heim.Verein = rwkbericht.ClubLabel(heim)
		b.Gast.Verein = rwkbericht.ClubLabel(gast)
	}
	var heimShots, gastShots []shotLine
	var heimSum, gastSum string
	for _, line := range clusterLines(words) {
		if date, ok := findDate(line); ok && b.Datum == "" {
			b.Datum = date
		}
		left, right := splitMid(line, mid)
		if s, ok := loneDecimal(left); ok {
			if _, okR := loneDecimal(right); okR || len(right) == 0 {
				if heimSum == "" {
					heimSum = s
				}
			}
		}
		if s, ok := loneDecimal(right); ok && heimSum != "" && gastSum == "" {
			gastSum = s
		}
		if sh, ok := parseSide(left, true); ok {
			heimShots = append(heimShots, sh)
		}
		if sh, ok := parseSide(right, false); ok {
			gastShots = append(gastShots, sh)
		}
	}
	b.Heim.Members, b.Heim.Einzel, b.Heim.Summe = assign(heimShots, heimSum)
	b.Gast.Members, b.Gast.Einzel, b.Gast.Summe = assign(gastShots, gastSum)
	return b
}

func hasShooters(b rwkbericht.Bericht) bool {
	return len(b.Heim.Members)+len(b.Heim.Einzel)+len(b.Gast.Members)+len(b.Gast.Einzel) > 0
}

type shotLine struct {
	name, pass, result string
	value              float64
	ak                 bool
}

func clusterLines(words []Word) [][]Word {
	if len(words) == 0 {
		return nil
	}
	sorted := append([]Word(nil), words...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Y == sorted[j].Y {
			return sorted[i].X < sorted[j].X
		}
		return sorted[i].Y < sorted[j].Y
	})
	var lines [][]Word
	var cur []Word
	base := 0.0
	for _, w := range sorted {
		if len(cur) == 0 {
			cur = []Word{w}
			base = w.Y
			continue
		}
		tol := math.Max(w.H, cur[0].H) * 0.8
		if tol < 4 {
			tol = 4
		}
		if math.Abs(w.Y-base) <= tol {
			cur = append(cur, w)
			continue
		}
		lines = append(lines, sortX(cur))
		cur = []Word{w}
		base = w.Y
	}
	if len(cur) > 0 {
		lines = append(lines, sortX(cur))
	}
	return lines
}

func sortX(line []Word) []Word {
	sort.Slice(line, func(i, j int) bool { return line[i].X < line[j].X })
	return line
}

func splitMid(line []Word, mid float64) (left, right []Word) {
	for _, w := range line {
		if w.X+w.W/2 < mid {
			left = append(left, w)
		} else {
			right = append(right, w)
		}
	}
	return left, right
}

func findDate(line []Word) (string, bool) {
	for _, w := range line {
		if reDate.MatchString(strings.TrimSpace(w.Text)) {
			return strings.TrimSpace(w.Text), true
		}
	}
	return "", false
}

// pairLabels reads the "Heim vs. Gast" line. Windows OCR often places "vs."
// a few pixels below the club names, so this does not use the line clusters.
func pairLabels(words []Word, pageW float64) (heim, gast string, ok bool) {
	var vs Word
	found := false
	for _, w := range words {
		if reVS.MatchString(strings.TrimSpace(w.Text)) {
			vs = w
			found = true
			break
		}
	}
	if !found {
		return "", "", false
	}
	band := pageW * 0.02
	if band < 16 {
		band = 16
	}
	var line []Word
	for _, w := range words {
		if math.Abs(w.Y-vs.Y) <= band {
			line = append(line, w)
		}
	}
	line = sortX(line)
	return findPair(line)
}

func findPair(line []Word) (heim, gast string, ok bool) {
	vs := -1
	for i, w := range line {
		if reVS.MatchString(strings.TrimSpace(w.Text)) {
			vs = i
			break
		}
	}
	if vs < 0 {
		return "", "", false
	}
	var left, right []string
	for i, w := range line {
		t := strings.TrimSpace(w.Text)
		if reDate.MatchString(t) || rePass.MatchString(t) {
			continue
		}
		if i < vs {
			left = append(left, t)
		} else if i > vs {
			right = append(right, t)
		}
	}
	heim = strings.TrimSpace(strings.Join(left, " "))
	gast = strings.TrimSpace(strings.Join(right, " "))
	if heim == "" || gast == "" {
		return "", "", false
	}
	return heim, gast, true
}

func loneDecimal(words []Word) (string, bool) {
	if len(words) == 0 {
		return "", false
	}
	var decs []string
	for _, w := range words {
		t := strings.TrimSpace(w.Text)
		if t == "" {
			continue
		}
		if rePass.MatchString(t) || isNameToken(t) {
			return "", false
		}
		if reDec.MatchString(t) {
			decs = append(decs, t)
		}
	}
	if len(decs) != 1 {
		return "", false
	}
	return decs[0], true
}

func parseSide(words []Word, home bool) (shotLine, bool) {
	var passes, decs, names []string
	for _, w := range words {
		t := strings.TrimSpace(w.Text)
		switch {
		case rePass.MatchString(t):
			passes = append(passes, t)
		case reDec.MatchString(t):
			decs = append(decs, t)
		default:
			if isNameToken(t) {
				names = append(names, t)
			}
		}
	}
	if len(passes) == 0 || len(decs) == 0 || len(names) == 0 {
		return shotLine{}, false
	}
	sh := shotLine{name: strings.Join(names, " ")}
	if home {
		sh.pass = passes[0]
		sh.result = decs[len(decs)-1]
	} else {
		sh.pass = passes[len(passes)-1]
		sh.result = decs[0]
	}
	sh.ak = reAK.MatchString(sh.name)
	sh.name = strings.TrimSpace(reAK.ReplaceAllString(sh.name, ""))
	sh.value = parseDec(sh.result)
	if sh.name == "" {
		return shotLine{}, false
	}
	return sh, true
}

func isNameToken(t string) bool {
	if t == "" || reDec.MatchString(t) || rePass.MatchString(t) || reDate.MatchString(t) || reVS.MatchString(t) {
		return false
	}
	if strings.EqualFold(t, "M") || isDigits(t) {
		return false
	}
	return true
}

func isDigits(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseDec(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return 0
	}
	return v
}

func assign(shots []shotLine, sumText string) (members, einzel []rwkbericht.Shooter, sum string) {
	sum = strings.TrimSpace(sumText)
	var pool []shotLine
	for _, s := range shots {
		if s.ak {
			einzel = append(einzel, toShooter(s))
			continue
		}
		pool = append(pool, s)
	}
	target := parseDec(sum)
	useSum := sum != ""
	var team []shotLine
	if useSum {
		total := 0.0
		for _, s := range pool {
			total += s.value
		}
		if math.Abs(total-target) <= 0.25 {
			team = pool
		} else {
			acc := 0.0
			matched := false
			for _, s := range pool {
				if !matched && acc+s.value <= target+0.25 {
					team = append(team, s)
					acc += s.value
					if math.Abs(acc-target) <= 0.25 {
						matched = true
					}
					continue
				}
				einzel = append(einzel, toShooter(s))
			}
		}
	} else if len(pool) > 3 {
		team = pool[:3]
		for _, s := range pool[3:] {
			einzel = append(einzel, toShooter(s))
		}
	} else {
		team = pool
	}
	if len(team) > 3 {
		team = team[:3]
	}
	if len(einzel) > 3 {
		einzel = einzel[:3]
	}
	for _, s := range team {
		members = append(members, toShooter(s))
	}
	return members, einzel, sum
}

func toShooter(s shotLine) rwkbericht.Shooter {
	return rwkbericht.Shooter{Name: s.name, Passnr: s.pass, Ergebnis: s.result}
}
