package ansageduell

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
	loader.RegisterBuiltin("ansage-duell", func(m *loader.Manifest) logicapi.Logic { return New(m) })
}

type Logic struct{ manifest *loader.Manifest }

func New(m *loader.Manifest) *Logic { return &Logic{manifest: m} }
func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "ansage-duell"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Ansage-Duell"
}
func (l *Logic) Version() string { return "1.0.0" }
func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{
		"autoStartWhenAllReady": true, "rounds": 8, "calibrateShots": 5, "failPenaltyFactor": 0.5,
		"holeInHoleMinOverlap": 0.5, "holeInHoleMinValue": 8.5, "shotDiameterMm": 4.5, "dsgPerMm": 100.0,
		"defaultTargetProfile": "air_rifle_10m", "disciplineTargets": gameutil.DefaultDisciplineTargets(),
		"handicaps": map[string]any{},
	}
}
func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"rounds":            map[string]any{"type": "integer"},
		"calibrateShots":    map[string]any{"type": "integer"},
		"failPenaltyFactor": map[string]any{"type": "number"},
	}}
}

type contractDef struct {
	ID, Label, Kind string
	Shots, Count    int
	Reward, Offset  float64
}

func allContracts() []contractDef {
	return []contractDef{
		{ID: "avg02", Label: "Schnitt +0,2", Kind: "avg", Shots: 3, Reward: 2, Offset: 0.2},
		{ID: "avg05", Label: "Schnitt +0,5", Kind: "avg", Shots: 3, Reward: 4, Offset: 0.5},
		{ID: "one10", Label: "Ein Schuss +1,0", Kind: "one", Shots: 1, Reward: 3, Offset: 1.0},
		{ID: "hih1", Label: "1× Hole-in-Hole", Kind: "hih", Shots: 4, Reward: 6, Count: 1},
		{ID: "hih2", Label: "2× Hole-in-Hole", Kind: "hih", Shots: 6, Reward: 12, Count: 2},
	}
}
func contractByID(id string) (contractDef, bool) {
	for _, c := range allContracts() {
		if c.ID == id {
			return c, true
		}
	}
	return contractDef{}, false
}

type Player struct {
	RangeNum, ShotsFired, Completed, ContractShots, HiHCount int
	Active, Seated, Ready, WasWarmup, HasLast, Calibrated    bool
	ShooterName, Discipline, Color, Hint, LastNote           string
	ContractID, ContractLabel, Progress                      string
	Handicap, LastRaw, Points, Par                           float64
	CalValues, ContractVals                                  []float64
	LastX, LastY                                             int
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
	NumRanges, WinnerRange                int
	FieldOpen                             bool
	Config                                map[string]any
	Players                               map[string]*Player
	Shots                                 []ShotMark
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := gameutil.MergeConfig(l.DefaultConfig(), cfg)
	gs := &GameState{Phase: gameutil.PhaseWarmup, NumRanges: gameutil.CfgInt(merged, "numRanges", 6), Config: merged, Players: map[string]*Player{}, StatusLine: "Ansage-Duell"}
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
			}
			if gs.Phase == gameutil.PhaseArming && gameutil.CfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allSeatedReady() {
				events = append(events, gs.beginPlay()...)
			}
		}
		if gs.Phase != gameutil.PhasePlaying {
			return marshalWithEvents(gs, events)
		}
	}

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
	needRounds := gameutil.CfgInt(gs.Config, "rounds", 8)
	if p.Completed >= needRounds {
		return marshalWithEvents(gs, events)
	}
	if p.ContractID == "" {
		gs.assignContract(p, "avg02")
	}
	c, ok := contractByID(p.ContractID)
	if !ok {
		gs.assignContract(p, "avg02")
		c, _ = contractByID(p.ContractID)
	}
	hih := false
	if p.HasLast {
		if ok, ov := gameutil.HoleInHole(float64(p.LastX), float64(p.LastY), float64(ctx.Shot.X), float64(ctx.Shot.Y), raw, gs.Config); ok {
			hih = true
			p.HiHCount++
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{"rangeNum": p.RangeNum, "overlap": ov}})
		}
	}
	p.ContractVals = append(p.ContractVals, raw)
	p.ContractShots++
	p.ShotsFired++
	p.LastRaw, p.LastX, p.LastY, p.HasLast = raw, ctx.Shot.X, ctx.Shot.Y, true
	note := fmt.Sprintf("%.1f", raw)
	if hih {
		note = "Hole-in-Hole"
	}
	p.Progress = fmt.Sprintf("%d/%d", p.ContractShots, c.Shots)
	p.Hint = p.ContractLabel + " · " + p.Progress
	p.LastNote = note
	gs.pushShot(ShotMark{RangeNum: p.RangeNum, Raw: raw, Result: "contract", Note: note, X: ctx.Shot.X, Y: ctx.Shot.Y, Color: p.Color})
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	if p.ContractShots >= c.Shots {
		events = append(events, gs.resolveContract(p, c)...)
		gs.maybeFinish()
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
	case "bid":
		rn := gameutil.CfgInt(params, "rangeNum", 0)
		if rn < 1 {
			rn = gameutil.CfgInt(params, "range", 0)
		}
		cid := gameutil.CfgString(params, "contractId", "")
		if cid == "" {
			cid = gameutil.CfgString(params, "id", "")
		}
		p := gs.Players[gameutil.Itoa(rn)]
		if gs.Phase == gameutil.PhasePlaying && p != nil && p.Seated && p.ContractShots == 0 {
			if _, ok := contractByID(cid); ok {
				gs.assignContract(p, cid)
			}
		}
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
			"contractLabel": p.ContractLabel, "points": p.Points, "progress": p.Progress,
		})
	}
	recent := []map[string]any{}
	for _, s := range gs.recentShots(8) {
		recent = append(recent, map[string]any{"rangeNum": s.RangeNum, "raw": s.Raw, "note": s.Note, "x": s.X, "y": s.Y, "color": s.Color})
	}
	contracts := []map[string]any{}
	for _, c := range allContracts() {
		contracts = append(contracts, map[string]any{"id": c.ID, "label": c.Label})
	}
	var me map[string]any
	if p := gs.Players[gameutil.Itoa(rangeNum)]; p != nil {
		me = map[string]any{"rangeNum": p.RangeNum, "label": gameutil.PlayerLabel(p.ShooterName, p.RangeNum), "hint": p.Hint,
			"lastRaw": p.LastRaw, "contractLabel": p.ContractLabel, "points": p.Points, "progress": p.Progress}
	}
	return map[string]any{"pluginId": l.ID(), "kind": "game", "mode": "shared", "label": l.Label(), "rangeNum": rangeNum,
		"game": map[string]any{"phase": gs.Phase, "statusLine": gs.StatusLine, "players": players, "recentShots": recent,
			"winnerRange": gs.WinnerRange, "contracts": contracts, "defaultTargetProfile": "air_rifle_10m"},
		"me": me}, nil
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.Phase, gs.FieldOpen, gs.WinnerRange, gs.Shots = gameutil.PhasePlaying, true, 0, nil
	gs.StatusLine = "Ansage — halten bringt Punkte"
	for _, p := range gs.Players {
		if p == nil {
			continue
		}
		p.Seated, p.Points, p.Completed, p.HasLast, p.ShotsFired = false, 0, 0, false, 0
		p.ContractID, p.ContractLabel, p.Progress, p.ContractShots, p.HiHCount = "", "", "", 0, 0
		p.ContractVals = nil
		p.Handicap = gameutil.HandicapFor(gs.Config, p.RangeNum)
		p.Par = gs.parFor(p)
		p.Hint = fmt.Sprintf("Schnitt %.1f", p.Par)
	}
	for _, p := range gs.seatNow() {
		p.Seated, p.Ready = true, true
	}
	return []logicapi.PluginEvent{{Type: "match_start"}}
}

