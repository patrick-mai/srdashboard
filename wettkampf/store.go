package wettkampf

import (
	"encoding/xml"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"srdashboard/state"
)

const (
	defaultExpected = 4
	saveDelay       = 1500 * time.Millisecond
	keySep          = "||"
)

// Store holds Mannschaft roster and team config. Lane ResetRange does not touch it.
type Store struct {
	mu    sync.Mutex
	path  string
	state fileState
	timer *time.Timer
}

type fileState struct {
	XMLName         xml.Name     `xml:"wettkampf"`
	ExpectedPerTeam int          `xml:"expectedPerTeam"`
	NextTeamSeq     int          `xml:"nextTeamSeq"`
	Teams           []fileTeam   `xml:"teams>team"`
	Roster          []fileRoster `xml:"roster>entry"`
}

type fileTeam struct {
	ID   string `xml:"id"`
	Name string `xml:"name"`
}

type fileRoster struct {
	Key        string  `xml:"key"`
	Name       string  `xml:"name"`
	Club       string  `xml:"club"`
	TeamHint   string  `xml:"teamHint"`
	Discipline string  `xml:"discipline"`
	SumInt     int     `xml:"sumInt"`
	SumDec     float64 `xml:"sumDec"`
	PredInt    int     `xml:"predInt"`
	PredDec    float64 `xml:"predDec"`
	ShotsFired int     `xml:"shotsFired"`
	TotalShots int     `xml:"totalShots"`
	RangeNum   int     `xml:"rangeNum"`
	IsWarmup   bool    `xml:"isWarmup"`
	HasWertung bool    `xml:"hasWertung"`
	Locked     bool    `xml:"locked"`
	Excluded   bool    `xml:"excluded"`
	TeamID     string  `xml:"teamId"`
}

// Snapshot is the JSON view for GET /api/wettkampf and WS.
type Snapshot struct {
	ExpectedPerTeam int          `json:"expectedPerTeam"`
	Teams           []TeamView   `json:"teams"`
	Roster          []RosterView `json:"roster"`
}

// TeamView is one Mannschaft column on the tile.
type TeamView struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	SumInt   int          `json:"sumInt"`
	SumDec   float64      `json:"sumDec"`
	PredInt  int          `json:"predInt"`
	PredDec  float64      `json:"predDec"`
	Count    int          `json:"count"`
	Expected int          `json:"expected"`
	Club     string       `json:"club,omitempty"`
	Decimal  bool         `json:"decimal,omitempty"`
	Members  []MemberView `json:"members"`
}

// MemberView is a shooter assigned to a team.
type MemberView struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Club       string  `json:"club"`
	Discipline string  `json:"discipline,omitempty"`
	SumInt     int     `json:"sumInt"`
	SumDec     float64 `json:"sumDec"`
	PredInt    int     `json:"predInt"`
	PredDec    float64 `json:"predDec"`
	ShotsFired int     `json:"shotsFired"`
	TotalShots int     `json:"totalShots"`
	RangeNum   int     `json:"rangeNum"`
	Locked     bool    `json:"locked"`
	HasWertung bool    `json:"hasWertung"`
	IsWarmup   bool    `json:"isWarmup"`
}

// RosterView is the config-table row for every seen shooter.
type RosterView struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Club       string  `json:"club"`
	TeamHint   string  `json:"teamHint"`
	Discipline string  `json:"discipline"`
	SumInt     int     `json:"sumInt"`
	SumDec     float64 `json:"sumDec"`
	PredInt    int     `json:"predInt"`
	PredDec    float64 `json:"predDec"`
	ShotsFired int     `json:"shotsFired"`
	TotalShots int     `json:"totalShots"`
	RangeNum   int     `json:"rangeNum"`
	Locked     bool    `json:"locked"`
	HasWertung bool    `json:"hasWertung"`
	IsWarmup   bool    `json:"isWarmup"`
	TeamID     string  `json:"teamId"`
	Excluded   bool    `json:"excluded"`
}

// Team is a configurable Mannschaft.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RosterPatch updates assignment / lock / exclude from the tile menu.
type RosterPatch struct {
	Key      string `json:"key"`
	TeamID   string `json:"teamId"`
	Locked   *bool  `json:"locked"`
	Excluded *bool  `json:"excluded"`
}

