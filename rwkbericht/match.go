package rwkbericht

import (
	"regexp"
	"strings"
	"unicode"
)

// MatchVereinID returns the ComboBox entry whose leading Vereinsnummer is id.
// An unknown or ambiguous id returns "" so the form is left blank.
func MatchVereinID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var hits []string
	for _, opt := range Vereine {
		num, _, ok := strings.Cut(opt, " ")
		if ok && num == id {
			hits = append(hits, opt)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

// MatchVerein returns the catalog entry for a club label, or "" when the
// label does not pick exactly one entry.
func MatchVerein(label string) string {
	if hit := matchChoice(label, Vereine); hit != "" {
		return hit
	}
	// OpticScore prints a team index ("Dinker1"). OCR often reads that 1 as l.
	folded := fold(label)
	for _, suffix := range []string{"l", "i"} {
		if strings.HasSuffix(folded, suffix) {
			if hit := matchChoice(strings.TrimSuffix(folded, suffix), Vereine); hit != "" {
				return hit
			}
		}
	}
	return ""
}

// ClubLabel returns the catalog entry when the label matches one club,
// otherwise the trimmed label. The shipped form writes that text directly.
func ClubLabel(label string) string {
	if hit := MatchVerein(label); hit != "" {
		return hit
	}
	return strings.TrimSpace(label)
}

// MatchDisziplin maps an OpticScore discipline label onto the ComboBox list.
func MatchDisziplin(label string) string {
	l := fold(label)
	if l == "" {
		return ""
	}
	auflage := strings.Contains(l, "auflage") || strings.Contains(l, "aufgelegt")
	switch {
	case strings.Contains(l, "grosskaliber") || strings.Contains(l, "9 mm") || strings.Contains(l, "9mm"):
		return "25m Großkaliberpistole 9 mm"
	case hasToken(l, "kk") || strings.Contains(l, "kleinkaliber"):
		return "KK 50 m Auflage"
	case hasToken(l, "lp") || strings.Contains(l, "luftpistole") || strings.Contains(l, "pistole"):
		if auflage {
			return "10m Luftpistole Auflage"
		}
		return "10m Luftpistole"
	case hasToken(l, "lg") || strings.Contains(l, "luftgewehr") || strings.Contains(l, "gewehr"):
		if auflage {
			return "10m Luftgewehr Auflage"
		}
		return "10m Luftgewehr"
	default:
		return ""
	}
}

func matchChoice(query string, options []string) string {
	q := fold(query)
	if q == "" {
		return ""
	}
	for _, opt := range options {
		name := fold(stripNumber(opt))
		if q == fold(opt) || q == name {
			return opt
		}
	}
	stem := trimIndex(q)
	if stem == "" {
		return ""
	}
	var hits []string
	for _, opt := range options {
		name := fold(stripNumber(opt))
		if name == "" {
			continue
		}
		if strings.Contains(name, stem) || (len(name) >= 4 && strings.Contains(stem, name)) {
			hits = append(hits, opt)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

func hasToken(folded, token string) bool {
	for _, part := range strings.Fields(folded) {
		if part == token {
			return true
		}
	}
	return false
}

func stripNumber(s string) string {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return strings.TrimSpace(s[i:])
}

var trailingIndex = regexp.MustCompile(`(?:\s+\d+|\d+)$`)

func trimIndex(folded string) string {
	folded = strings.TrimSpace(folded)
	for {
		next := strings.TrimSpace(trailingIndex.ReplaceAllString(folded, ""))
		if next == folded {
			return folded
		}
		folded = next
	}
}

func fold(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	)
	s = repl.Replace(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
