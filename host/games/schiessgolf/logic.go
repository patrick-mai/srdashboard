package schiessgolf

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
	loader.RegisterBuiltin("schiessgolf", func(m *loader.Manifest) logicapi.Logic { return New(m) })
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }
func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "schiessgolf"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Schießgolf"
}
func (l *Logic) Version() string { return "1.0.0" }
func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true, "holes": 9,
		"holeDistances": []any{360, 420, 280, 500, 340, 460, 300, 380, 240},
		"advanceFloor":  6.0, "advanceScale": 60.0, "puttValue": 10.5,
		"holeInHoleMinOverlap": 0.5, "holeInHoleMinValue": 8.5, "shotDiameterMm": 4.5, "dsgPerMm": 100.0,
		"defaultTargetProfile": "air_rifle_10m", "disciplineTargets": gameutil.DefaultDisciplineTargets(),
		"handicaps": map[string]any{},
	}
}
func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"holes":         map[string]any{"type": "integer"},
		"advanceFloor":  map[string]any{"type": "number"},
		"advanceScale":  map[string]any{"type": "number"},
		"puttValue":     map[string]any{"type": "number"},
		"holeDistances": map[string]any{"type": "array"},
	}}
}

type Player struct {
	RangeNum, ShotsFired, Strokes                    int
	Active, Seated, Ready, WasWarmup, HasLast, Holed bool
	ShooterName, Discipline, Color, Hint, LastNote   string
	Handicap, LastRaw, Remaining                     float64
	LastX, LastY                                     int
}

type ShotMark struct {
	RangeNum int     `json:"rangeNum"`
	Raw      float64 `json:"raw"`
	Result   string  `json:"result"`
	Note     string  `json:"note"`
	X, Y     int
	Color    string `json:"color"`
}

type GameState struct {
	Phase, StatusLine, StartBlockedReason string
	NumRanges, WinnerRange, Hole          int
	FieldOpen                             bool
	Config                                map[string]any
	Players                               map[string]*Player
	Shots                                 []ShotMark
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(l.DefaultConfig(), cfg)
	gs := &GameState{Phase: gameutil.PhaseWarmup, NumRanges: gameutil.CfgInt(merged, "numRanges", 6), Config: merged, Players: map[string]*Player{}, StatusLine: "Schießgolf", Hole: 1}
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
	if gs.Phase != gameutil.PhasePlaying || !p.Seated || p.Holed {
		return marshalWithEvents(gs, events)
	}
	hih := false
	if p.HasLast {
		if ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config); ok {
			hih = true
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{"rangeNum": p.RangeNum, "overlap": ov}})
		}
	}
	floor := gameutil.CfgFloat(gs.Config, "advanceFloor", 6)
	scale := gameutil.CfgFloat(gs.Config, "advanceScale", 60)
	putt := gameutil.CfgFloat(gs.Config, "puttValue", 10.5)
	advance := (raw - floor) * scale
	if advance < 0 {
		advance = 0
	}
	p.Strokes++
	p.ShotsFired++
	canHole := raw >= putt || hih
	note := fmt.Sprintf("+%.0f", advance)
	if p.Remaining > 0 {
		p.Remaining -= advance
		if p.Remaining <= 0 {
			p.Remaining = 0
			if canHole {
				p.Holed = true
				note = "eingelocht"
			} else {
				note = "Grün"
			}
		}
	} else {
		if canHole {
			p.Holed = true
			p.Remaining = 0
			note = "eingelocht"
		} else {
			note = "Putt"
		}
	}
	if hih && note != "eingelocht" {
		note = "Hole-in-Hole"
	} else if hih {
		note = "Hole-in-Hole · eingelocht"
	}
	p.LastRaw, p.LastX, p.LastY, p.HasLast = raw, ctx.Shot.X, ctx.Shot.Y, true
	if p.Holed {
		p.Hint = fmt.Sprintf("Bahn %d · %d Schläge", gs.Hole, p.Strokes)
	} else if p.Remaining <= 0 {
		p.Hint = "Auf dem Grün"
	} else {
		p.Hint = fmt.Sprintf("%.0f übrig", p.Remaining)
	}
	p.LastNote = note
	gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: "stroke", Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	events = append(events, gs.maybeAdvanceHole()...)
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
			"remaining": p.Remaining, "strokes": p.Strokes, "holed": p.Holed,
		})
	}
	recent := []map[string]any{}
	for _, s := range gs.recentShots(8) {
		recent = append(recent, map[string]any{"rangeNum": s.RangeNum, "raw": s.Raw, "note": s.Note, "x": s.X, "y": s.Y, "color": s.Color})
	}
	var me map[string]any
	if p := gs.Players[gameutil.Itoa(rangeNum)]; p != nil {
		me = map[string]any{"rangeNum": p.RangeNum, "label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum), "hint": p.Hint,
			"lastRaw": p.LastRaw, "remaining": p.Remaining, "strokes": p.Strokes, "holed": p.Holed}
	}
	return map[string]any{"pluginId": l.ID(), "kind": "game", "mode": "shared", "label": l.Label(), "rangeNum": rangeNum,
		"game": map[string]any{"phase": gs.Phase, "statusLine": gs.StatusLine, "players": players, "recentShots": recent,
			"winnerRange": gs.WinnerRange, "hole": gs.Hole, "holeDistance": gs.holeDistance(), "defaultTargetProfile": "air_rifle_10m"},
		"me": me}, nil
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.Phase, gs.FieldOpen, gs.WinnerRange, gs.Shots, gs.Hole = gameutil.PhasePlaying, true, 0, nil, 1
	dist := gs.holeDistance()
	gs.StatusLine = fmt.Sprintf("Bahn %d · %.0f", gs.Hole, dist)
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated, p.Strokes, p.Holed, p.HasLast, p.ShotsFired, p.Remaining = false, 0, false, false, 0, dist
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Hint = fmt.Sprintf("%.0f übrig", dist)
	}
	for _, p := range gs.seatNow() {
		p.Seated, p.Ready = true, true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}

