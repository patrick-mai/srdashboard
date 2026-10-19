package rwkbericht

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"srdashboard/wettkampf"
)

const formRows = 3

// FromSnapshot builds a report from the condensed-board Wettkampf snapshot.
// Passnummer, Jahrgang, Durchgang, Gruppe and Mannschaftsnummer are not in the snapshot.
func FromSnapshot(snap wettkampf.Snapshot, heimID, gastID string, now time.Time) (Bericht, error) {
	if strings.TrimSpace(heimID) == "" || strings.TrimSpace(gastID) == "" {
		return Bericht{}, fmt.Errorf("heim und gast sind erforderlich")
	}
	if heimID == gastID {
		return Bericht{}, fmt.Errorf("heim und gast müssen verschieden sein")
	}
	heim, ok := teamByID(snap, heimID)
	if !ok {
		return Bericht{}, fmt.Errorf("mannschaft %s nicht gefunden", heimID)
	}
	gast, ok := teamByID(snap, gastID)
	if !ok {
		return Bericht{}, fmt.Errorf("mannschaft %s nicht gefunden", gastID)
	}
	return Bericht{
		Datum: Today(now),
		Heim:  sideFrom(snap, heim),
		Gast:  sideFrom(snap, gast),
	}, nil
}

func teamByID(snap wettkampf.Snapshot, id string) (wettkampf.TeamView, bool) {
	for _, t := range snap.Teams {
		if t.ID == id {
			return t, true
		}
	}
	return wettkampf.TeamView{}, false
}

func sideFrom(snap wettkampf.Snapshot, team wettkampf.TeamView) Side {
	side := Side{
		Verein:    MatchVerein(team.Club),
		Disziplin: MatchDisziplin(disciplineOf(snap, team)),
		Summe:     teamScore(team),
	}
	for _, m := range team.Members {
		if !m.HasWertung || len(side.Members) >= formRows {
			continue
		}
		side.Members = append(side.Members, Shooter{
			Name:     m.Name,
			Ergebnis: memberScore(m, team.Decimal),
		})
	}
	for _, m := range extrasFor(snap, team.ID) {
		if len(side.Einzel) >= formRows {
			break
		}
		sh := Shooter{Name: m.Name}
		if m.HasWertung {
			sh.Ergebnis = rosterScore(m, team.Decimal || isAuflage(m.Discipline))
		}
		side.Einzel = append(side.Einzel, sh)
	}
	return side
}

func disciplineOf(snap wettkampf.Snapshot, team wettkampf.TeamView) string {
	for _, m := range team.Members {
		if s := strings.TrimSpace(m.Discipline); s != "" {
			return s
		}
	}
	for _, m := range extrasFor(snap, team.ID) {
		if s := strings.TrimSpace(m.Discipline); s != "" {
			return s
		}
	}
	return ""
}

func teamScore(team wettkampf.TeamView) string {
	if team.Decimal {
		return formatDec(team.SumDec)
	}
	if team.Count == 0 && team.SumInt == 0 && team.SumDec == 0 {
		return ""
	}
	return strconv.Itoa(team.SumInt)
}

func memberScore(m wettkampf.MemberView, decimal bool) string {
	return scored(m.HasWertung, m.SumInt, m.SumDec, decimal)
}

func rosterScore(r wettkampf.RosterView, decimal bool) string {
	return scored(r.HasWertung, r.SumInt, r.SumDec, decimal)
}

func scored(has bool, sumInt int, sumDec float64, decimal bool) string {
	if !has {
		return ""
	}
	if decimal {
		return formatDec(sumDec)
	}
	return strconv.Itoa(sumInt)
}

func formatDec(v float64) string {
	s := strconv.FormatFloat(math.Round(v*10)/10, 'f', 1, 64)
	return strings.ReplaceAll(s, ".", ",")
}

func isAuflage(discipline string) bool {
	d := strings.ToLower(discipline)
	return strings.Contains(d, "auflage") || strings.Contains(d, "aufgelegt")
}

// extrasFor mirrors the condensed tile: excluded shooters listed under a team.
func extrasFor(snap wettkampf.Snapshot, teamID string) []wettkampf.RosterView {
	var out []wettkampf.RosterView
	for _, r := range snap.Roster {
		if !r.Excluded {
			continue
		}
		if extraTeamID(r, snap.Teams) == teamID {
			out = append(out, r)
		}
	}
	col := collate.New(language.German)
	sort.SliceStable(out, func(i, j int) bool {
		return col.CompareString(out[i].Name, out[j].Name) < 0
	})
	return out
}

func extraTeamID(r wettkampf.RosterView, teams []wettkampf.TeamView) string {
	if r.TeamID != "" {
		for _, t := range teams {
			if t.ID == r.TeamID {
				return t.ID
			}
		}
	}
	hint := strings.ToLower(strings.TrimSpace(r.TeamHint))
	for _, t := range teams {
		if hint != "" && hint == strings.ToLower(strings.TrimSpace(t.Name)) {
			return t.ID
		}
	}
	club := strings.ToLower(strings.TrimSpace(r.Club))
	if club == "" {
		return ""
	}
	for _, t := range teams {
		if club == strings.ToLower(strings.TrimSpace(t.Club)) {
			return t.ID
		}
		for _, m := range t.Members {
			if club == strings.ToLower(strings.TrimSpace(m.Club)) {
				return t.ID
			}
		}
	}
	return ""
}
