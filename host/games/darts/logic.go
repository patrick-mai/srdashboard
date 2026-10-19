package darts

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"srdashboard/host/games/gameutil"
	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func init() {
	loader.RegisterBuiltin("darts", func(m *loader.Manifest) logicapi.Logic {
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
	return "darts"
}

func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Darts"
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
			"startScore":            map[string]any{"type": "integer", "enum": []any{301, 501, 701}},
			"doubleIn":              map[string]any{"type": "boolean"},
			"doubleOut":             map[string]any{"type": "boolean"},
			"boardScale":            map[string]any{"type": "number"},
			"defaultTargetProfile":  map[string]any{"type": "string"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true,
		"startScore":            501,
		"doubleIn":              false,
		"doubleOut":             false,
		"boardScale":            1.0,
		"defaultTargetProfile":  "dartboard",
		"disciplineTargets": map[string]any{
			"Luftgewehr":   "dartboard",
			"LG":           "dartboard",
			"Luftpistole":  "dartboard",
			"LP":           "dartboard",
			"Kleinkaliber": "dartboard",
			"KK-Gewehr":    "dartboard",
			"KK":           "dartboard",
		},
	}
}

type Player struct {
	RangeNum    int      `json:"rangeNum"`
	Active      bool     `json:"active"`
	Seated      bool     `json:"seated"`
	Ready       bool     `json:"ready"`
	WasWarmup   bool     `json:"wasWarmup"`
	ShooterName string   `json:"shooterName"`
	Discipline  string   `json:"discipline"`
	Color       string   `json:"color"`
	Remaining   int      `json:"remaining"`
	Opened      bool     `json:"opened"`
	Finished    bool     `json:"finished"`
	LastThree   []string `json:"lastThree"`
}

type ShotMark struct {
	RangeNum  int     `json:"rangeNum"`
	Label     string  `json:"label"`
	Points    int     `json:"points"`
	Note      string  `json:"note"`
	Result    string  `json:"result"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	DecValue  float64 `json:"decValue"`
	FullValue int     `json:"fullValue"`
	Distance  float64 `json:"distance"`
}

type GameState struct {
	Phase              string             `json:"phase"`
	NumRanges          int                `json:"numRanges"`
	Config             map[string]any     `json:"config"`
	Players            map[string]*Player `json:"players"`
	Shots              []ShotMark         `json:"shots"`
	WinnerRange        int                `json:"winnerRange"`
	StartBlockedReason string             `json:"startBlockedReason"`
	StatusLine         string             `json:"statusLine"`
	RulesLabel         string             `json:"rulesLabel"`
	FieldOpen          bool               `json:"fieldOpen"`
	Calibrating        bool               `json:"calibrating"`
	Calibrated         bool               `json:"calibrated"`
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(defaultConfig(), cfg)
	n := gameutil.CfgInt(merged, "numRanges", 6)
	gs := &GameState{
		Phase:     gameutil.PhaseWarmup,
		NumRanges: n,
		Config:    merged,
		Players:   map[string]*Player{},
	}
	if cfgScale(merged) != 1 {
		gs.Calibrated = true
	}
	gs.ensure()
	gs.refreshLabels()
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

	if gs.Calibrating {
		ev := gs.applyCalibration(p, ctx.Shot)
		gs.refreshLabels()
		if ev != nil {
			events = append(events, *ev)
		}
		return marshalWithEvents(gs, events)
	}

	if gameutil.IsWarmupShot(ctx.Live.IsWarmup, ctx.Shot.IsWarmup) {
		p.WasWarmup = true
		gs.plot(p, ctx.Shot, "0", 0, "warmup", "Einschießen")
		return marshalWithEvents(gs, nil)
	}

	_, openNow := gameutil.WarmupDiscard(gs.FieldOpen, false)
	if openNow {
		gs.openCompetitionField()
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
			events = append(events, gs.beginPlay()...)
		} else if gs.Phase == gameutil.PhaseArming {
			gs.StartBlockedReason = gs.blockReason()
		}
	}
	if gs.Phase != gameutil.PhasePlaying || gs.WinnerRange != 0 || !p.Seated || p.Finished {
		gs.refreshLabels()
		return marshalWithEvents(gs, events)
	}

	hit := Classify(ctx.Shot.X, ctx.Shot.Y, cfgScale(gs.Config))
	note := gs.applyHit(p, hit)
	gs.plot(p, ctx.Shot, hit.Label, hit.Points, resultOf(note), note)
	if p.Finished && gs.WinnerRange == 0 {
		gs.WinnerRange = p.RangeNum
		gs.Phase = gameutil.PhaseFinished
		events = append(events, logicapi.PluginEvent{Type: "win", Data: map[string]any{"rangeNum": p.RangeNum}})
		events = append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerRange": p.RangeNum}})
	}
	gs.refreshLabels()
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
	case "calibrate":
		gs.Calibrating = true
	case "reset":
		cfg := gs.Config
		n := gs.NumRanges
		fresh, _ := l.Init(cfg)
		gs2, _ := unmarshalState(fresh)
		gs2.NumRanges = n
		gs2.ensure()
		gs2.refreshLabels()
		return marshalWithEvents(gs2, []logicapi.PluginEvent{{Type: "reset"}})
	}
	gs.refreshLabels()
	return marshalWithEvents(gs, events)
}

func (l *Logic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return nil, err
	}
	gs.ensure()
	gs.refreshLabels()
	players := make([]map[string]any, 0, gs.NumRanges)
	for i := 1; i <= gs.NumRanges; i++ {
		p := gs.Players[gameutil.Itoa(i)]
		if p == nil {
			continue
		}
		players = append(players, playerVM(p))
	}
	me := gs.Players[gameutil.Itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = playerVM(me)
	}
	recent := gs.recentShots(12)
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
	scale := cfgScale(gs.Config)
	return map[string]any{
		"pluginId": l.ID(),
		"kind":     "game",
		"mode":     "shared",
		"label":    l.Label(),
		"rangeNum": rangeNum,
		"game": map[string]any{
			"phase":                gs.Phase,
			"statusLine":           gs.StatusLine,
			"rulesLabel":           gs.RulesLabel,
			"startBlockedReason":   gs.StartBlockedReason,
			"winnerRange":          gs.WinnerRange,
			"calibrating":          gs.Calibrating,
			"calibrated":           gs.Calibrated,
			"boardScale":           scale,
			"outerDoubleMm":        doubleOutMM * scale,
			"startScore":           startScore(gs.Config),
			"doubleIn":             gs.doubleIn(),
			"doubleOut":            gs.doubleOut(),
			"players":              players,
			"recentShots":          recentVM,
			"defaultTargetProfile": "dartboard",
		},
		"me":          meVM,
		"lastOwn":     lastOwn,
		"lastForeign": lastForeign,
	}, nil
}

func (gs *GameState) applyCalibration(p *Player, shot state.Shot) *logicapi.PluginEvent {
	scale, ok := CalibrationScale(shot.X, shot.Y)
	if !ok {
		gs.plot(p, shot, "Mitte", 0, "calibrate-reject", "Maß unverändert")
		return nil
	}
	gs.Config["boardScale"] = scale
	gs.Calibrating = false
	gs.Calibrated = true
	note := "Außenkante Doppel: " + formatMM(doubleOutMM*scale) + " mm"
	gs.plot(p, shot, "Maß", 0, "calibrate", note)
	return &logicapi.PluginEvent{Type: "persist_config", Data: map[string]any{"boardScale": scale}}
}

func (gs *GameState) applyHit(p *Player, hit Hit) string {
	gs.pushVisit(p, hit.Label)
	if gs.doubleIn() && !p.Opened {
		if !hit.IsDouble || hit.Points <= 0 {
			return "Double-In"
		}
		p.Opened = true
	}
	if hit.Points == 0 {
		return "0"
	}
	next := p.Remaining - hit.Points
	if gs.doubleOut() {
		if next < 0 || next == 1 || (next == 0 && !hit.IsDouble) {
			return "Bust"
		}
	} else if next < 0 {
		return "Bust"
	}
	p.Remaining = next
	if next == 0 {
		p.Finished = true
		return "Checkout"
	}
	return hit.Label
}

func (gs *GameState) pushVisit(p *Player, label string) {
	p.LastThree = append(p.LastThree, label)
	if len(p.LastThree) > 3 {
		p.LastThree = p.LastThree[len(p.LastThree)-3:]
	}
}

func (gs *GameState) plot(p *Player, shot state.Shot, label string, points int, result, note string) {
	gs.Shots = append(gs.Shots, ShotMark{
		RangeNum: p.RangeNum, Label: label, Points: points, Note: note, Result: result,
		X: shot.X, Y: shot.Y, DecValue: shot.DecValue, FullValue: shot.FullValue, Distance: shot.Distance,
	})
	if len(gs.Shots) > 80 {
		gs.Shots = gs.Shots[len(gs.Shots)-80:]
	}
}

func resultOf(note string) string {
	switch note {
	case "Bust":
		return "bust"
	case "Double-In":
		return "double-in"
	case "Checkout":
		return "checkout"
	case "0":
		return "miss"
	default:
		return "score"
	}
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.ensure()
	seated := gs.seatNow()
	start := startScore(gs.Config)
	open := !gs.doubleIn()
	for _, p := range seated {
		p.Seated = true
		p.Ready = true
		p.WasWarmup = false
		p.Remaining = start
		p.Opened = open
		p.Finished = false
		p.LastThree = nil
	}
	gs.Phase = gameutil.PhasePlaying
	gs.WinnerRange = 0
	gs.FieldOpen = true
	gs.StartBlockedReason = ""
	gs.refreshLabels()
	return []logicapi.PluginEvent{{Type: "match_start", Data: map[string]any{"startScore": start}}}
}

func (gs *GameState) refreshLabels() {
	gs.RulesLabel = rulesLabel(gs.Config)
	if gs.Calibrating {
		gs.StatusLine = "Scheibe einmessen — ein Schuss auf den äußeren Doppel-Draht"
		return
	}
	if gs.Phase == gameutil.PhaseFinished && gs.WinnerRange != 0 {
		gs.StatusLine = gs.RulesLabel + " — " + winnerLabel(gs) + " gewinnt"
		return
	}
	if gs.Calibrated {
		gs.StatusLine = "Außenkante Doppel: " + formatMM(doubleOutMM*cfgScale(gs.Config)) + " mm"
		return
	}
	gs.StatusLine = gs.RulesLabel
}

func rulesLabel(cfg map[string]any) string {
	parts := []string{strconv.Itoa(startScore(cfg))}
	if gameutil.CfgBool(cfg, "doubleIn", false) {
		parts = append(parts, "Double-In")
	}
	if gameutil.CfgBool(cfg, "doubleOut", false) {
		parts = append(parts, "Double-Out")
	}
	return strings.Join(parts, ", ")
}

func winnerLabel(gs *GameState) string {
	p := gs.Players[gameutil.Itoa(gs.WinnerRange)]
	if p == nil {
		return "Stand " + strconv.Itoa(gs.WinnerRange)
	}
	return playerLabel(p)
}

func startScore(cfg map[string]any) int {
	n := gameutil.CfgInt(cfg, "startScore", 501)
	switch n {
	case 301, 501, 701:
		return n
	default:
		return 501
	}
}

func cfgScale(cfg map[string]any) float64 {
	if cfg == nil {
		return 1
	}
	v, ok := cfg["boardScale"]
	if !ok || v == nil {
		return 1
	}
	if _, ok := v.(bool); ok {
		return 1
	}
	s := gameutil.ToFloat(v, 1)
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return 1
	}
	return s
}

func (gs *GameState) doubleIn() bool  { return gameutil.CfgBool(gs.Config, "doubleIn", false) }
func (gs *GameState) doubleOut() bool { return gameutil.CfgBool(gs.Config, "doubleOut", false) }

func formatMM(mm float64) string {
	return strings.Replace(strconv.FormatFloat(mm, 'f', 1, 64), ".", ",", 1)
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
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{
				RangeNum: i, Active: true, WasWarmup: true, Color: gameutil.LaneColor(i),
			}
		}
	}
	gs.applyMembership(gs.Config)
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
	return out
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
	var missing []int
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
	}
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
			continue
		}
		if name, ok := m["shooterName"].(string); ok && name != "" {
			p.ShooterName = name
		}
		if disc, ok := m["discipline"].(string); ok && disc != "" {
			p.Discipline = disc
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

func playerVM(p *Player) map[string]any {
	three := append([]string(nil), p.LastThree...)
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "ready": p.Ready,
		"shooterName": p.ShooterName, "color": p.Color, "label": playerLabel(p),
		"remaining": p.Remaining, "opened": p.Opened, "finished": p.Finished,
		"lastThree": three,
	}
}

func shotVM(s ShotMark, gs *GameState) map[string]any {
	color := ""
	if p := gs.Players[gameutil.Itoa(s.RangeNum)]; p != nil {
		color = p.Color
	}
	return map[string]any{
		"rangeNum": s.RangeNum, "label": s.Label, "points": s.Points,
		"note": s.Note, "result": s.Result, "x": s.X, "y": s.Y,
		"decValue": s.DecValue, "fullValue": s.FullValue, "distance": s.Distance,
		"color": color,
	}
}

func playerLabel(p *Player) string {
	if p.ShooterName != "" {
		return p.ShooterName
	}
	return "Stand " + gameutil.Itoa(p.RangeNum)
}

func marshalState(gs *GameState) (logicapi.SessionState, error) {
	b, err := json.Marshal(gs)
	if err != nil {
		return nil, err
	}
	return logicapi.SessionState(b), nil
}

func marshalWithEvents(gs *GameState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := marshalState(gs)
	return st, events, err
}

func unmarshalState(sess logicapi.SessionState) (*GameState, error) {
	var gs GameState
	if len(sess) == 0 {
		return &gs, nil
	}
	if err := json.Unmarshal(sess, &gs); err != nil {
		return nil, err
	}
	return &gs, nil
}
