package kopokal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"srdashboard/host/games/gameutil"
	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func init() {
	loader.RegisterBuiltin("ko-pokal", func(m *loader.Manifest) logicapi.Logic { return New(m) })
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }
func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "ko-pokal"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "KO-Pokal"
}
func (l *Logic) Version() string { return "1.0.0" }
func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true, "qualifyShots": 5, "pointsToWin": 2,
		"holeInHoleMinOverlap": 0.5, "holeInHoleMinValue": 8.5, "shotDiameterMm": 4.5, "dsgPerMm": 100.0,
		"defaultTargetProfile": "air_rifle_10m", "disciplineTargets": gameutil.DefaultDisciplineTargets(),
		"handicaps": map[string]any{},
	}
}
func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"qualifyShots": map[string]any{"type": "integer"},
		"pointsToWin":  map[string]any{"type": "integer"},
	}}
}

type Player struct {
	RangeNum, ShotsFired, QualifyCount, MatchShots        int
	Active, Seated, Ready, WasWarmup, HasLast, Eliminated bool
	ShooterName, Discipline, Color, Hint, LastNote        string
	Handicap, LastRaw, QualifySum                         float64
	LastX, LastY                                          int
}

type ShotMark struct {
	RangeNum int     `json:"rangeNum"`
	Raw      float64 `json:"raw"`
	Result   string  `json:"result"`
	Note     string  `json:"note"`
	X, Y     int
	Color    string `json:"color"`
}

type Match struct {
	RoundLabel  string  `json:"roundLabel"`
	Left        int     `json:"left"`
	RightNum    int     `json:"right"`
	WinnerNum   int     `json:"winner"`
	LeftPoints  int     `json:"leftPoints"`
	RightPoints int     `json:"rightPoints"`
	LeftHas     bool    `json:"leftHas"`
	RightHas    bool    `json:"rightHas"`
	LeftRaw     float64 `json:"leftRaw"`
	RightRaw    float64 `json:"rightRaw"`
	LeftHiH     bool    `json:"leftHiH"`
	RightHiH    bool    `json:"rightHiH"`
}

