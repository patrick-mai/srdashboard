package schrumpfenderkreis

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
	loader.RegisterBuiltin("schrumpfender-kreis", func(m *loader.Manifest) logicapi.Logic { return New(m) })
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }
func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "schrumpfender-kreis"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Schrumpfender Kreis"
}
func (l *Logic) Version() string { return "1.0.0" }
func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true, "calibrateShots": 5, "startOffset": 1.5, "stepPerRound": 0.15,
		"lives": 1, "maxLives": 2, "holeInHoleGrantsLife": true, "maxRounds": 20,
		"holeInHoleMinOverlap": 0.5, "holeInHoleMinValue": 8.5, "shotDiameterMm": 4.5, "dsgPerMm": 100.0,
		"defaultTargetProfile": "air_rifle_10m", "disciplineTargets": gameutil.DefaultDisciplineTargets(),
		"handicaps": map[string]any{},
	}
}
func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"calibrateShots": map[string]any{"type": "integer"}, "maxRounds": map[string]any{"type": "integer"},
		"lives": map[string]any{"type": "integer"}, "stepPerRound": map[string]any{"type": "number"},
	}}
}

type Player struct {
	RangeNum, ShotsFired, RoundShot, Lives int
	Active, Seated, Ready, WasWarmup       bool
	HasLast, Out, Calibrated               bool
	ShooterName, Discipline, Color, Hint   string
	LastNote                               string
	Handicap, LastRaw, Par, Need, TotalRaw float64
	LastX, LastY                           int
	CalValues                              []float64
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
	NumRanges, WinnerRange, Round         int
	FieldOpen                             bool
	Config                                map[string]any
	Players                               map[string]*Player
	Shots                                 []ShotMark
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(l.DefaultConfig(), cfg)
	gs := &GameState{Phase: gameutil.PhaseWarmup, NumRanges: gameutil.CfgInt(merged, "numRanges", 6), Config: merged, Players: map[string]*Player{}, StatusLine: "Schrumpfender Kreis"}
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
		gs.collectCal(p, raw)
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
	if p.Out || p.RoundShot >= gs.Round {
		return marshalWithEvents(gs, events)
	}
	hih := false
	if p.HasLast {
		if ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config); ok {
			hih = true
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{"rangeNum": p.RangeNum, "overlap": ov}})
		}
	}
	p.Need = gs.needFor(p)
	note, result := "daneben", "miss"
	if raw >= p.Need {
		note, result = "sicher", "hit"
	} else {
		p.Lives--
		if p.Lives < 0 {
			p.Lives = 0
		}
	}
	if hih && gameutil.CfgBool(gs.Config, "holeInHoleGrantsLife", true) {
		maxL := gameutil.CfgInt(gs.Config, "maxLives", 2)
		if p.Lives < maxL {
			p.Lives++
		}
		note = "Hole-in-Hole +Leben"
		result = "hole_in_hole"
	}
	if p.Lives <= 0 {
		p.Out, p.Lives = true, 0
		note = "aus"
		events = append(events, logicapi.PluginEvent{Type: "player_out", Data: map[string]any{"rangeNum": p.RangeNum}})
	}
	p.LastRaw, p.LastX, p.LastY, p.HasLast = raw, ctx.Shot.X, ctx.Shot.Y, true
	p.ShotsFired++
	p.RoundShot = gs.Round
	p.TotalRaw += raw
	p.LastNote = note
	p.Hint = gs.playerHint(p)
	gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: result, Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	events = append(events, gs.maybeResolveRound()...)
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
			"need": p.Need, "round": gs.Round, "out": p.Out, "lives": p.Lives,
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
		"game": map[string]any{"phase": gs.Phase, "statusLine": gs.StatusLine, "players": players, "recentShots": recent, "winnerRange": gs.WinnerRange, "defaultTargetProfile": "air_rifle_10m"},
		"me":   me}, nil
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.Phase, gs.FieldOpen, gs.WinnerRange, gs.Shots, gs.Round = gameutil.PhasePlaying, true, 0, nil, 1
	gs.StatusLine = "Kreis schrumpft"
	lives := gameutil.CfgInt(gs.Config, "lives", 1)
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated, p.Out, p.ShotsFired, p.RoundShot, p.HasLast, p.TotalRaw = false, false, 0, 0, false, 0
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Par = gs.parFor(p)
		p.Lives = lives
		p.Need = gs.needFor(p)
		p.Hint = gs.playerHint(p)
	}
	for _, p := range gs.seatNow() {
		p.Seated, p.Ready = true, true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}
