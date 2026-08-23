package kronenduell

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
	loader.RegisterBuiltin("kronen-duell", func(m *loader.Manifest) logicapi.Logic { return New(m) })
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }
func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "kronen-duell"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Kronen-Duell"
}
func (l *Logic) Version() string { return "1.0.0" }
func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true, "matchSeconds": 900, "decayPerShot": 0.1, "decayFloor": 8.0,
		"shotsPerPlayer": 20, "handicaps": map[string]any{},
		"holeInHoleMinOverlap": 0.5, "holeInHoleMinValue": 8.5, "shotDiameterMm": 4.5, "dsgPerMm": 100.0,
		"defaultTargetProfile": "air_rifle_10m", "disciplineTargets": gameutil.DefaultDisciplineTargets(),
	}
}
func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"matchSeconds": map[string]any{"type": "integer"}, "shotsPerPlayer": map[string]any{"type": "integer"},
		"decayPerShot": map[string]any{"type": "number"}, "decayFloor": map[string]any{"type": "number"},
	}}
}

type Player struct {
	RangeNum, ShotsFired                 int
	Active, Seated, Ready, WasWarmup     bool
	HasLast                              bool
	ShooterName, Discipline, Color, Hint string
	LastNote                             string
	Handicap, LastRaw, HoldSeconds       float64
	LastX, LastY                         int
}

type ShotMark struct {
	RangeNum int     `json:"rangeNum"`
	Raw      float64 `json:"raw"`
	Result   string  `json:"result"`
	Note     string  `json:"note"`
	X, Y     int     `json:"x"`
	Color    string  `json:"color"`
}

type GameState struct {
	Phase, StatusLine, StartBlockedReason string
	NumRanges, WinnerRange, HolderRange   int
	FieldOpen                             bool
	Bar                                   float64
	LastTick, MatchStart                  time.Time
	Config                                map[string]any
	Players                               map[string]*Player
	Shots                                 []ShotMark
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(l.DefaultConfig(), cfg)
	gs := &GameState{Phase: gameutil.PhaseWarmup, NumRanges: gameutil.CfgInt(merged, "numRanges", 6), Config: merged, Players: map[string]*Player{}, StatusLine: "Kronen-Duell"}
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
	need := gameutil.CfgInt(gs.Config, "shotsPerPlayer", 20)
	if p.ShotsFired >= need {
		return marshalWithEvents(gs, events)
	}
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}
	gs.accrueHold(now)
	hih := false
	if p.HasLast {
		if ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config); ok {
			hih = true
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{"rangeNum": p.RangeNum, "overlap": ov}})
		}
	}
	if gs.HolderRange > 0 {
		gs.Bar -= gameutil.CfgFloat(gs.Config, "decayPerShot", 0.1)
		floor := gameutil.CfgFloat(gs.Config, "decayFloor", 8)
		if gs.Bar < floor {
			gs.Bar = floor
		}
	}
	eff := gameutil.Effective(raw, p.Handicap)
	took := false
	if gs.HolderRange == 0 {
		took = true
	} else if p.RangeNum != gs.HolderRange && (hih || eff > gs.Bar) {
		took = true
	}
	note, result := fmt.Sprintf("%.1f", raw), "score"
	if took {
		gs.HolderRange, gs.Bar = p.RangeNum, eff
		note, result = "Krone", "crown"
		if hih {
			note, result = "Hole-in-Hole · Krone", "hole_in_hole"
		}
		events = append(events, logicapi.PluginEvent{Type: "crown_taken", Data: map[string]any{"rangeNum": p.RangeNum, "bar": gs.Bar}})
	} else if p.RangeNum == gs.HolderRange {
		gs.Bar = eff
		note = "hält"
	} else {
		note = fmt.Sprintf("Latte %.1f", gs.Bar)
	}
	p.LastRaw, p.LastX, p.LastY, p.HasLast = raw, ctx.Shot.X, ctx.Shot.Y, true
	p.ShotsFired++
	p.LastNote = note
	p.Hint = gs.playerHint(p)
	for _, q := range gs.Players {
		if q != nil && q.Seated {
			q.Hint = gs.playerHint(q)
		}
	}
	gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: result, Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	gs.maybeFinish(now)
	return marshalWithEvents(gs, events)
}
func (l *Logic) Tick(sess logicapi.SessionState, now time.Time) (logicapi.SessionState, []logicapi.PluginEvent, bool, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, false, err
	}
	gs.ensure()
	if gs.Phase != gameutil.PhasePlaying {
		return sess, nil, false, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	gs.accrueHold(now)
	gs.maybeFinish(now)
	st, err := marshalState(gs)
	return st, nil, true, err
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
			"holdSeconds": p.HoldSeconds,
		})
	}
	recent := []map[string]any{}
	for _, s := range gs.recentShots(8) {
		recent = append(recent, map[string]any{"rangeNum": s.RangeNum, "raw": s.Raw, "note": s.Note, "x": s.X, "y": s.Y, "color": s.Color})
	}
	var me map[string]any
	if p := gs.Players[gameutil.Itoa(rangeNum)]; p != nil {
		me = map[string]any{"rangeNum": p.RangeNum, "label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum), "hint": p.Hint}
	}
	return map[string]any{"pluginId": l.ID(), "kind": "game", "mode": "shared", "label": l.Label(), "rangeNum": rangeNum,
		"game": map[string]any{"phase": gs.Phase, "statusLine": gs.StatusLine, "players": players, "recentShots": recent, "winnerRange": gs.WinnerRange, "holderRange": gs.HolderRange, "bar": gs.Bar, "defaultTargetProfile": "air_rifle_10m"},
		"me":   me}, nil
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	now := time.Now()
	gs.Phase, gs.FieldOpen, gs.WinnerRange, gs.Shots = gameutil.PhasePlaying, true, 0, nil
	gs.HolderRange, gs.Bar, gs.LastTick, gs.MatchStart = 0, 0, now, now
	gs.StatusLine = "Kronen-Duell — nimm die Krone"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated, p.ShotsFired, p.HasLast, p.HoldSeconds = false, 0, false, 0
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Hint = "erste Krone"
	}
	for _, p := range gs.seatNow() {
		p.Seated, p.Ready = true, true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}