type GameState struct {
	Phase, StatusLine, StartBlockedReason string
	NumRanges, WinnerRange, CurrentIdx    int
	FieldOpen, QualDone                   bool
	Config                                map[string]any
	Players                               map[string]*Player
	Shots                                 []ShotMark
	Bracket                               []Match
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(l.DefaultConfig(), cfg)
	gs := &GameState{Phase: gameutil.PhaseWarmup, NumRanges: gameutil.CfgInt(merged, "numRanges", 6), Config: merged, Players: map[string]*Player{}, StatusLine: "KO-Pokal"}
	gs.ensure()
	return marshalState(gs)
}
func (l *Logic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	return l.OnShotCtx(sess, logicapi.ShotContext{RangeNum: rangeNum, Shot: shot, ShotIndex: shotIndex, Live: logicapi.LiveRangeInfo{IsWarmup: shot.IsWarmup}, Now: time.Now()})
}
func (l *Logic) OnShotCtx(sess logicapi.SessionState, ctx logicapi.ShotContext) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	gs.ensure()
	var events []logicapi.PluginEvent
	if ctx.InactiveRanges != nil {
		gs.applyInactiveList(ctx.InactiveRanges)
	}
	p := gs.Players[gameutil.Itoa(ctx.RangeNum)]
	if p == nil || !p.Active {
		return marshalWithEvents(gs, nil)
	}
	if ctx.Live.ShooterName != "" {
		p.ShooterName = ctx.Live.ShooterName
	}
	raw := gameutil.RawShotValue(ctx.Shot.DecValue, ctx.Shot.FullValue)
	discard, openNow := gameutil.WarmupDiscard(gs.FieldOpen, gameutil.IsWarmupShot(ctx.Live.IsWarmup, ctx.Shot.IsWarmup))
	if discard {
		p.WasWarmup = true
		return marshalWithEvents(gs, events)
	}
	if openNow {
		gs.openCompetitionField()
		if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
			events = append(events, gs.beginPlay()...)
		}
	}
	if gs.Phase != gameutil.PhasePlaying || !p.Seated {
		return marshalWithEvents(gs, events)
	}
	hih := false
	if p.HasLast {
		if ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config); ok {
			hih = true
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{"rangeNum": p.RangeNum, "overlap": ov}})
		}
	}
	p.LastRaw, p.LastX, p.LastY, p.HasLast = raw, ctx.Shot.X, ctx.Shot.Y, true
	p.ShotsFired++
	note := fmt.Sprintf("%.1f", raw)
	if hih {
		note = "Hole-in-Hole"
	}
	needQ := gameutil.CfgInt(gs.Config, "qualifyShots", 5)
	if !gs.QualDone {
		if p.QualifyCount < needQ {
			p.QualifySum += gameutil.Effective(raw, p.Handicap)
			p.QualifyCount++
			p.Hint = fmt.Sprintf("Quali %d/%d · %.1f", p.QualifyCount, needQ, p.QualifySum)
			p.LastNote = note
			gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: "qualify", Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
			gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " Quali " + note
			events = append(events, gs.maybeBuildBracket()...)
		}
		return marshalWithEvents(gs, events)
	}
	m := gs.liveMatch()
	if m == nil || (ctx.RangeNum != m.Left && ctx.RangeNum != m.RightNum) || p.Eliminated {
		return marshalWithEvents(gs, events)
	}
	if p.MatchShots > 0 {
		return marshalWithEvents(gs, events)
	}
	p.MatchShots = 1
	if ctx.RangeNum == m.Left {
		m.LeftHas, m.LeftRaw, m.LeftHiH = true, raw, hih
	} else {
		m.RightHas, m.RightRaw, m.RightHiH = true, raw, hih
	}
	p.Hint = fmt.Sprintf("%d:%d", m.LeftPoints, m.RightPoints)
	p.LastNote = note
	res := "match"
	if hih {
		res = "hole_in_hole"
	}
	gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: res, Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	if m.LeftHas && m.RightHas {
		events = append(events, gs.resolvePoint(m)...)
	}
	return marshalWithEvents(gs, events)
}
func (l *Logic) Tick(sess logicapi.SessionState, now time.Time) (logicapi.SessionState, []logicapi.PluginEvent, bool, error) {
	return sess, nil, false, nil
}
func (l *Logic) Control(sess logicapi.SessionState, action string, params map[string]any) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	gs.ensure()
	var events []logicapi.PluginEvent
	switch action {
	case "sync_live":
		gs.applyLive(params)
		if gs.Phase == gameutil.PhaseWarmup && gs.anyReady() {
			gs.Phase = gameutil.PhaseArming
		}
		if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
			events = append(events, gs.beginPlay()...)
		}
	case "start", "skipCalibration":
		events = append(events, gs.beginPlay()...)
	case "reset":
		fresh, _ := l.Init(gs.Config)
		gs2, _ := unmarshalState(fresh)
		gs2.NumRanges = gs.NumRanges
		gs2.ensure()
		return marshalWithEvents(gs2, []logicapi.PluginEvent{{Type: "reset"}})
	}
	return marshalWithEvents(gs, events)
}
func (l *Logic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return nil, err
	}
	gs.ensure()
	players := []map[string]any{}
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p == nil {
			continue
		}
		players = append(players, map[string]any{
			"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "color": p.Color,
			"label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum), "hint": p.Hint, "lastRaw": p.LastRaw,
			"qualifySum": p.QualifySum,
		})
	}
	recent := []map[string]any{}
	for _, s := range gs.recentShots(8) {
		recent = append(recent, map[string]any{"rangeNum": s.RangeNum, "raw": s.Raw, "note": s.Note, "x": s.X, "y": s.Y, "color": s.Color})
	}
	bracket := []map[string]any{}
	for i := range gs.Bracket {
		m := &gs.Bracket[i]
		entry := map[string]any{"roundLabel": m.RoundLabel, "leftLabel": gs.labelOf(m.Left), "rightLabel": gs.rightLabel(m), "winnerLabel": ""}
		if m.WinnerNum > 0 {
			entry["winnerLabel"] = gs.labelOf(m.WinnerNum)
		}
		bracket = append(bracket, entry)
	}
	cur := map[string]any{"leftLabel": "", "rightLabel": "", "leftPoints": 0, "rightPoints": 0}
	if m := gs.liveMatch(); m != nil {
		cur = map[string]any{"leftLabel": gs.labelOf(m.Left), "rightLabel": gs.rightLabel(m), "leftPoints": m.LeftPoints, "rightPoints": m.RightPoints}
	}
	var me map[string]any
	if p := gs.Players[gameutil.Itoa(rangeNum)]; p != nil {
		me = map[string]any{"rangeNum": p.RangeNum, "label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum), "hint": p.Hint, "lastRaw": p.LastRaw, "qualifySum": p.QualifySum}
	}
	return map[string]any{"pluginId": l.ID(), "kind": "game", "mode": "shared", "label": l.Label(), "rangeNum": rangeNum,
		"game": map[string]any{"phase": gs.Phase, "statusLine": gs.StatusLine, "players": players, "recentShots": recent,
			"winnerRange": gs.WinnerRange, "bracket": bracket, "currentMatch": cur, "defaultTargetProfile": "air_rifle_10m"},
		"me": me}, nil
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.Phase, gs.FieldOpen, gs.WinnerRange, gs.Shots = gameutil.PhasePlaying, true, 0, nil
	gs.QualDone, gs.CurrentIdx, gs.Bracket = false, 0, nil
	gs.StatusLine = "Qualifikation — Setzliste"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated, p.QualifySum, p.QualifyCount, p.MatchShots, p.HasLast, p.ShotsFired, p.Eliminated = false, 0, 0, 0, false, 0, false
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Hint = "Qualifikation"
	}
	for _, p := range gs.seatNow() {
		p.Seated, p.Ready = true, true
	}
	if gameutil.CfgInt(gs.Config, "qualifyShots", 5) <= 0 {
		return append([]logicapi.PluginEvent{{Type: "match_start"}}, gs.maybeBuildBracket()...)
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}

