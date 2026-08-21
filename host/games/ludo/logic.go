package ludo

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
	loader.RegisterBuiltin("ludo", func(m *loader.Manifest) logicapi.Logic {
		return New(m)
	})
}

type Logic struct {
	manifest *loader.Manifest
}

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }

func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "ludo"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Ludo"
}
func (l *Logic) Version() string {
	if l.manifest != nil && l.manifest.Version != "" {
		return l.manifest.Version
	}
	return "1.0.0"
}

func (l *Logic) DefaultConfig() map[string]any { return defaultConfig() }

func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"autoStartWhenAllReady": map[string]any{"type": "boolean"},
			"cellsPerPlayer":        map[string]any{"type": "integer"},
			"homeLength":            map[string]any{"type": "integer"},
			"enterMin":              map[string]any{"type": "number"},
			"stepMin":               map[string]any{"type": "number"},
			"doubleMin":             map[string]any{"type": "number"},
			"defaultTargetProfile":  map[string]any{"type": "string"},
			"handicaps":             map[string]any{"type": "object"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true,
		"cellsPerPlayer":        10,
		"homeLength":            4,
		"enterMin":              10.0,
		"stepMin":               9.0,
		"doubleMin":             10.5,
		"defaultTargetProfile":  "air_rifle_10m",
		"disciplineTargets":     gameutil.DefaultDisciplineTargets(),
		"handicaps":             map[string]any{},
	}
}

type Player struct {
	RangeNum    int     `json:"rangeNum"`
	Active      bool    `json:"active"`
	Seated      bool    `json:"seated"`
	Ready       bool    `json:"ready"`
	WasWarmup   bool    `json:"wasWarmup"`
	ShooterName string  `json:"shooterName"`
	Discipline  string  `json:"discipline"`
	Color       string  `json:"color"`
	Handicap    float64 `json:"handicap"`
	InYard      bool    `json:"inYard"`
	Entry       int     `json:"entry"`
	LapProgress int     `json:"lapProgress"`
	RingCell    int     `json:"ringCell"`
	Home        int     `json:"home"`
	Finished    bool    `json:"finished"`
}

