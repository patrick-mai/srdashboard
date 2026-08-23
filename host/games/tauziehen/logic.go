package tauziehen

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
	loader.RegisterBuiltin("tauziehen", func(m *loader.Manifest) logicapi.Logic {
		return New(m)
	})
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }

func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "tauziehen"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Tauziehen"
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
			"calibrateShots":        map[string]any{"type": "integer"},
			"parMin":                map[string]any{"type": "number"},
			"parMax":                map[string]any{"type": "number"},
			"pullMax":               map[string]any{"type": "number"},
			"ropeTarget":            map[string]any{"type": "number"},
			"ropeScale":             map[string]any{"type": "number"},
			"holeInHoleBonusPull":   map[string]any{"type": "number"},
			"maxRounds":             map[string]any{"type": "integer"},
			"teamAssignments":       map[string]any{"type": "object"},
			"parOverrides":          map[string]any{"type": "object"},
			"handicaps":             map[string]any{"type": "object"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true,
		"calibrateShots":        5,
		"parMin":                5.0,
		"parMax":                10.5,
		"pullMax":               2.0,
		"ropeTarget":            20.0,
		"ropeScale":             1.0,
		"holeInHoleBonusPull":   1.0,
		"holeInHoleMinOverlap":  0.5,
		"holeInHoleMinValue":    8.5,
		"shotDiameterMm":        4.5,
		"dsgPerMm":              100.0,
		"maxRounds":             30,
		"defaultTargetProfile":  "air_rifle_10m",
		"disciplineTargets":     gameutil.DefaultDisciplineTargets(),
		"teamAssignments":       map[string]any{},
		"parOverrides":          map[string]any{},
		"handicaps":             map[string]any{},
	}
}

type Player struct {
	RangeNum    int       `json:"rangeNum"`
	Active      bool      `json:"active"`
	Seated      bool      `json:"seated"`
	Ready       bool      `json:"ready"`
	WasWarmup   bool      `json:"wasWarmup"`
	ShooterName string    `json:"shooterName"`
	Discipline  string    `json:"discipline"`
	Color       string    `json:"color"`
	Handicap    float64   `json:"handicap"`
	Team        string    `json:"team"`
	Par         float64   `json:"par"`
	CalValues   []float64 `json:"calValues"`
	Calibrated  bool      `json:"calibrated"`
	ShotsFired  int       `json:"shotsFired"`
	LastRaw     float64   `json:"lastRaw"`
	LastPull    float64   `json:"lastPull"`
	RoundPull   float64   `json:"roundPull"`
	RoundShot   int       `json:"roundShot"`
	LastX       int       `json:"lastX"`
	LastY       int       `json:"lastY"`
	HasLast     bool      `json:"hasLast"`
	LastNote    string    `json:"lastNote"`
	Hint        string    `json:"hint"`
}