func (gs *GameState) maybeBuildBracket() []logicapi.PluginEvent {
	if gs.QualDone {
		return nil
	}
	need := gameutil.CfgInt(gs.Config, "qualifyShots", 5)
	seated := gs.seatedList()
	if len(seated) == 0 {
		return nil
	}
	for _, p := range seated {
		if p.QualifyCount < need {
			return nil
		}
	}
	gs.QualDone = true
	seeds := gs.aliveSeeds()
	if len(seeds) <= 1 {
		return gs.finishChampion(seeds)
	}
	gs.buildRound(seeds)
	gs.StatusLine = gs.liveStatus()
	return []logicapi.PluginEvent{{Type: "bracket_ready"}}
}

func (gs *GameState) buildRound(seeds []*Player) {
	if len(seeds) <= 1 {
		return
	}
	label := roundLabel(len(seeds))
	players := append([]*Player(nil), seeds...)
	if len(players)%2 == 1 {
		bye := players[0]
		gs.Bracket = append(gs.Bracket, Match{RoundLabel: label, Left: bye.RangeNum, RightNum: 0, WinnerNum: bye.RangeNum})
		players = players[1:]
	}
	n := len(players)
	for i := 0; i < n/2; i++ {
		left, right := players[i], players[n-1-i]
		gs.Bracket = append(gs.Bracket, Match{RoundLabel: label, Left: left.RangeNum, RightNum: right.RangeNum})
	}
}

func roundLabel(n int) string {
	switch {
	case n <= 2:
		return "Finale"
	case n <= 4:
		return "Halbfinale"
	case n <= 8:
		return "Viertelfinale"
	default:
		return "Achtelfinale"
	}
}

func (gs *GameState) liveMatch() *Match {
	for i := range gs.Bracket {
		m := &gs.Bracket[i]
		if m.WinnerNum == 0 && m.RightNum > 0 {
			gs.CurrentIdx = i
			return m
		}
	}
	return nil
}