// Patch is PUT /api/wettkampf.
type Patch struct {
	Action          string        `json:"action"`
	Name            string        `json:"name"`
	ID              string        `json:"id"`
	ExpectedPerTeam *int          `json:"expectedPerTeam"`
	Teams           []Team        `json:"teams"`
	Roster          []RosterPatch `json:"roster"`
	AssignByClub    bool          `json:"assignByClub"`
	Reset           bool          `json:"reset"`
}

func emptyState() fileState {
	return fileState{ExpectedPerTeam: defaultExpected, NextTeamSeq: 1}
}

// Open loads wettkampf.xml (missing file is empty state).
func Open(path string) (*Store, error) {
	s := &Store{path: path, state: emptyState()}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var st fileState
	if err := xml.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.ExpectedPerTeam < 0 {
		st.ExpectedPerTeam = 0
	}
	if st.ExpectedPerTeam == 0 && !strings.Contains(string(data), "expectedPerTeam") {
		st.ExpectedPerTeam = defaultExpected
	}
	if st.NextTeamSeq < 1 {
		st.NextTeamSeq = 1
		for _, t := range st.Teams {
			if n := seqFromID(t.ID); n >= st.NextTeamSeq {
				st.NextTeamSeq = n + 1
			}
		}
	}
	s.state = st
	return s, nil
}

// OpenSession starts an empty Wettkampf for a new board process.
// Previous wettkampf.xml is not loaded; the path is still used for this session's saves.
func OpenSession(path string) (*Store, error) {
	s := &Store{path: path, state: emptyState()}
	if path == "" {
		return s, nil
	}
	if err := s.Flush(); err != nil {
		return s, err
	}
	return s, nil
}

func seqFromID(id string) int {
	id = strings.TrimPrefix(id, "t")
	n, err := strconv.Atoi(id)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// RosterKey is name+club; OpticScore has no stable shooter id on Shot.
func RosterKey(name, club string) string {
	return strings.ToLower(strings.TrimSpace(name)) + keySep + strings.ToLower(strings.TrimSpace(club))
}

func groupingHint(teamName, clubName string) string {
	if s := strings.TrimSpace(teamName); s != "" {
		return s
	}
	return strings.TrimSpace(clubName)
}

// Observe upserts a live lane into the roster. Probe does not clobber Wertung.
func (s *Store) Observe(snap state.RangeSnapshot) {
	if s == nil {
		return
	}
	name := strings.TrimSpace(snap.ShooterName)
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	club := strings.TrimSpace(snap.ClubName)
	key := RosterKey(name, club)
	hint := groupingHint(snap.TeamName, club)
	idx := s.rosterIndex(key)
	var e *fileRoster
	if idx < 0 {
		s.state.Roster = append(s.state.Roster, fileRoster{Key: key, Name: name, Club: club})
		idx = len(s.state.Roster) - 1
	}
	e = &s.state.Roster[idx]
	e.Name = name
	if club != "" {
		e.Club = club
	}
	if hint != "" {
		e.TeamHint = hint
	}
	if d := strings.TrimSpace(snap.Discipline); d != "" {
		e.Discipline = d
	}
	e.RangeNum = snap.RangeNum
	e.IsWarmup = snap.IsWarmup

	if !e.Excluded && e.TeamID == "" && hint != "" {
		e.TeamID = s.ensureTeamLocked(hint)
	}

	if e.Locked {
		s.scheduleSaveLocked()
		return
	}

	if snap.IsWarmup {
		if e.HasWertung {
			s.scheduleSaveLocked()
			return
		}
		s.scheduleSaveLocked()
		return
	}

	e.HasWertung = true
	e.SumInt = snap.OverallSumInt
	e.SumDec = snap.OverallSumDec
	e.PredInt = snap.PredictionInt
	e.PredDec = snap.PredictionDec
	e.ShotsFired = snap.ShotNumber
	if snap.TotalShotsToFire > 0 {
		e.TotalShots = snap.TotalShotsToFire
	}
	if e.TotalShots > 0 && e.ShotsFired >= e.TotalShots {
		e.Locked = true
	}
	s.scheduleSaveLocked()
}

func (s *Store) rosterIndex(key string) int {
	for i := range s.state.Roster {
		if s.state.Roster[i].Key == key {
			return i
		}
	}
	return -1
}

func (s *Store) allocTeamIDLocked() string {
	if s.state.NextTeamSeq < 1 {
		s.state.NextTeamSeq = 1
	}
	id := "t" + strconv.Itoa(s.state.NextTeamSeq)
	s.state.NextTeamSeq++
	return id
}

func (s *Store) ensureTeamLocked(name string) string {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return ""
	}
	for _, t := range s.state.Teams {
		if strings.ToLower(strings.TrimSpace(t.Name)) == want {
			return t.ID
		}
	}
	id := s.allocTeamIDLocked()
	s.state.Teams = append(s.state.Teams, fileTeam{ID: id, Name: strings.TrimSpace(name)})
	return id
}