func (gs *GameState) maybeAdvanceHole() []logicapi.PluginEvent {
	seated := gs.seatedList()
	if len(seated) == 0 {
		return nil
	}
	for _, p := range seated {
		if !p.Holed {
			return nil
		}
	}
	holes := gs.holeCount()
	if gs.Hole >= holes {
		return gs.finish()
	}
	gs.Hole++
	dist := gs.holeDistance()
	for _, p := range seated {
		p.Holed, p.Remaining = false, dist
		p.Hint = fmt.Sprintf("%.0f übrig", dist)
	}
	gs.StatusLine = fmt.Sprintf("Bahn %d · %.0f", gs.Hole, dist)
	return []logicapi.PluginEvent{{Type: "hole_complete", Data: map[string]any{"hole": gs.Hole - 1}}}
}

func (gs *GameState) finish() []logicapi.PluginEvent {
	seated := gs.seatedList()
	if len(seated) == 0 {
		return nil
	}
	best := seated[0]
	for _, p := range seated[1:] {
		if p.Strokes < best.Strokes || (p.Strokes == best.Strokes && p.Remaining < best.Remaining) {
			best = p
		}
	}
	gs.WinnerRange, gs.Phase = best.RangeNum, gameutil.PhaseFinished
	gs.StatusLine = gameutil.PlayerLabel(best.ShooterName, best.RangeNum) + " gewinnt"
	return []logicapi.PluginEvent{{Type: "finished", Data: map[string]any{"winnerRange": best.RangeNum}}, {Type: "match_finished", Data: map[string]any{"winnerRange": best.RangeNum}}}
}

func (gs *GameState) holeCount() int {
	n := gameutil.CfgInt(gs.Config, "holes", 9)
	if n < 1 {
		n = 9
	}
	return n
}
func (gs *GameState) holeDistance() float64 {
	dists := cfgFloatList(gs.Config, "holeDistances", []float64{360, 420, 280, 500, 340, 460, 300, 380, 240})
	idx := gs.Hole - 1
	if idx < 0 {
		idx = 0
	}
	if idx < len(dists) {
		return dists[idx]
	}
	if len(dists) > 0 {
		return dists[len(dists)-1]
	}
	return 360
}
func cfgFloatList(m map[string]any, key string, def []float64) []float64 {
	if m == nil {
		return append([]float64(nil), def...)
	}
	v, ok := m[key]
	if !ok || v == nil {
		return append([]float64(nil), def...)
	}
	var out []float64
	switch t := v.(type) {
	case []float64:
		out = append(out, t...)
	case []any:
		for _, item := range t {
			out = append(out, gameutil.ToFloat(item, 0))
		}
	case []int:
		for _, n := range t {
			out = append(out, float64(n))
		}
	}
	if len(out) == 0 {
		return append([]float64(nil), def...)
	}
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
	if gs.Hole < 1 {
		gs.Hole = 1
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