type ShotMark struct {
	RangeNum  int     `json:"rangeNum"`
	Raw       float64 `json:"raw"`
	Effective float64 `json:"effective"`
	Result    string  `json:"result"`
	Note      string  `json:"note"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Distance  float64 `json:"distance"`
	FullValue int     `json:"fullValue"`
}

type GameState struct {
	Phase              string             `json:"phase"`
	NumRanges          int                `json:"numRanges"`
	Config             map[string]any     `json:"config"`
	Players            map[string]*Player `json:"players"`
	Shots              []ShotMark         `json:"shots"`
	RingSize           int                `json:"ringSize"`
	BoardArms          int                `json:"boardArms"`
	HomeLength         int                `json:"homeLength"`
	WinnerRange        int                `json:"winnerRange"`
	StartBlockedReason string             `json:"startBlockedReason"`
	StatusLine         string             `json:"statusLine"`
	FieldOpen          bool               `json:"fieldOpen"`
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(defaultConfig(), cfg)
	n := gameutil.CfgInt(merged, "numRanges", 6)
	gs := &GameState{
		Phase:      gameutil.PhaseWarmup,
		NumRanges:  n,
		Config:     merged,
		Players:    map[string]*Player{},
		HomeLength: gameutil.CfgInt(merged, "homeLength", 4),
		StatusLine: "Einschießen — danach Ludo",
	}
	gs.ensure()
	return marshalState(gs)
}

func (l *Logic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	return l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum: rangeNum, Shot: shot, ShotIndex: shotIndex,
		Live: logicapi.LiveRangeInfo{IsWarmup: shot.IsWarmup}, Now: time.Now(),
	})
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
	if ctx.Live.Discipline != "" {
		p.Discipline = ctx.Live.Discipline
	}
	discard, openNow := gameutil.WarmupDiscard(gs.FieldOpen, gameutil.IsWarmupShot(ctx.Live.IsWarmup, ctx.Shot.IsWarmup))
	if discard {
		p.WasWarmup = true
		return marshalWithEvents(gs, nil)
	}
	if openNow {
		gs.openCompetitionField()
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
			events = append(events, gs.beginPlay()...)
		} else if gs.Phase == gameutil.PhaseArming {
			gs.StartBlockedReason = gs.blockReason()
		}
	}

	raw := gameutil.RawShotValue(ctx.Shot.DecValue, ctx.Shot.FullValue)
	if gs.Phase != gameutil.PhasePlaying {
		return marshalWithEvents(gs, events)
	}
	if !p.Seated || p.Finished {
		return marshalWithEvents(gs, events)
	}

	eff := gameutil.Effective(raw, p.Handicap)
	enterMin := gameutil.CfgFloat(gs.Config, "enterMin", 10)
	stepMin := gameutil.CfgFloat(gs.Config, "stepMin", 9)
	doubleMin := gameutil.CfgFloat(gs.Config, "doubleMin", 10.5)
	mark := ShotMark{
		RangeNum: ctx.RangeNum, Raw: raw, Effective: eff, Result: "miss",
		X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
	}

	if p.InYard {
		if eff+1e-9 < enterMin {
			mark.Note = "Hof — 10.0 setzt ein"
			gs.pushShot(mark)
			gs.StatusLine = fmt.Sprintf("Stand %d: im Hof (%.1f)", ctx.RangeNum, raw)
			events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{"rangeNum": ctx.RangeNum, "reason": "yard"}})
			return marshalWithEvents(gs, events)
		}
		captured := gs.enter(p)
		mark.Result = "enter"
		mark.Note = "eingesetzt"
		gs.pushShot(mark)
		gs.StatusLine = fmt.Sprintf("%s setzt ein", playerLabel(p))
		events = append(events, logicapi.PluginEvent{Type: "enter", Data: map[string]any{"rangeNum": p.RangeNum, "cell": p.RingCell}})
		if captured != 0 {
			events = append(events, logicapi.PluginEvent{Type: "capture", Data: map[string]any{"from": p.RangeNum, "victim": captured}})
			gs.StatusLine = fmt.Sprintf("%s setzt ein und ärgert Stand %d", playerLabel(p), captured)
		}
		return marshalWithEvents(gs, events)
	}

	steps := 0
	if eff+1e-9 >= doubleMin {
		steps = 2
	} else if eff+1e-9 >= stepMin {
		steps = 1
	}
	if steps == 0 {
		mark.Note = "kein Zug"
		gs.pushShot(mark)
		gs.StatusLine = fmt.Sprintf("Stand %d: Fehl (%.1f)", ctx.RangeNum, raw)
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		return marshalWithEvents(gs, events)
	}

	victim, won := gs.walk(p, steps)
	mark.Result = "walk"
	mark.Note = fmt.Sprintf("+%d", steps)
	gs.pushShot(mark)
	events = append(events, logicapi.PluginEvent{Type: "walk", Data: map[string]any{
		"rangeNum": p.RangeNum, "steps": steps, "home": p.Home, "ring": p.RingCell, "progress": p.LapProgress,
	}})
	if victim != 0 {
		events = append(events, logicapi.PluginEvent{Type: "capture", Data: map[string]any{"from": p.RangeNum, "victim": victim}})
		gs.StatusLine = fmt.Sprintf("%s ärgert Stand %d", playerLabel(p), victim)
	} else {
		gs.StatusLine = fmt.Sprintf("%s geht %d", playerLabel(p), steps)
	}
	if won && gs.WinnerRange == 0 {
		gs.WinnerRange = p.RangeNum
		gs.Phase = gameutil.PhaseFinished
		gs.StatusLine = fmt.Sprintf("%s ist im Ziel!", playerLabel(p))
		events = append(events, logicapi.PluginEvent{Type: "win", Data: map[string]any{"rangeNum": p.RangeNum}})
		events = append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerRange": p.RangeNum}})
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
		if gs.Phase == gameutil.PhaseWarmup || gs.Phase == gameutil.PhaseArming {
			if gs.anyReady() && gs.Phase == gameutil.PhaseWarmup {
				gs.Phase = gameutil.PhaseArming
			}
			if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
				events = append(events, gs.beginPlay()...)
			} else {
				gs.StartBlockedReason = gs.blockReason()
			}
		}
	case "start":
		events = append(events, gs.beginPlay()...)
	case "reset":
		cfg := gs.Config
		n := gs.NumRanges
		fresh, _ := l.Init(cfg)
		gs2, _ := unmarshalState(fresh)
		gs2.NumRanges = n
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
	players := make([]map[string]any, 0, gs.NumRanges)
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p == nil {
			continue
		}
		players = append(players, playerVM(p, gs))
	}
	me := gs.Players[gameutil.Itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = playerVM(me, gs)
	}
	recent := gs.recentShots(6)
	recentVM := make([]map[string]any, 0, len(recent))
	for _, s := range recent {
		recentVM = append(recentVM, shotVM(s, gs))
	}
	var lastOwn, lastForeign map[string]any
	for i := len(gs.Shots) - 1; i >= 0; i-- {
		s := gs.Shots[i]
		if lastOwn == nil && s.RangeNum == rangeNum {
			lastOwn = shotVM(s, gs)
		}
		if lastForeign == nil && s.RangeNum != rangeNum {
			lastForeign = shotVM(s, gs)
		}
		if lastOwn != nil && lastForeign != nil {
			break
		}
	}
	return map[string]any{
		"pluginId": l.ID(),
		"kind":     "game",
		"mode":     "shared",
		"label":    l.Label(),
		"rangeNum": rangeNum,
		"game": map[string]any{
			"phase":                gs.Phase,
			"statusLine":           gs.StatusLine,
			"startBlockedReason":   gs.StartBlockedReason,
			"winnerRange":          gs.WinnerRange,
			"ringSize":             gs.RingSize,
			"boardArms":            gs.BoardArms,
			"homeLength":           gs.HomeLength,
			"cellsPerPlayer":       cellsPerArm(gs),
			"enterMin":             gameutil.CfgFloat(gs.Config, "enterMin", 10),
			"stepMin":              gameutil.CfgFloat(gs.Config, "stepMin", 9),
			"doubleMin":            gameutil.CfgFloat(gs.Config, "doubleMin", 10.5),
			"players":              players,
			"recentShots":          recentVM,
			"defaultTargetProfile": gameutil.CfgString(gs.Config, "defaultTargetProfile", "air_rifle_10m"),
		},
		"me":          meVM,
		"lastOwn":     lastOwn,
		"lastForeign": lastForeign,
	}, nil
}

func playerVM(p *Player, gs *GameState) map[string]any {
	hint := "Hof — 10.0 setzt ein"
	if !p.InYard {
		if p.Home > 0 {
			left := gs.HomeLength - p.Home
			if left < 0 {
				left = 0
			}
			hint = fmt.Sprintf("%d Felder ins Ziel", left)
		} else {
			hint = "9+ geht, 10.5 geht zwei"
		}
	}
	if p.Finished {
		hint = "Im Ziel"
	}
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "ready": p.Ready,
		"shooterName": p.ShooterName, "color": p.Color, "label": playerLabel(p),
		"inYard": p.InYard, "entry": p.Entry, "lapProgress": p.LapProgress,
		"ringCell": p.RingCell, "home": p.Home, "finished": p.Finished, "hint": hint,
	}
}

func shotVM(s ShotMark, gs *GameState) map[string]any {
	color := ""
	if p := gs.Players[gameutil.Itoa(s.RangeNum)]; p != nil {
		color = p.Color
	}
	return map[string]any{
		"rangeNum": s.RangeNum, "raw": s.Raw, "effective": s.Effective,
		"result": s.Result, "note": s.Note, "x": s.X, "y": s.Y,
		"distance": s.Distance, "fullValue": s.FullValue, "color": color,
	}
}

func playerLabel(p *Player) string {
	if p.ShooterName != "" {
		return p.ShooterName
	}
	return "Stand " + gameutil.Itoa(p.RangeNum)
}

func (gs *GameState) enter(p *Player) int {
	p.InYard = false
	p.LapProgress = 0
	p.Home = 0
	p.RingCell = p.Entry
	return gs.captureAt(p.RingCell, p.RangeNum, false)
}

func (gs *GameState) walk(p *Player, steps int) (victim int, won bool) {
	p.LapProgress += steps
	if p.LapProgress < gs.RingSize {
		p.Home = 0
		p.RingCell = (p.Entry + p.LapProgress) % gs.RingSize
		victim = gs.captureAt(p.RingCell, p.RangeNum, false)
		return victim, false
	}
	h := p.LapProgress - gs.RingSize + 1
	if h > gs.HomeLength {
		h = gs.HomeLength
	}
	p.Home = h
	p.RingCell = -1
	if p.Home >= gs.HomeLength {
		p.Finished = true
		p.Home = gs.HomeLength
		return 0, true
	}
	return 0, false
}

func (gs *GameState) captureAt(cell, mover int, home bool) int {
	if home || cell < 0 {
		return 0
	}
	for _, o := range gs.Players {
		if o == nil || !o.Seated || o.RangeNum == mover || o.InYard || o.Finished || o.Home > 0 {
			continue
		}
		if o.RingCell == cell {
			o.InYard = true
			o.LapProgress = 0
			o.Home = 0
			o.RingCell = o.Entry
			o.Finished = false
			return o.RangeNum
		}
	}
	return 0
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.ensure()
	seated := gs.seatNow()
	per := gameutil.CfgInt(gs.Config, "cellsPerPlayer", 10)
	if per < 4 {
		per = 10
	}
	gs.HomeLength = gameutil.CfgInt(gs.Config, "homeLength", 4)
	if gs.HomeLength < 1 {
		gs.HomeLength = 4
	}
	n := len(seated)
	if n < 1 {
		n = 1
	}
	// 2–3 players still use the classic 4-arm board (40+ fields). A 2-point
	// star only has a handful of cells and looks broken.
	arms := n
	if n < 4 {
		arms = 4
		if per < 10 {
			per = 10
		}
	}
	gs.BoardArms = arms
	gs.RingSize = per * arms
	gs.Phase = gameutil.PhasePlaying
	gs.WinnerRange = 0
	gs.Shots = nil
	gs.StartBlockedReason = ""
	gs.StatusLine = "Ludo — 10.0 setzt ein"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated = false
		p.InYard = true
		p.LapProgress = 0
		p.Home = 0
		p.Finished = false
		p.Entry = 0
		p.RingCell = 0
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
	}
	for i, p := range seated {
		p.Seated = true
		p.InYard = true
		arm := i
		if n == 2 && arms == 4 {
			arm = i * 2
		}
		p.Entry = arm * per
		p.RingCell = p.Entry
	}
	return []logicapi.PluginEvent{{Type: "match_start", Data: map[string]any{"ringSize": gs.RingSize, "boardArms": gs.BoardArms}}}
}

func cellsPerArm(gs *GameState) int {
	if gs == nil {
		return 10
	}
	if gs.BoardArms > 0 && gs.RingSize >= gs.BoardArms {
		return gs.RingSize / gs.BoardArms
	}
	per := gameutil.CfgInt(gs.Config, "cellsPerPlayer", 10)
	if per < 4 {
		return 10
	}
	return per
}

func (gs *GameState) seatNow() []*Player {
	var out []*Player
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p == nil || !p.Active {
			continue
		}
		if p.ShooterName != "" || p.Ready || !p.WasWarmup {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		for i := 1; i <= gs.NumRanges; i++ {
			p := gs.Players[gameutil.Itoa(i)]
			if p != nil && p.Active {
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RangeNum < out[j].RangeNum })
	return out
}

func (gs *GameState) ensure() {
	if gs.Config == nil {
		gs.Config = defaultConfig()
	}
	if gs.Players == nil {
		gs.Players = map[string]*Player{}
	}
	if gs.NumRanges < 1 {
		gs.NumRanges = 6
	}
	if gs.HomeLength < 1 {
		gs.HomeLength = 4
	}
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{
				RangeNum: i, Active: true, WasWarmup: true, InYard: true,
				Color: gameutil.LaneColor(i),
			}
		}
	}
	gs.applyMembership(gs.Config)
}

func (gs *GameState) allSeatedReady() bool {
	seated := gs.seatNow()
	if len(seated) == 0 {
		return false
	}
	for _, p := range seated {
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

func (gs *GameState) blockReason() string {
	missing := []int{}
	for _, p := range gs.seatNow() {
		if !p.Ready {
			missing = append(missing, p.RangeNum)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("Warte auf Bereitschaft: Stände %v", missing)
}

func (gs *GameState) pushShot(m ShotMark) {
	gs.Shots = append(gs.Shots, m)
	if len(gs.Shots) > 80 {
		gs.Shots = gs.Shots[len(gs.Shots)-80:]
	}
}

func (gs *GameState) recentShots(n int) []ShotMark {
	if n <= 0 || len(gs.Shots) == 0 {
		return nil
	}
	if len(gs.Shots) <= n {
		out := make([]ShotMark, len(gs.Shots))
		copy(out, gs.Shots)
		return out
	}
	out := make([]ShotMark, n)
	copy(out, gs.Shots[len(gs.Shots)-n:])
	return out
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
	if live == nil {
		return
	}
	for k, raw := range live {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rn, _ := strconv.Atoi(k)
		if rn < 1 {
			rn = gameutil.CfgInt(m, "rangeNum", 0)
		}
		if rn < 1 {
			continue
		}
		p := gs.Players[gameutil.Itoa(rn)]
		if p == nil {
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, InYard: true, Color: gameutil.LaneColor(rn)}
			gs.Players[gameutil.Itoa(rn)] = p
		}
		if name, ok := m["shooterName"].(string); ok && name != "" {
			p.ShooterName = name
		}
		if disc, ok := m["discipline"].(string); ok && disc != "" {
			p.Discipline = disc
		}
		if warmup, ok := m["isWarmup"].(bool); ok && warmup && !gs.FieldOpen {
			p.WasWarmup = true
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
	if gs.Config != nil {
		list := make([]int, 0, len(skip))
		for n := range skip {
			list = append(list, n)
		}
		gs.Config["inactiveRanges"] = list
	}
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p != nil {
			p.Active = !skip[i]
		}
	}
}

func (gs *GameState) openCompetitionField() {
	gs.FieldOpen = true
	for _, p := range gs.Players {
		if p == nil || !p.Active {
			continue
		}
		p.WasWarmup = false
		p.Ready = true
	}
	if gs.Phase == gameutil.PhaseWarmup {
		gs.Phase = gameutil.PhaseArming
		gs.StatusLine = "Bereit — Ludo starten"
	}
}

func marshalState(gs *GameState) (logicapi.SessionState, error) {
	b, err := json.Marshal(gs)
	if err != nil {
		return nil, err
	}
	return logicapi.SessionState(b), nil
}

func unmarshalState(sess logicapi.SessionState) (*GameState, error) {
	var gs GameState
	if len(sess) == 0 {
		return &GameState{Players: map[string]*Player{}, Config: defaultConfig()}, nil
	}
	if err := json.Unmarshal(sess, &gs); err != nil {
		return nil, err
	}
	if gs.Config == nil {
		gs.Config = defaultConfig()
	}
	if gs.Players == nil {
		gs.Players = map[string]*Player{}
	}
	return &gs, nil
}

func marshalWithEvents(gs *GameState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := marshalState(gs)
	return st, events, err
}

var _ logicapi.ExtendedLogic = (*Logic)(nil)