type ShotMark struct {
	RangeNum int     `json:"rangeNum"`
	Raw      float64 `json:"raw"`
	Result   string  `json:"result"`
	Note     string  `json:"note"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Color    string  `json:"color"`
}

type GameState struct {
	Phase              string             `json:"phase"`
	NumRanges          int                `json:"numRanges"`
	Config             map[string]any     `json:"config"`
	Players            map[string]*Player `json:"players"`
	Shots              []ShotMark         `json:"shots"`
	Rope               float64            `json:"rope"`
	Round              int                `json:"round"`
	WinnerTeam         string             `json:"winnerTeam"`
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
		StatusLine: "Einschießen — Tauziehen",
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
	raw := gameutil.RawShotValue(ctx.Shot.DecValue, ctx.Shot.FullValue)

	if gs.Phase == gameutil.PhaseWarmup || gs.Phase == gameutil.PhaseArming {
		need := gameutil.CfgInt(gs.Config, "calibrateShots", 5)
		if !p.Calibrated && need > 0 {
			p.CalValues = append(p.CalValues, raw)
			if len(p.CalValues) >= need {
				p.Calibrated = true
				p.Par = gs.parFor(p)
				p.Ready = true
			}
			gs.StatusLine = fmt.Sprintf("%s kalibriert %d/%d", gameutil.PlayerLabel(p.ShooterName, p.RangeNum), len(p.CalValues), need)
		}
		discard, openNow := gameutil.WarmupDiscard(gs.FieldOpen, gameutil.IsWarmupShot(ctx.Live.IsWarmup, ctx.Shot.IsWarmup))
		if discard && !p.Calibrated {
			p.WasWarmup = true
			return marshalWithEvents(gs, events)
		}
		if openNow || gs.allCalibrated() {
			if !gs.FieldOpen {
				gs.openCompetitionField()
				events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
			}
			if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
				events = append(events, gs.beginPlay()...)
			}
		}
		if gs.Phase != gameutil.PhasePlaying {
			return marshalWithEvents(gs, events)
		}
	}

	if gs.Phase != gameutil.PhasePlaying || !p.Seated {
		return marshalWithEvents(gs, events)
	}

	pullMax := gameutil.CfgFloat(gs.Config, "pullMax", 2)
	pull := gameutil.ClampFloat(raw-p.Par, -pullMax, pullMax)
	note := fmt.Sprintf("%+.1f", pull)
	hih := false
	if p.HasLast {
		ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config)
		if ok {
			hih = true
			bonus := gameutil.CfgFloat(gs.Config, "holeInHoleBonusPull", 1)
			if pull < 0 {
				pull = 0
			}
			pull += bonus
			note = fmt.Sprintf("Hole-in-Hole +%.1f", bonus)
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{
				"rangeNum": p.RangeNum, "overlap": ov,
			}})
		}
	}
	p.LastX, p.LastY, p.HasLast = ctx.Shot.X, ctx.Shot.Y, true
	p.LastRaw = raw
	p.LastPull = pull
	p.LastNote = note
	p.Hint = fmt.Sprintf("Schnitt %.1f", p.Par)
	p.ShotsFired++
	p.RoundShot = p.ShotsFired
	p.RoundPull = pull
	mark := ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: "pull", Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color}
	if hih {
		mark.Result = "hole_in_hole"
	}
	gs.pushShot(mark)
	gs.StatusLine = fmt.Sprintf("%s %s", gameutil.PlayerLabel(p.ShooterName, p.RangeNum), note)
	events = append(events, gs.maybeResolveRound()...)
	return marshalWithEvents(gs, events)
}

func (gs *GameState) maybeResolveRound() []logicapi.PluginEvent {
	seated := gs.seatNow()
	if len(seated) == 0 {
		return nil
	}
	need := gs.Round
	if need < 1 {
		need = 1
	}
	for _, p := range seated {
		if p.ShotsFired < need {
			return nil
		}
	}
	var sumA, sumB float64
	var nA, nB int
	for _, p := range seated {
		if p.Team == "B" {
			sumB += p.RoundPull
			nB++
		} else {
			sumA += p.RoundPull
			nA++
		}
	}
	teamA, teamB := 0.0, 0.0
	if nA > 0 {
		teamA = sumA / float64(nA)
	}
	if nB > 0 {
		teamB = sumB / float64(nB)
	}
	scale := gameutil.CfgFloat(gs.Config, "ropeScale", 1)
	delta := (teamA - teamB) * scale
	gs.Rope += delta
	gs.StatusLine = fmt.Sprintf("Runde %d · Seil %+.1f", gs.Round, gs.Rope)
	events := []logicapi.PluginEvent{{
		Type: "round_resolved",
		Data: map[string]any{"round": gs.Round, "pullA": teamA, "pullB": teamB, "rope": gs.Rope},
	}, {
		Type: "rope_move",
		Data: map[string]any{"rope": gs.Rope, "delta": delta},
	}}
	target := gameutil.CfgFloat(gs.Config, "ropeTarget", 20)
	maxR := gameutil.CfgInt(gs.Config, "maxRounds", 30)
	if gs.Rope >= target {
		gs.finish("A", events)
		return events
	}
	if gs.Rope <= -target {
		gs.finish("B", events)
		return events
	}
	if gs.Round >= maxR {
		if gs.Rope > 0 {
			gs.finish("A", events)
		} else if gs.Rope < 0 {
			gs.finish("B", events)
		} else {
			gs.Phase = gameutil.PhaseFinished
			gs.StatusLine = "Unentschieden"
			events = append(events, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerTeam": ""}})
		}
		return events
	}
	gs.Round++
	return events
}

func (gs *GameState) finish(team string, events []logicapi.PluginEvent) []logicapi.PluginEvent {
	gs.WinnerTeam = team
	gs.Phase = gameutil.PhaseFinished
	gs.StatusLine = "Mannschaft " + team + " gewinnt"
	return append(events,
		logicapi.PluginEvent{Type: "finished", Data: map[string]any{"winnerTeam": team}},
		logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{"winnerTeam": team}},
	)
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
		} else {
			gs.StartBlockedReason = gs.blockReason()
		}
	case "start", "skipCalibration":
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
	recent := gs.recentShots(8)
	recentVM := make([]map[string]any, 0, len(recent))
	for _, s := range recent {
		recentVM = append(recentVM, map[string]any{
			"rangeNum": s.RangeNum, "raw": s.Raw, "result": s.Result, "note": s.Note,
			"x": s.X, "y": s.Y, "color": s.Color,
		})
	}
	var lastOwn, lastForeign map[string]any
	for i := len(gs.Shots) - 1; i >= 0; i-- {
		s := gs.Shots[i]
		m := map[string]any{"rangeNum": s.RangeNum, "raw": s.Raw, "note": s.Note, "x": s.X, "y": s.Y, "color": s.Color}
		if lastOwn == nil && s.RangeNum == rangeNum {
			lastOwn = m
		}
		if lastForeign == nil && s.RangeNum != rangeNum {
			lastForeign = m
		}
		if lastOwn != nil && lastForeign != nil {
			break
		}
	}
	return map[string]any{
		"pluginId": l.ID(), "kind": "game", "mode": "shared", "label": l.Label(), "rangeNum": rangeNum,
		"game": map[string]any{
			"phase": gs.Phase, "statusLine": gs.StatusLine, "startBlockedReason": gs.StartBlockedReason,
			"rope": gs.Rope, "ropeTarget": gameutil.CfgFloat(gs.Config, "ropeTarget", 20),
			"round": gs.Round, "winnerTeam": gs.WinnerTeam, "players": players, "recentShots": recentVM,
			"teamA": map[string]any{"label": "Mannschaft A"},
			"teamB": map[string]any{"label": "Mannschaft B"},
			"defaultTargetProfile": gameutil.CfgString(gs.Config, "defaultTargetProfile", "air_rifle_10m"),
		},
		"me": meVM, "lastOwn": lastOwn, "lastForeign": lastForeign,
	}, nil
}

func playerVM(p *Player) map[string]any {
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "seated": p.Seated, "ready": p.Ready,
		"shooterName": p.ShooterName, "color": p.Color, "label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum),
		"team": p.Team, "par": p.Par, "lastRaw": p.LastRaw, "lastPull": p.LastPull,
		"hint": p.Hint, "lastNote": p.LastNote,
	}
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.ensure()
	seated := gs.seatNow()
	gs.Phase = gameutil.PhasePlaying
	gs.Rope = 0
	gs.Round = 1
	gs.WinnerTeam = ""
	gs.Shots = nil
	gs.StartBlockedReason = ""
	gs.StatusLine = "Tauziehen — ziehen über den Schnitt"
	gs.FieldOpen = true
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated = false
		p.ShotsFired = 0
		p.HasLast = false
		p.RoundPull = 0
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Team = gs.teamOf(p.RangeNum)
		p.Par = gs.parFor(p)
		p.Hint = fmt.Sprintf("Schnitt %.1f", p.Par)
	}
	for _, p := range seated {
		p.Seated = true
		p.Ready = true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}

func (gs *GameState) teamOf(rangeNum int) string {
	m := gs.Config["teamAssignments"]
	obj, _ := m.(map[string]any)
	if obj != nil {
		if v, ok := obj[gameutil.Itoa(rangeNum)]; ok {
			if s, ok := v.(string); ok && (s == "A" || s == "B") {
				return s
			}
		}
	}
	if rangeNum%2 == 0 {
		return "B"
	}
	return "A"
}

func (gs *GameState) parFor(p *Player) float64 {
	ov := gameutil.CfgFloatMap(gs.Config, "parOverrides")
	if v, ok := ov[gameutil.Itoa(p.RangeNum)]; ok && v > 0 {
		return v
	}
	lo := gameutil.CfgFloat(gs.Config, "parMin", 5)
	hi := gameutil.CfgFloat(gs.Config, "parMax", 10.5)
	if len(p.CalValues) > 0 {
		return gameutil.ClampFloat(gameutil.MeanFloats(p.CalValues), lo, hi)
	}
	return gameutil.ClampFloat(8.0, lo, hi)
}

func (gs *GameState) allCalibrated() bool {
	need := gameutil.CfgInt(gs.Config, "calibrateShots", 5)
	if need <= 0 {
		return true
	}
	seated := gs.seatNow()
	if len(seated) == 0 {
		return false
	}
	for _, p := range seated {
		if !p.Calibrated && len(p.CalValues) < need {
			return false
		}
	}
	return true
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
			gs.Players[k] = &Player{RangeNum: i, Active: true, WasWarmup: true, Color: gameutil.LaneColor(i), Team: gs.teamOf(i)}
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
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, Color: gameutil.LaneColor(rn)}
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
		if p.Calibrated {
			p.Ready = true
		}
	}
	if gs.Phase == gameutil.PhaseWarmup {
		gs.Phase = gameutil.PhaseArming
		gs.StatusLine = "Bereit — Tauziehen starten"
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