func (gs *GameState) resolvePoint(m *Match) []logicapi.PluginEvent {
	left, right := gs.Players[gameutil.Itoa(m.Left)], gs.Players[gameutil.Itoa(m.RightNum)]
	if left == nil || right == nil {
		return nil
	}
	win := 0
	switch {
	case m.LeftHiH && !m.RightHiH:
		win = m.Left
	case m.RightHiH && !m.LeftHiH:
		win = m.RightNum
	default:
		le, re := gameutil.Effective(m.LeftRaw, left.Handicap), gameutil.Effective(m.RightRaw, right.Handicap)
		if le > re {
			win = m.Left
		} else if re > le {
			win = m.RightNum
		}
	}
	m.LeftHas, m.RightHas, m.LeftHiH, m.RightHiH = false, false, false, false
	left.MatchShots, right.MatchShots = 0, 0
	if win == 0 {
		gs.StatusLine = "Gleichstand — Punkt neu"
		left.Hint, right.Hint = "Neu", "Neu"
		return nil
	}
	if win == m.Left {
		m.LeftPoints++
	} else {
		m.RightPoints++
	}
	need := gameutil.CfgInt(gs.Config, "pointsToWin", 2)
	left.Hint = fmt.Sprintf("%d:%d", m.LeftPoints, m.RightPoints)
	right.Hint = left.Hint
	gs.StatusLine = fmt.Sprintf("%s %d:%d %s", gs.labelOf(m.Left), m.LeftPoints, m.RightPoints, gs.labelOf(m.RightNum))
	if m.LeftPoints >= need {
		m.WinnerNum = m.Left
		right.Eliminated = true
		return gs.afterMatch(m)
	}
	if m.RightPoints >= need {
		m.WinnerNum = m.RightNum
		left.Eliminated = true
		return gs.afterMatch(m)
	}
	return nil
}

func (gs *GameState) afterMatch(m *Match) []logicapi.PluginEvent {
	events := []logicapi.PluginEvent{{Type: "match_finished", Data: map[string]any{"winnerRange": m.WinnerNum}}}
	if gs.liveMatch() != nil {
		gs.StatusLine = gs.liveStatus()
		return events
	}
	seeds := gs.aliveSeeds()
	if len(seeds) <= 1 {
		return append(events, gs.finishChampion(seeds)...)
	}
	gs.buildRound(seeds)
	gs.StatusLine = gs.liveStatus()
	return events
}

func (gs *GameState) finishChampion(seeds []*Player) []logicapi.PluginEvent {
	best := 0
	if len(seeds) > 0 {
		best = seeds[0].RangeNum
	} else {
		for _, p := range gs.seatedList() {
			if p != nil && !p.Eliminated {
				best = p.RangeNum
				break
			}
		}
	}
	if best == 0 {
		return nil
	}
	gs.WinnerRange, gs.Phase = best, gameutil.PhaseFinished
	gs.StatusLine = gs.labelOf(best) + " gewinnt den Pokal"
	return []logicapi.PluginEvent{{Type: "finished", Data: map[string]any{"winnerRange": best}}, {Type: "match_finished", Data: map[string]any{"winnerRange": best}}}
}

