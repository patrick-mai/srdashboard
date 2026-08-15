package malefiz

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
	loader.RegisterBuiltin("malefiz", func(m *loader.Manifest) logicapi.Logic {
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
	return "malefiz"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Malefiz"
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
			"pathLength":            map[string]any{"type": "integer"},
			"citadel":               map[string]any{"type": "integer"},
			"barricadeSlots":        map[string]any{"type": "string"},
			"initialBarricades":     map[string]any{"type": "string"},
			"stepMin":               map[string]any{"type": "number"},
			"liftMin":               map[string]any{"type": "number"},
			"surgeMin":              map[string]any{"type": "number"},
			"defaultTargetProfile":  map[string]any{"type": "string"},
			"handicaps":             map[string]any{"type": "object"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true,
		"pathLength":            24,
		"citadel":               25,
		"barricadeSlots":        "4,8,14,19,22",
		"initialBarricades":     "8,14,22",
		"stepMin":               9.0,
		"liftMin":               10.0,
		"surgeMin":              10.5,
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
	Cell        int     `json:"cell"`
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
	Citadel            int                `json:"citadel"`
	Slots              []int              `json:"slots"`
	Barricades         []int              `json:"barricades"`
	WinnerRange        int                `json:"winnerRange"`
	StartBlockedReason string             `json:"startBlockedReason"`
	StatusLine         string             `json:"statusLine"`
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(defaultConfig(), cfg)
	n := gameutil.CfgInt(merged, "numRanges", 6)
	gs := &GameState{
		Phase:      gameutil.PhaseWarmup,
		NumRanges:  n,
		Config:     merged,
		Players:    map[string]*Player{},
		Citadel:    gameutil.CfgInt(merged, "citadel", 25),
		Slots:      gameutil.CfgIntList(merged, "barricadeSlots", []int{4, 8, 14, 19, 22}),
		Barricades: gameutil.CfgIntList(merged, "initialBarricades", []int{8, 14, 22}),
		StatusLine: "Einschießen — danach Malefiz",
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
	if ctx.Live.IsWarmup || ctx.Shot.IsWarmup {
		p.WasWarmup = true
		return marshalWithEvents(gs, nil)
	}
	if p.WasWarmup && !ctx.Live.IsWarmup {
		p.WasWarmup = false
		p.Ready = true
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		if gs.Phase == gameutil.PhaseWarmup {
			gs.Phase = gameutil.PhaseArming
			gs.StatusLine = "Bereit — Malefiz starten"
		}
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
	stepMin := gameutil.CfgFloat(gs.Config, "stepMin", 9)
	liftMin := gameutil.CfgFloat(gs.Config, "liftMin", 10)
	surgeMin := gameutil.CfgFloat(gs.Config, "surgeMin", 10.5)
	mark := ShotMark{
		RangeNum: ctx.RangeNum, Raw: raw, Effective: eff, Result: "miss",
		X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
	}

	if eff+1e-9 < stepMin {
		mark.Note = "kein Zug"
		gs.pushShot(mark)
		gs.StatusLine = fmt.Sprintf("Stand %d: Fehl (%.1f)", ctx.RangeNum, raw)
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		return marshalWithEvents(gs, events)
	}

	steps := 1
	if eff+1e-9 >= surgeMin {
		steps = 2
	}

	_, blocked, barCell := gs.nextLanding(p.Cell, p.RangeNum)
	lifted := false
	if blocked && eff+1e-9 >= liftMin {
		gs.relocate(barCell, p.Cell)
		lifted = true
		events = append(events, logicapi.PluginEvent{Type: "lift", Data: map[string]any{
			"rangeNum": p.RangeNum, "from": barCell, "barricades": append([]int(nil), gs.Barricades...),
		}})
	} else if blocked {
		mark.Note = "Mauer — 10.0 hebt sie"
		gs.pushShot(mark)
		gs.StatusLine = fmt.Sprintf("%s steht vor der Mauer", playerLabel(p))
		events = append(events, logicapi.PluginEvent{Type: "blocked", Data: map[string]any{"rangeNum": p.RangeNum, "cell": p.Cell}})
		return marshalWithEvents(gs, events)
	}

	gs.walk(p, steps)
	mark.Result = "walk"
	if lifted {
		mark.Result = "lift"
		mark.Note = fmt.Sprintf("Mauer weg, Feld %d", p.Cell)
	} else {
		mark.Note = fmt.Sprintf("Feld %d", p.Cell)
	}
	gs.pushShot(mark)
	events = append(events, logicapi.PluginEvent{Type: "walk", Data: map[string]any{
		"rangeNum": p.RangeNum, "cell": p.Cell, "lifted": lifted,
	}})
	gs.StatusLine = fmt.Sprintf("%s auf Feld %d", playerLabel(p), p.Cell)

	if p.Cell >= gs.Citadel {
		p.Cell = gs.Citadel
		p.Finished = true
		if gs.WinnerRange == 0 {
			gs.WinnerRange = p.RangeNum
			gs.Phase = gameutil.PhaseFinished
			gs.StatusLine = fmt.Sprintf("%s erreicht die Burg!", playerLabel(p))
			events = append(events, logicapi.PluginEvent{Type: "win", Data: map[string]any{"rangeNum": p.RangeNum}})
			events = append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerRange": p.RangeNum}})
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
			"citadel":              gs.Citadel,
			"slots":                gs.Slots,
			"barricades":           gs.Barricades,
			"stepMin":              gameutil.CfgFloat(gs.Config, "stepMin", 9),
			"liftMin":              gameutil.CfgFloat(gs.Config, "liftMin", 10),
			"surgeMin":             gameutil.CfgFloat(gs.Config, "surgeMin", 10.5),
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
	hint := "Frei — 9+ geht weiter"
	_, blocked, _ := gs.nextLanding(p.Cell, p.RangeNum)
	if blocked {
		hint = "Mauer — 10.0 hebt sie"
	}
	left := gs.Citadel - p.Cell
	if left < 0 {
		left = 0
	}
	if p.Finished {
		hint = "In der Burg"
	} else if !blocked {
		hint = fmt.Sprintf("Frei — %d Felder zur Burg", left)
	}
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "ready": p.Ready,
		"shooterName": p.ShooterName, "color": p.Color, "label": playerLabel(p),
		"cell": p.Cell, "finished": p.Finished, "hint": hint, "blocked": blocked,
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

func (gs *GameState) isBarricade(cell int) bool {
	for _, b := range gs.Barricades {
		if b == cell {
			return true
		}
	}
	return false
}

func (gs *GameState) pawnAt(cell, except int) bool {
	if cell <= 0 {
		return false
	}
	for _, p := range gs.Players {
		if p == nil || !p.Seated || p.RangeNum == except || p.Finished {
			continue
		}
		if p.Cell == cell {
			return true
		}
	}
	return false
}

func (gs *GameState) nextLanding(from, except int) (cell int, blocked bool, barCell int) {
	n := from + 1
	for n < gs.Citadel && gs.pawnAt(n, except) {
		n++
	}
	if n > gs.Citadel {
		n = gs.Citadel
	}
	if gs.isBarricade(n) {
		return n, true, n
	}
	return n, false, 0
}

func (gs *GameState) walk(p *Player, steps int) {
	for i := 0; i < steps; i++ {
		land, blocked, _ := gs.nextLanding(p.Cell, p.RangeNum)
		if blocked {
			return
		}
		p.Cell = land
		if p.Cell >= gs.Citadel {
			p.Cell = gs.Citadel
			return
		}
	}
}

func (gs *GameState) relocate(from, moverCell int) {
	next := make([]int, 0, len(gs.Barricades))
	for _, b := range gs.Barricades {
		if b != from {
			next = append(next, b)
		}
	}
	gs.Barricades = next
	best := -1
	for _, slot := range gs.Slots {
		if slot >= moverCell {
			continue
		}
		if gs.isBarricade(slot) || gs.pawnAt(slot, 0) {
			continue
		}
		if slot > best {
			best = slot
		}
	}
	if best < 0 {
		for _, slot := range gs.Slots {
			if slot == from {
				continue
			}
			if gs.isBarricade(slot) || gs.pawnAt(slot, 0) {
				continue
			}
			best = slot
			break
		}
	}
	if best < 0 {
		best = from
	}
	gs.Barricades = append(gs.Barricades, best)
	sort.Ints(gs.Barricades)
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.ensure()
	seated := gs.seatNow()
	gs.Citadel = gameutil.CfgInt(gs.Config, "citadel", 25)
	if gs.Citadel < 8 {
		gs.Citadel = 25
	}
	gs.Slots = gameutil.CfgIntList(gs.Config, "barricadeSlots", []int{4, 8, 14, 19, 22})
	gs.Barricades = gameutil.CfgIntList(gs.Config, "initialBarricades", []int{8, 14, 22})
	gs.Phase = gameutil.PhasePlaying
	gs.WinnerRange = 0
	gs.Shots = nil
	gs.StartBlockedReason = ""
	gs.StatusLine = "Malefiz — 9+ geht, 10.0 hebt die Mauer"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated = false
		p.Cell = 0
		p.Finished = false
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
	}
	for _, p := range seated {
		p.Seated = true
		p.Cell = 0
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
	if gs.Citadel < 8 {
		gs.Citadel = 25
	}
	if len(gs.Slots) == 0 {
		gs.Slots = []int{4, 8, 14, 19, 22}
	}
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{
				RangeNum: i, Active: true, WasWarmup: true, Color: gameutil.LaneColor(i),
			}
		}
	}
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
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, Color: gameutil.LaneColor(rn)}
			gs.Players[gameutil.Itoa(rn)] = p
		}
		if name, ok := m["shooterName"].(string); ok && name != "" {
			p.ShooterName = name
		}
		if disc, ok := m["discipline"].(string); ok && disc != "" {
			p.Discipline = disc
		}
		if warmup, ok := m["isWarmup"].(bool); ok && warmup {
			p.WasWarmup = true
		}
		if active, ok := m["active"].(bool); ok {
			p.Active = active
		}
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