func (gs *GameState) maybeResolveRound() []logicapi.PluginEvent {
	if gs.Phase != gameutil.PhasePlaying {
		return nil
	}
	survivors := gs.survivors()
	needRound := gs.Round
	if needRound < 1 {
		needRound = 1
	}
	for _, p := range survivors {
		if p.RoundShot < needRound {
			return nil
		}
	}
	events := []logicapi.PluginEvent{{Type: "round_resolved", Data: map[string]any{"round": gs.Round}}}
	survivors = gs.survivors()
	if len(survivors) <= 1 {
		if len(survivors) == 1 {
			gs.finish(survivors[0].RangeNum, events)
		} else {
			gs.finishBySum(gs.seatNow(), events)
		}
		return events
	}
	maxR := gameutil.CfgInt(gs.Config, "maxRounds", 20)
	if gs.Round >= maxR {
		gs.finishBySum(survivors, events)
		return events
	}
	gs.Round++
	for _, p := range survivors {
		p.Need = gs.needFor(p)
		p.Hint = gs.playerHint(p)
	}
	gs.StatusLine = fmt.Sprintf("Runde %d · Schwelle steigt", gs.Round)
	return events
}
func (gs *GameState) finish(rangeNum int, events []logicapi.PluginEvent) []logicapi.PluginEvent {
	gs.WinnerRange, gs.Phase = rangeNum, gameutil.PhaseFinished
	label := gameutil.PlayerLabel("", rangeNum)
	if p := gs.Players[gameutil.Itoa(rangeNum)]; p != nil {
		label = gameutil.PlayerLabel(p.ShooterName, rangeNum)
	}
	gs.StatusLine = label + " gewinnt"
	return append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerRange": rangeNum}})
}
func (gs *GameState) finishBySum(cands []*Player, events []logicapi.PluginEvent) []logicapi.PluginEvent {
	if len(cands) == 0 {
		gs.Phase = gameutil.PhaseFinished
		gs.StatusLine = "Beendet"
		return events
	}
	best, bestSum := cands[0].RangeNum, cands[0].TotalRaw
	for _, p := range cands[1:] {
		if p.TotalRaw > bestSum {
			best, bestSum = p.RangeNum, p.TotalRaw
		}
	}
	return gs.finish(best, events)
}
func (gs *GameState) survivors() []*Player {
	var out []*Player
	for _, p := range gs.seatNow() {
		if !p.Out {
			out = append(out, p)
		}
	}
	return out
}
func (gs *GameState) needFor(p *Player) float64 {
	r := gs.Round
	if r < 1 {
		r = 1
	}
	start := gameutil.CfgFloat(gs.Config, "startOffset", 1.5)
	step := gameutil.CfgFloat(gs.Config, "stepPerRound", 0.15)
	return p.Par - start + float64(r-1)*step
}
func (gs *GameState) playerHint(p *Player) string {
	if p.Out {
		return "aus"
	}
	return fmt.Sprintf("≥ %.1f · %d Leben", p.Need, p.Lives)
}
func (gs *GameState) parFor(p *Player) float64 {
	if len(p.CalValues) > 0 {
		return gameutil.MeanFloats(p.CalValues)
	}
	return 8
}
func (gs *GameState) collectCal(p *Player, raw float64) {
	need := gameutil.CfgInt(gs.Config, "calibrateShots", 5)
	if p.Calibrated || need <= 0 {
		return
	}
	p.CalValues = append(p.CalValues, raw)
	if len(p.CalValues) >= need {
		p.Calibrated, p.Ready, p.Par = true, true, gs.parFor(p)
	}
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