func (gs *GameState) accrueHold(now time.Time) {
	if gs.LastTick.IsZero() {
		gs.LastTick = now
		return
	}
	if now.Before(gs.LastTick) {
		gs.LastTick = now
		return
	}
	if gs.HolderRange > 0 {
		if p := gs.Players[gameutil.Itoa(gs.HolderRange)]; p != nil {
			p.HoldSeconds += now.Sub(gs.LastTick).Seconds()
		}
	}
	gs.LastTick = now
}
func (gs *GameState) maybeFinish(now time.Time) {
	need := gameutil.CfgInt(gs.Config, "shotsPerPlayer", 20)
	seated := gs.seatNow()
	if len(seated) == 0 {
		return
	}
	allDone := true
	for _, p := range seated {
		if p.ShotsFired < need {
			allDone = false
			break
		}
	}
	timedOut := false
	if sec := gameutil.CfgInt(gs.Config, "matchSeconds", 900); sec > 0 && !gs.MatchStart.IsZero() {
		if now.Sub(gs.MatchStart).Seconds() >= float64(sec) {
			timedOut = true
		}
	}
	if !allDone && !timedOut {
		return
	}
	gs.accrueHold(now)
	best, bestHold := 0, -1.0
	for _, p := range seated {
		if p.HoldSeconds > bestHold {
			best, bestHold = p.RangeNum, p.HoldSeconds
		}
	}
	gs.WinnerRange, gs.Phase = best, gameutil.PhaseFinished
	gs.StatusLine = gameutil.PlayerLabel(gs.Players[gameutil.Itoa(best)].ShooterName, best) + " gewinnt"
}
func (gs *GameState) playerHint(p *Player) string {
	if gs.HolderRange == p.RangeNum {
		return fmt.Sprintf("Krone · %.0fs", p.HoldSeconds)
	}
	return fmt.Sprintf("Latte %.1f", gs.Bar)
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