func (gs *GameState) aliveSeeds() []*Player {
	var out []*Player
	for _, p := range gs.seatedList() {
		if p != nil && !p.Eliminated {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].QualifySum != out[j].QualifySum {
			return out[i].QualifySum > out[j].QualifySum
		}
		return out[i].RangeNum < out[j].RangeNum
	})
	return out
}
func (gs *GameState) seatedList() []*Player {
	var out []*Player
	for _, p := range gs.seatNow() {
		if p != nil && p.Seated {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return gs.seatNow()
	}
	return out
}
func (gs *GameState) labelOf(rn int) string {
	if rn < 1 {
		return ""
	}
	p := gs.Players[gameutil.Itoa(rn)]
	if p == nil {
		return "Stand " + gameutil.Itoa(rn)
	}
	return gameutil.PlayerLabel(p.ShooterName, rn)
}
func (gs *GameState) rightLabel(m *Match) string {
	if m.RightNum < 1 {
		return "Freilos"
	}
	return gs.labelOf(m.RightNum)
}
func (gs *GameState) liveStatus() string {
	if m := gs.liveMatch(); m != nil {
		return m.RoundLabel + " · " + gs.labelOf(m.Left) + " vs " + gs.rightLabel(m)
	}
	return "KO-Pokal"
}

func (gs *GameState) seatNow() []*Player {
	var out []*Player
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p != nil && p.Active && (p.ShooterName != "" || p.Ready || !p.WasWarmup) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		for i := 1; i <= gs.NumRanges; i++ {
			if p := gs.Players[gameutil.Itoa(i)]; p != nil && p.Active {
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RangeNum < out[j].RangeNum })
	return out
}
func (gs *GameState) ensure() {
	if gs.Config == nil {
		gs.Config = map[string]any{}
	}
	if gs.Players == nil {
		gs.Players = map[string]*Player{}
	}
	if gs.NumRanges < 1 {
		gs.NumRanges = 6
	}
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{RangeNum: i, Active: true, WasWarmup: true, Color: gameutil.LaneColor(i)}
		}
	}
	gs.applyMembership(gs.Config)
}
func (gs *GameState) allSeatedReady() bool {
	s := gs.seatNow()
	if len(s) == 0 {
		return false
	}
	for _, p := range s {
		if !p.Ready {
			return false
		}
	}
	return true
}
func (gs *GameState) anyReady() bool {
	for _, p := range gs.Players {
		if p != nil && p.Active && p.Ready {
			return true
		}
	}
	return false
}
func (gs *GameState) pushShot(m ShotMark) {
	gs.Shots = append(gs.Shots, m)
	if len(gs.Shots) > 80 {
		gs.Shots = gs.Shots[len(gs.Shots)-80:]
	}
}
func (gs *GameState) recentShots(n int) []ShotMark {
	if len(gs.Shots) <= n {
		return gs.Shots
	}
	return gs.Shots[len(gs.Shots)-n:]
}
func (gs *GameState) applyLive(params map[string]any) {
	if params == nil {
		return
	}
	if n, ok := params["numRanges"]; ok {
		gs.NumRanges = gameutil.CfgInt(map[string]any{"n": n}, "n", gs.NumRanges)
		gs.ensure()
	}
	gs.applyMembership(params)
	live, _ := params["live"].(map[string]any)
	for k, raw := range live {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rn, _ := strconv.Atoi(k)
		if rn < 1 {
			continue
		}
		p := gs.Players[gameutil.Itoa(rn)]
		if p == nil {
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, Color: gameutil.LaneColor(rn)}
			gs.Players[gameutil.Itoa(rn)] = p
		}
		if name, ok := m["shooterName"].(string); ok && name != "" {
			p.ShooterName = name
		}
		p.Active = gameutil.LiveActive(m, p.Active)
	}
}
func (gs *GameState) applyMembership(params map[string]any) {
	skip, ok := gameutil.InactiveSetFromParams(params)
	if !ok {
		return
	}
	gs.applyInactiveListFromSet(skip)
}
func (gs *GameState) applyInactiveList(nums []int) {
	gs.applyInactiveListFromSet(gameutil.IntSet(nums))
}
func (gs *GameState) applyInactiveListFromSet(skip map[int]bool) {
	for i := 1; i <= gs.NumRanges; i++ {
		if p := gs.Players[gameutil.Itoa(i)]; p != nil {
			p.Active = !skip[i]
		}
	}
}
func (gs *GameState) openCompetitionField() {
	gs.FieldOpen = true
	for _, p := range gs.Players {
		if p != nil && p.Active {
			p.WasWarmup, p.Ready = false, true
		}
	}
	if gs.Phase == gameutil.PhaseWarmup {
		gs.Phase, gs.StatusLine = gameutil.PhaseArming, "Bereit"
	}
}
func marshalState(gs *GameState) (logicapi.SessionState, error) {
	b, err := json.Marshal(gs)
	return logicapi.SessionState(b), err
}
func unmarshalState(sess logicapi.SessionState) (*GameState, error) {
	var gs GameState
	if len(sess) == 0 {
		return &GameState{Players: map[string]*Player{}}, nil
	}
	if err := json.Unmarshal(sess, &gs); err != nil {
		return nil, err
	}
	if gs.Players == nil {
		gs.Players = map[string]*Player{}
	}
	if gs.Config == nil {
		gs.Config = map[string]any{}
	}
	return &gs, nil
}
func marshalWithEvents(gs *GameState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := marshalState(gs)
	return st, events, err
}

var _ logicapi.ExtendedLogic = (*Logic)(nil)