func (gs *GameState) assignContract(p *Player, id string) {
	c, ok := contractByID(id)
	if !ok {
		return
	}
	p.ContractID, p.ContractLabel = c.ID, c.Label
	p.ContractShots, p.HiHCount, p.ContractVals = 0, 0, nil
	p.Progress = fmt.Sprintf("0/%d", c.Shots)
	p.Hint = c.Label
}

func (gs *GameState) resolveContract(p *Player, c contractDef) []logicapi.PluginEvent {
	ok := false
	switch c.Kind {
	case "avg":
		ok = gameutil.MeanFloats(p.ContractVals) >= p.Par+c.Offset
	case "one":
		for _, v := range p.ContractVals {
			if v >= p.Par+c.Offset {
				ok = true
				break
			}
		}
	case "hih":
		ok = p.HiHCount >= c.Count
	}
	factor := gameutil.CfgFloat(gs.Config, "failPenaltyFactor", 0.5)
	note := "gehalten"
	delta := c.Reward
	if !ok {
		delta = -c.Reward * factor
		note = "verfehlt"
	}
	p.Points += delta
	p.Completed++
	p.LastNote = note
	p.Hint = fmt.Sprintf("%s · %.1f Pkt", note, p.Points)
	events := []logicapi.PluginEvent{{Type: "contract_resolved", Data: map[string]any{"rangeNum": p.RangeNum, "ok": ok, "points": p.Points}}}
	p.ContractID, p.ContractLabel, p.Progress, p.ContractShots, p.HiHCount = "", "", "", 0, 0
	p.ContractVals = nil
	gs.StatusLine = gameutil.PlayerLabel(p.ShooterName, p.RangeNum) + " " + note
	return events
}

func (gs *GameState) maybeFinish() {
	need := gameutil.CfgInt(gs.Config, "rounds", 8)
	seated := gs.seatedList()
	if len(seated) == 0 {
		return
	}
	for _, p := range seated {
		if p.Completed < need {
			return
		}
	}
	best := seated[0]
	for _, p := range seated[1:] {
		if p.Points > best.Points {
			best = p
		}
	}
	gs.WinnerRange, gs.Phase = best.RangeNum, gameutil.PhaseFinished
	gs.StatusLine = gameutil.PlayerLabel(best.ShooterName, best.RangeNum) + " gewinnt"
}

func (gs *GameState) parFor(p *Player) float64 {
	if len(p.CalValues) > 0 {
		return gameutil.MeanFloats(p.CalValues)
	}
	return 8.0
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
	for i := 1; i <= gs.NumRanges; i++ {
		k := gameutil.Itoa(i)
		if gs.Players[k] == nil {
			gs.Players[k] = &Player{RangeNum: i, Active: true, WasWarmup: true, Color: gameutil.LaneColor(i), Par: 8}
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
			p = &Player{RangeNum: rn, Active: true, WasWarmup: true, Color: gameutil.LaneColor(rn), Par: 8}
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