func (s *Store) scheduleSaveLocked() {
	if s.path == "" {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(saveDelay, func() { _ = s.Flush() })
}

// Flush writes wettkampf.xml immediately.
func (s *Store) Flush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	data, err := xml.MarshalIndent(&s.state, "", "  ")
	if err != nil {
		return err
	}
	doc := append([]byte(xml.Header), data...)
	doc = append(doc, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wettkampf-*.xml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(doc); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// View returns the aggregated snapshot.
func (s *Store) View() Snapshot {
	if s == nil {
		return Snapshot{ExpectedPerTeam: defaultExpected, Teams: []TeamView{}, Roster: []RosterView{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked()
}

func (s *Store) viewLocked() Snapshot {
	exp := s.state.ExpectedPerTeam
	teams := make([]TeamView, 0, len(s.state.Teams))
	for _, t := range s.state.Teams {
		tv := TeamView{
			ID:       t.ID,
			Name:     t.Name,
			Expected: exp,
			Members:  []MemberView{},
		}
		for _, e := range s.state.Roster {
			if e.Excluded || e.TeamID != t.ID {
				continue
			}
			tv.Members = append(tv.Members, memberFrom(e))
			if e.HasWertung {
				tv.SumInt += e.SumInt
				tv.SumDec += e.SumDec
				tv.PredInt += e.PredInt
				tv.PredDec += e.PredDec
				tv.Count++
			}
		}
		sort.Slice(tv.Members, func(i, j int) bool {
			return tv.Members[i].Name < tv.Members[j].Name
		})
		tv.PredDec = round1(tv.PredDec)
		tv.SumDec = round1(tv.SumDec)
		tv.Club = clubForTeam(t.ID, t.Name, s.state.Roster)
		tv.Decimal = teamUsesDecimal(tv.Members)
		teams = append(teams, tv)
	}
	roster := make([]RosterView, 0, len(s.state.Roster))
	for _, e := range s.state.Roster {
		roster = append(roster, rosterFrom(e))
	}
	sort.Slice(roster, func(i, j int) bool {
		if roster[i].Name != roster[j].Name {
			return roster[i].Name < roster[j].Name
		}
		return roster[i].Club < roster[j].Club
	})
	return Snapshot{
		ExpectedPerTeam: exp,
		Teams:           teams,
		Roster:          roster,
	}
}

func clubForTeam(id, name string, roster []fileRoster) string {
	for _, e := range roster {
		if e.Excluded || e.TeamID != id {
			continue
		}
		if c := strings.TrimSpace(e.Club); c != "" {
			return c
		}
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, e := range roster {
		if strings.ToLower(strings.TrimSpace(e.TeamHint)) != want {
			continue
		}
		if c := strings.TrimSpace(e.Club); c != "" {
			return c
		}
	}
	return ""
}

func isAuflage(discipline string) bool {
	d := strings.ToLower(discipline)
	return strings.Contains(d, "auflage") || strings.Contains(d, "aufgelegt")
}

func teamUsesDecimal(members []MemberView) bool {
	saw := false
	for _, m := range members {
		if !m.HasWertung {
			continue
		}
		saw = true
		if !isAuflage(m.Discipline) {
			return false
		}
	}
	return saw
}

func memberFrom(e fileRoster) MemberView {
	return MemberView{
		Key:        e.Key,
		Name:       e.Name,
		Club:       e.Club,
		Discipline: e.Discipline,
		SumInt:     e.SumInt,
		SumDec:     round1(e.SumDec),
		PredInt:    e.PredInt,
		PredDec:    round1(e.PredDec),
		ShotsFired: e.ShotsFired,
		TotalShots: e.TotalShots,
		RangeNum:   e.RangeNum,
		Locked:     e.Locked,
		HasWertung: e.HasWertung,
		IsWarmup:   e.IsWarmup,
	}
}

func rosterFrom(e fileRoster) RosterView {
	return RosterView{
		Key:        e.Key,
		Name:       e.Name,
		Club:       e.Club,
		TeamHint:   e.TeamHint,
		Discipline: e.Discipline,
		SumInt:     e.SumInt,
		SumDec:     round1(e.SumDec),
		PredInt:    e.PredInt,
		PredDec:    round1(e.PredDec),
		ShotsFired: e.ShotsFired,
		TotalShots: e.TotalShots,
		RangeNum:   e.RangeNum,
		Locked:     e.Locked,
		HasWertung: e.HasWertung,
		IsWarmup:   e.IsWarmup,
		TeamID:     e.TeamID,
		Excluded:   e.Excluded,
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// Apply applies a config patch from the tile menu and saves immediately.
func (s *Store) Apply(p Patch) Snapshot {
	if s == nil {
		return Snapshot{Teams: []TeamView{}, Roster: []RosterView{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	action := strings.ToLower(strings.TrimSpace(p.Action))
	if p.Reset || action == "reset" {
		s.state.Roster = nil
	}
	if action == "addteam" {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = "Mannschaft"
		}
		s.ensureTeamLocked(name)
	}
	if action == "deleteteam" && strings.TrimSpace(p.ID) != "" {
		s.deleteTeamLocked(p.ID)
	}
	if p.ExpectedPerTeam != nil {
		n := *p.ExpectedPerTeam
		if n < 0 {
			n = 0
		}
		s.state.ExpectedPerTeam = n
	}
	if p.Teams != nil {
		s.replaceTeamsLocked(p.Teams)
	}
	for _, rp := range p.Roster {
		idx := s.rosterIndex(rp.Key)
		if idx < 0 {
			continue
		}
		e := &s.state.Roster[idx]
		if rp.Excluded != nil {
			e.Excluded = *rp.Excluded
		}
		if e.Excluded {
			if rp.TeamID != "" {
				e.TeamID = rp.TeamID
			}
		} else {
			e.TeamID = rp.TeamID
		}
		if rp.Locked != nil {
			e.Locked = *rp.Locked
		}
	}
	if p.AssignByClub || action == "assignbyclub" {
		s.assignByClubLocked()
	}
	_ = s.saveLocked()
	return s.viewLocked()
}

func (s *Store) deleteTeamLocked(id string) {
	keep := s.state.Teams[:0]
	for _, t := range s.state.Teams {
		if t.ID != id {
			keep = append(keep, t)
		}
	}
	s.state.Teams = keep
	for i := range s.state.Roster {
		if s.state.Roster[i].TeamID == id {
			s.state.Roster[i].TeamID = ""
		}
	}
}

func (s *Store) replaceTeamsLocked(teams []Team) {
	seen := make(map[string]bool)
	out := make([]fileTeam, 0, len(teams))
	for _, t := range teams {
		id := strings.TrimSpace(t.ID)
		name := strings.TrimSpace(t.Name)
		if id == "" {
			if name == "" {
				continue
			}
			id = s.allocTeamIDLocked()
		}
		if name == "" {
			name = id
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, fileTeam{ID: id, Name: name})
	}
	s.state.Teams = out
	for i := range s.state.Roster {
		if s.state.Roster[i].TeamID != "" && !seen[s.state.Roster[i].TeamID] {
			s.state.Roster[i].TeamID = ""
		}
	}
}

func (s *Store) assignByClubLocked() {
	for i := range s.state.Roster {
		e := &s.state.Roster[i]
		if e.Excluded {
			continue
		}
		if e.Locked && e.TeamID != "" {
			continue
		}
		hint := groupingHint(e.TeamHint, e.Club)
		if hint == "" {
			continue
		}
		e.TeamID = s.ensureTeamLocked(hint)
	}
}
