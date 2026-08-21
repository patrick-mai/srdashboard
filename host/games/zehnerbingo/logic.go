package zehnerbingo

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
	loader.RegisterBuiltin("zehner-bingo", func(m *loader.Manifest) logicapi.Logic {
		return New(m)
	})
}

const (
	WinLine     = "line"
	WinBlackout = "blackout"
	ResultMark  = "mark"
	ResultMiss  = "miss"
)

// CardValues is row-major 3×3. Every line contains at least one 10.x.
var CardValues = []float64{
	10.9, 9.5, 10.5,
	10.0, 10.7, 9.0,
	10.3, 10.8, 10.6,
}

var bingoLines = [][]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

type Logic struct {
	manifest *loader.Manifest
}

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }

func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "zehner-bingo"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Zehner-Bingo"
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
			"winMode":               map[string]any{"type": "string", "enum": []string{WinLine, WinBlackout}},
			"defaultTargetProfile":  map[string]any{"type": "string"},
			"handicaps":             map[string]any{"type": "object"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true,
		"winMode":               WinLine,
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
	Marked      []bool  `json:"marked"`
	BingoLine   []int   `json:"bingoLine,omitempty"`
	Finished    bool    `json:"finished"`
}

type ShotMark struct {
	RangeNum  int     `json:"rangeNum"`
	Raw       float64 `json:"raw"`
	Effective float64 `json:"effective"`
	Result    string  `json:"result"`
	Cell      int     `json:"cell"`
	Value     float64 `json:"value"`
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
		StatusLine: "Einschießen — danach Zehner-Bingo",
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
	idx, val, ok := markHighest(p.Marked, CardValues, eff)
	mark := ShotMark{
		RangeNum: ctx.RangeNum, Raw: raw, Effective: eff,
		Result: ResultMiss, Cell: -1,
		X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
	}
	if !ok {
		gs.pushShot(mark)
		gs.StatusLine = fmt.Sprintf("Stand %d: Fehl — keine offene Zelle ≤ %.1f", ctx.RangeNum, eff)
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "raw": raw, "effective": eff,
		}})
		return marshalWithEvents(gs, events)
	}
	p.Marked[idx] = true
	mark.Result = ResultMark
	mark.Cell = idx
	mark.Value = val
	gs.pushShot(mark)
	events = append(events, logicapi.PluginEvent{Type: "mark", Data: map[string]any{
		"rangeNum": ctx.RangeNum, "raw": raw, "cell": idx, "value": val,
	}})
	gs.StatusLine = fmt.Sprintf("Stand %d: %.1f → %.1f", ctx.RangeNum, raw, val)

	if line, win := gs.playerWins(p); win {
		p.Finished = true
		p.BingoLine = line
		if gs.WinnerRange == 0 {
			gs.WinnerRange = p.RangeNum
			gs.Phase = gameutil.PhaseFinished
			gs.StatusLine = fmt.Sprintf("%s hat Bingo!", playerLabel(p))
			events = append(events, logicapi.PluginEvent{Type: "bingo", Data: map[string]any{
				"rangeNum": p.RangeNum, "line": line,
			}})
			events = append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{
				"winnerRange": p.RangeNum,
			}})
		}
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
		players = append(players, playerVM(p))
	}
	me := gs.Players[gameutil.Itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = playerVM(me)
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
			"winMode":              gameutil.CfgString(gs.Config, "winMode", WinLine),
			"cardValues":           CardValues,
			"players":              players,
			"recentShots":          recentVM,
			"defaultTargetProfile": gameutil.CfgString(gs.Config, "defaultTargetProfile", "air_rifle_10m"),
		},
		"me":          meVM,
		"lastOwn":     lastOwn,
		"lastForeign": lastForeign,
	}, nil
}

func playerVM(p *Player) map[string]any {
	markedN := 0
	for _, m := range p.Marked {
		if m {
			markedN++
		}
	}
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "ready": p.Ready,
		"shooterName": p.ShooterName, "discipline": p.Discipline, "color": p.Color,
		"handicap": p.Handicap, "marked": p.Marked, "markedCount": markedN,
		"bingoLine": p.BingoLine, "finished": p.Finished, "label": playerLabel(p),
	}
}

func shotVM(s ShotMark, gs *GameState) map[string]any {
	color := ""
	if p := gs.Players[gameutil.Itoa(s.RangeNum)]; p != nil {
		color = p.Color
	}
	return map[string]any{
		"rangeNum": s.RangeNum, "raw": s.Raw, "effective": s.Effective,
		"result": s.Result, "cell": s.Cell, "value": s.Value,
		"x": s.X, "y": s.Y, "distance": s.Distance, "fullValue": s.FullValue, "color": color,
	}
}

func playerLabel(p *Player) string {
	if p.ShooterName != "" {
		return p.ShooterName
	}
	return "Stand " + gameutil.Itoa(p.RangeNum)
}

func markHighest(marked []bool, values []float64, shot float64) (int, float64, bool) {
	bestI := -1
	bestV := -1.0
	for i, v := range values {
		if i < len(marked) && marked[i] {
			continue
		}
		if v <= shot+1e-9 && v > bestV {
			bestV = v
			bestI = i
		}
	}
	if bestI < 0 {
		return -1, 0, false
	}
	return bestI, bestV, true
}

func (gs *GameState) playerWins(p *Player) ([]int, bool) {
	mode := gameutil.CfgString(gs.Config, "winMode", WinLine)
	if mode == WinBlackout {
		for _, m := range p.Marked {
			if !m {
				return nil, false
			}
		}
		all := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
		return all, true
	}
	for _, line := range bingoLines {
		ok := true
		for _, i := range line {
			if i >= len(p.Marked) || !p.Marked[i] {
				ok = false
				break
			}
		}
		if ok {
			return append([]int(nil), line...), true
		}
	}
	return nil, false
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.ensure()
	seated := gs.seatNow()
	gs.Phase = gameutil.PhasePlaying
	gs.WinnerRange = 0
	gs.Shots = nil
	gs.StartBlockedReason = ""
	gs.StatusLine = "Zehner-Bingo — immer die höchste offene Zelle"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Finished = false
		p.BingoLine = nil
		p.Marked = make([]bool, len(CardValues))
		p.Seated = false
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
	}
	for _, p := range seated {
		p.Seated = true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
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
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{
				RangeNum: i, Active: true, WasWarmup: true,
				Color: gameutil.LaneColor(i), Marked: make([]bool, len(CardValues)),
			}
		}
		if len(gs.Players[k].Marked) != len(CardValues) {
			gs.Players[k].Marked = make([]bool, len(CardValues))
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
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, Color: gameutil.LaneColor(rn), Marked: make([]bool, len(CardValues))}
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
		gs.StatusLine = "Bereit — Bingo starten"
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
