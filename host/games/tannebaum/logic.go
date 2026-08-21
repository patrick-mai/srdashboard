package tannebaum

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"srdashboard/host/games/gameutil"
	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func init() {
	loader.RegisterBuiltin("tannebaum-einzel", func(m *loader.Manifest) logicapi.Logic {
		return New(m, ModeEinzel)
	})
	loader.RegisterBuiltin("tannebaum-team", func(m *loader.Manifest) logicapi.Logic {
		return New(m, ModeTeam)
	})
}

const (
	ModeEinzel = "einzel"
	ModeTeam   = "team"

	PhaseWarmup   = "warmup"
	PhaseArming   = "arming"
	PhasePlaying  = "playing"
	PhaseFinished = "finished"

	ResultOwn  = "own"
	ResultGift = "gift"
	ResultMiss = "miss"
)

var laneColors = []string{
	"#2f6b3a", "#c45c26", "#1f4e79", "#8b4513",
	"#4a7c59", "#b8860b", "#5c4033", "#2e5a4c",
}

type Logic struct {
	manifest *loader.Manifest
	mode     string
}

func New(m *loader.Manifest, mode string) *Logic {
	return &Logic{manifest: m, mode: mode}
}

func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	if l.mode == ModeTeam {
		return "tannebaum-team"
	}
	return "tannebaum-einzel"
}

func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	if l.mode == ModeTeam {
		return "Tannebaum Team"
	}
	return "Tannebaum Einzel"
}

func (l *Logic) Version() string {
	if l.manifest != nil && l.manifest.Version != "" {
		return l.manifest.Version
	}
	return "1.0.0"
}

func (l *Logic) DefaultConfig() map[string]any { return defaultConfig(l.mode) }

func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"autoStartWhenAllReady": map[string]any{"type": "boolean"},
			"defaultTargetProfile":  map[string]any{"type": "string"},
			"teamAssignments":       map[string]any{"type": "object"},
		},
	}
}

func defaultConfig(mode string) map[string]any {
	return map[string]any{
		"mode":                  mode,
		"autoStartWhenAllReady": true,
		"defaultTargetProfile":  "air_rifle_10m",
		"teamAssignments":       map[string]any{},
	}
}

// Shooter is a physical range / lane.
type Shooter struct {
	RangeNum    int    `json:"rangeNum"`
	Active      bool   `json:"active"`
	Ready       bool   `json:"ready"`
	WasWarmup   bool   `json:"wasWarmup"`
	ShooterName string `json:"shooterName"`
	Discipline  string `json:"discipline"`
	Color       string `json:"color"`
	ContenderID string `json:"contenderId"`
}

// Contender owns one tree (Einzel: one range; Team: one side).
type Contender struct {
	ID           string                    `json:"id"`
	Label        string                    `json:"label"`
	Color        string                    `json:"color"`
	RangeNums    []int                     `json:"rangeNums"`
	Stages       map[string]map[string]int `json:"stages"` // stageID → leafKey → remaining
	CurrentStage string                    `json:"currentStage"`
	Finished     bool                      `json:"finished"`
	FinishOrder  int                       `json:"finishOrder"`
}

type ShotMark struct {
	RangeNum    int     `json:"rangeNum"`
	ContenderID string  `json:"contenderId"`
	TargetID    string  `json:"targetId"`
	Raw         float64 `json:"raw"`
	Mapped      float64 `json:"mapped"`
	StageID     string  `json:"stageId"`
	Result      string  `json:"result"` // own | gift | miss
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Distance    float64 `json:"distance"`
	FullValue   int     `json:"fullValue"`
}

type GameState struct {
	Phase              string                `json:"phase"`
	Mode               string                `json:"mode"`
	NumRanges          int                   `json:"numRanges"`
	Config             map[string]any        `json:"config"`
	Shooters           map[string]*Shooter   `json:"shooters"`
	Contenders         map[string]*Contender `json:"contenders"`
	TeamPicks          map[string]string     `json:"teamPicks"` // rangeNum → "a"|"b" (runtime picks)
	Shots              []ShotMark            `json:"shots"`
	WinnerID           string                `json:"winnerId"`
	FinishCount        int                   `json:"finishCount"`
	StartBlockedReason string                `json:"startBlockedReason"`
	StatusLine         string                `json:"statusLine"`
	FieldOpen          bool                  `json:"fieldOpen"`
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := defaultConfig(l.mode)
	for k, v := range cfg {
		merged[k] = v
	}
	merged["mode"] = l.mode
	n := cfgInt(merged, "numRanges", 6)
	gs := &GameState{
		Phase:      PhaseWarmup,
		Mode:       l.mode,
		NumRanges:  n,
		Config:     merged,
		Shooters:   map[string]*Shooter{},
		Contenders: map[string]*Contender{},
		TeamPicks:  map[string]string{},
		Shots:      []ShotMark{},
		StatusLine: "Einschießen — danach Tannebaum",
	}
	for i := 1; i <= n; i++ {
		gs.Shooters[itoa(i)] = &Shooter{
			RangeNum:  i,
			Active:    true,
			WasWarmup: true,
			Color:     laneColors[(i-1)%len(laneColors)],
		}
	}
	gs.applyMembership(merged)
	gs.rebuildContenders()
	return marshalState(gs)
}

func (l *Logic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	return l.OnShotCtx(sess, logicapi.ShotContext{
		RangeNum:  rangeNum,
		Shot:      shot,
		ShotIndex: shotIndex,
		Live:      logicapi.LiveRangeInfo{IsWarmup: shot.IsWarmup},
		Now:       time.Now(),
	})
}

func (l *Logic) OnShotCtx(sess logicapi.SessionState, ctx logicapi.ShotContext) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	gs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	if gs.Mode == "" {
		gs.Mode = l.mode
	}
	gs.ensure()
	var events []logicapi.PluginEvent
	if ctx.InactiveRanges != nil {
		gs.applyInactiveList(ctx.InactiveRanges)
	}

	sh := gs.Shooters[itoa(ctx.RangeNum)]
	if sh == nil || !sh.Active {
		return marshalWithEvents(gs, nil)
	}
	if ctx.Live.ShooterName != "" {
		sh.ShooterName = ctx.Live.ShooterName
	}
	if ctx.Live.Discipline != "" {
		sh.Discipline = ctx.Live.Discipline
	}

	discard, openNow := gameutil.WarmupDiscard(gs.FieldOpen, gameutil.IsWarmupShot(ctx.Live.IsWarmup, ctx.Shot.IsWarmup))
	if discard {
		sh.WasWarmup = true
		return marshalWithEvents(gs, nil)
	}
	if openNow {
		gs.openCompetitionField()
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		if gs.Phase == PhaseArming && cfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allReady() {
			events = append(events, gs.beginPlay()...)
		} else if gs.Phase == PhaseArming {
			gs.StartBlockedReason = gs.blockReason()
		}
	}

	raw := rawShotValue(ctx.Shot.DecValue, ctx.Shot.FullValue)

	if gs.Phase != PhasePlaying {
		return marshalWithEvents(gs, events)
	}

	own := gs.contenderForRange(ctx.RangeNum)
	if own == nil || own.Finished {
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "raw": raw, "reason": "finished",
		}})
		return marshalWithEvents(gs, events)
	}

	candidates := mapShotCandidates(raw)
	mark := ShotMark{
		RangeNum: ctx.RangeNum, ContenderID: own.ID,
		Raw: raw, Result: ResultMiss,
		X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
	}
	if len(candidates) == 0 {
		gs.pushShot(mark)
		gs.StatusLine = "Fehl — Wert außerhalb A/B/C"
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "raw": raw,
		}})
		return marshalWithEvents(gs, events)
	}

	// 1) Strike own open leaf (prefer higher/more precise stage: C → B → A).
	for _, cand := range candidates {
		if hasLeaf(own.Stages[cand.StageID], cand.Value) {
			strikeLeaf(own.Stages[cand.StageID], cand.Value)
			mark.Mapped = cand.Value
			mark.StageID = cand.StageID
			mark.Result = ResultOwn
			mark.TargetID = own.ID
			gs.pushShot(mark)
			events = append(events, logicapi.PluginEvent{Type: "strike", Data: map[string]any{
				"rangeNum": ctx.RangeNum, "raw": raw, "mapped": cand.Value, "stageId": cand.StageID,
				"contenderId": own.ID, "gift": false,
			}})
			gs.afterStrike(own)
			events = append(events, gs.finishEvents(own)...)
			gs.refreshStatus()
			return marshalWithEvents(gs, events)
		}
	}

	// 2) Own matching stage(s) already cleared for this hit → gift to opponent still needing it.
	for _, cand := range candidates {
		target := gs.findGiftTarget(own.ID, cand.StageID, cand.Value)
		if target == nil {
			continue
		}
		strikeLeaf(target.Stages[cand.StageID], cand.Value)
		mark.Mapped = cand.Value
		mark.StageID = cand.StageID
		mark.Result = ResultGift
		mark.TargetID = target.ID
		gs.pushShot(mark)
		events = append(events, logicapi.PluginEvent{Type: "strike", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "raw": raw, "mapped": cand.Value, "stageId": cand.StageID,
			"contenderId": target.ID, "fromId": own.ID, "gift": true,
		}})
		gs.afterStrike(target)
		events = append(events, gs.finishEvents(target)...)
		gs.StatusLine = fmt.Sprintf("Geschenk → %s (Stufe %s: %s)", target.Label, cand.StageID, fmtLeaf(cand.Value))
		gs.refreshStatus()
		return marshalWithEvents(gs, events)
	}

	mark.Mapped = candidates[0].Value
	mark.StageID = candidates[0].StageID
	gs.pushShot(mark)
	gs.StatusLine = fmt.Sprintf("Fehl — %s schon geräumt", fmtLeaf(candidates[0].Value))
	events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{
		"rangeNum": ctx.RangeNum, "raw": raw, "mapped": mark.Mapped, "stageId": mark.StageID,
	}})
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
	if gs.Mode == "" {
		gs.Mode = l.mode
	}
	gs.ensure()
	var events []logicapi.PluginEvent
	switch action {
	case "sync_live":
		gs.applyLive(params)
		if gs.Phase == PhaseWarmup || gs.Phase == PhaseArming {
			if gs.anyReady() && gs.Phase == PhaseWarmup {
				gs.Phase = PhaseArming
			}
			if gs.Phase == PhaseArming && cfgBool(gs.Config, "autoStartWhenAllReady", true) && gs.allReady() {
				events = append(events, gs.beginPlay()...)
			} else {
				gs.StartBlockedReason = gs.blockReason()
			}
		}
	case "start":
		if gs.Phase == PhaseWarmup || gs.Phase == PhaseArming || gs.Phase == PhasePlaying {
			events = append(events, gs.beginPlay()...)
		}
	case "set_team":
		if gs.Mode != ModeTeam {
			return marshalWithEvents(gs, events)
		}
		rn := cfgInt(params, "rangeNum", 0)
		team := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["team"])))
		if rn < 1 || rn > gs.NumRanges {
			return marshalWithEvents(gs, events)
		}
		switch team {
		case "a", "team-a", "teama", "1":
			team = "a"
		case "b", "team-b", "teamb", "2":
			team = "b"
		default:
			return marshalWithEvents(gs, events)
		}
		if gs.TeamPicks == nil {
			gs.TeamPicks = map[string]string{}
		}
		gs.TeamPicks[itoa(rn)] = team
		gs.rebuildContenders()
		gs.StatusLine = fmt.Sprintf("Stand %d → Team %s", rn, strings.ToUpper(team))
		events = append(events, logicapi.PluginEvent{Type: "team_pick", Data: map[string]any{
			"rangeNum": rn, "team": team,
		}})
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
	if gs.Mode == "" {
		gs.Mode = l.mode
	}
	gs.ensure()

	contenders := make([]map[string]any, 0, len(gs.Contenders))
	ids := make([]string, 0, len(gs.Contenders))
	for id := range gs.Contenders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		contenders = append(contenders, contenderVM(gs.Contenders[id]))
	}

	shooters := make([]map[string]any, 0, gs.NumRanges)
	for i := 1; i <= gs.NumRanges; i++ {
		sh := gs.Shooters[itoa(i)]
		if sh == nil {
			continue
		}
		shooters = append(shooters, shooterVM(sh))
	}

	recent := gs.recentShots(5)
	recentVM := make([]map[string]any, 0, len(recent))
	for _, s := range recent {
		recentVM = append(recentVM, shotVM(s, gs))
	}

	me := gs.Shooters[itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = shooterVM(me)
	}
	myTree := gs.contenderForRange(rangeNum)
	var myTreeVM map[string]any
	if myTree != nil {
		myTreeVM = contenderVM(myTree)
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

	stageMeta := make([]map[string]any, 0, len(stageSpecs()))
	for _, sp := range stageSpecs() {
		stageMeta = append(stageMeta, map[string]any{
			"id": sp.ID, "label": sp.Label, "min": sp.Min, "max": sp.Max, "step": sp.Step,
		})
	}

	return map[string]any{
		"pluginId": l.ID(),
		"kind":     "game",
		"mode":     "shared",
		"label":    l.Label(),
		"rangeNum": rangeNum,
		"tree": map[string]any{
			"gameMode":             gs.Mode,
			"phase":                gs.Phase,
			"statusLine":           gs.StatusLine,
			"startBlockedReason":   gs.StartBlockedReason,
			"winnerId":             gs.WinnerID,
			"contenders":           contenders,
			"shooters":             shooters,
			"recentShots":          recentVM,
			"stageMeta":            stageMeta,
			"teamPicks":            gs.TeamPicks,
			"canPickTeam":          gs.Mode == ModeTeam && gs.Phase != PhaseFinished,
			"defaultTargetProfile": cfgString(gs.Config, "defaultTargetProfile", "air_rifle_10m"),
		},
		"me":          meVM,
		"myTree":      myTreeVM,
		"lastOwn":     lastOwn,
		"lastForeign": lastForeign,
	}, nil
}

func (gs *GameState) afterStrike(c *Contender) {
	if c == nil {
		return
	}
	c.CurrentStage = firstOpenStage(c)
	if totalRemaining(c) > 0 {
		return
	}
	if c.Finished {
		return
	}
	c.Finished = true
	c.CurrentStage = ""
	gs.FinishCount++
	c.FinishOrder = gs.FinishCount
	if gs.WinnerID == "" {
		gs.WinnerID = c.ID
		gs.Phase = PhaseFinished
		gs.StatusLine = fmt.Sprintf("%s hat den Tannebaum geräumt!", c.Label)
	}
}

func (gs *GameState) finishEvents(c *Contender) []logicapi.PluginEvent {
	if c == nil || !c.Finished {
		return nil
	}
	ev := []logicapi.PluginEvent{{Type: "cleared", Data: map[string]any{
		"contenderId": c.ID, "winner": gs.WinnerID == c.ID,
	}}}
	if gs.Phase == PhaseFinished && gs.WinnerID == c.ID {
		ev = append(ev, logicapi.PluginEvent{Type: "match_finished", Data: map[string]any{
			"winnerId": gs.WinnerID,
		}})
	}
	return ev
}

func firstOpenStage(c *Contender) string {
	if c == nil {
		return ""
	}
	for _, id := range stageOrder {
		if !stageCleared(c.Stages[id]) {
			return id
		}
	}
	return ""
}

func (gs *GameState) findGiftTarget(fromID, stageID string, value float64) *Contender {
	var best *Contender
	ids := make([]string, 0, len(gs.Contenders))
	for id := range gs.Contenders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == fromID {
			continue
		}
		c := gs.Contenders[id]
		if c == nil || c.Finished {
			continue
		}
		// Opponent must still have this stage to clear (not yet past it with no leaves).
		if stageCleared(c.Stages[stageID]) {
			continue
		}
		if !hasLeaf(c.Stages[stageID], value) {
			continue
		}
		if best == nil {
			best = c
			continue
		}
		// Prefer opponent still working this stage, then most remaining overall.
		if c.CurrentStage == stageID && best.CurrentStage != stageID {
			best = c
			continue
		}
		if totalRemaining(c) > totalRemaining(best) {
			best = c
		}
	}
	return best
}

func totalRemaining(c *Contender) int {
	if c == nil {
		return 0
	}
	n := 0
	for _, leaves := range c.Stages {
		n += stageRemaining(leaves)
	}
	return n
}

func (gs *GameState) beginPlay() []logicapi.PluginEvent {
	gs.rebuildContenders()
	gs.Phase = PhasePlaying
	gs.WinnerID = ""
	gs.FinishCount = 0
	gs.Shots = nil
	gs.StartBlockedReason = ""
	gs.StatusLine = "Tannebaum — Team wählen, dann räumen"
	for _, c := range gs.Contenders {
		if c == nil {
			continue
		}
		c.Finished = false
		c.FinishOrder = 0
		c.CurrentStage = StageA
		c.Stages = freshStages()
	}
	return []logicapi.PluginEvent{{Type: "match_start", Data: map[string]any{"mode": gs.Mode}}}
}

func freshStages() map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, sp := range stageSpecs() {
		out[sp.ID] = cloneLeaves(sp.Leaves)
	}
	return out
}

func (gs *GameState) rebuildContenders() {
	if gs.Contenders == nil {
		gs.Contenders = map[string]*Contender{}
	}
	if gs.TeamPicks == nil {
		gs.TeamPicks = map[string]string{}
	}
	assignments := map[string]any{}
	if a, ok := gs.Config["teamAssignments"].(map[string]any); ok {
		assignments = a
	}

	if gs.Mode == ModeTeam {
		oldA := gs.Contenders["team-a"]
		oldB := gs.Contenders["team-b"]
		teamA := &Contender{
			ID: "team-a", Label: "Team A", Color: laneColors[0],
			RangeNums: []int{}, Stages: freshStages(), CurrentStage: StageA,
		}
		teamB := &Contender{
			ID: "team-b", Label: "Team B", Color: laneColors[1],
			RangeNums: []int{}, Stages: freshStages(), CurrentStage: StageA,
		}
		// Keep tree progress when only membership changes.
		if oldA != nil && (gs.Phase == PhasePlaying || gs.Phase == PhaseFinished) {
			teamA.Stages = oldA.Stages
			teamA.CurrentStage = oldA.CurrentStage
			teamA.Finished = oldA.Finished
			teamA.FinishOrder = oldA.FinishOrder
		}
		if oldB != nil && (gs.Phase == PhasePlaying || gs.Phase == PhaseFinished) {
			teamB.Stages = oldB.Stages
			teamB.CurrentStage = oldB.CurrentStage
			teamB.Finished = oldB.Finished
			teamB.FinishOrder = oldB.FinishOrder
		}
		for i := 1; i <= gs.NumRanges; i++ {
			sh := gs.Shooters[itoa(i)]
			if sh == nil || !sh.Active {
				continue
			}
			teamKey := gs.teamKeyForRange(i, assignments)
			var c *Contender
			if teamKey == "b" {
				c = teamB
			} else {
				c = teamA
			}
			c.RangeNums = append(c.RangeNums, i)
			sh.ContenderID = c.ID
			sh.Color = c.Color
		}
		gs.Contenders = map[string]*Contender{}
		if len(teamA.RangeNums) > 0 {
			gs.Contenders[teamA.ID] = teamA
		}
		if len(teamB.RangeNums) > 0 {
			gs.Contenders[teamB.ID] = teamB
		}
		return
	}

	// Einzel: one tree per active range
	next := map[string]*Contender{}
	for i := 1; i <= gs.NumRanges; i++ {
		sh := gs.Shooters[itoa(i)]
		if sh == nil || !sh.Active {
			continue
		}
		id := "r" + itoa(i)
		label := "Stand " + itoa(i)
		if sh.ShooterName != "" {
			label = sh.ShooterName
		}
		c := &Contender{
			ID: id, Label: label, Color: sh.Color,
			RangeNums: []int{i}, Stages: freshStages(), CurrentStage: StageA,
		}
		if old, ok := gs.Contenders[id]; ok && old != nil && gs.Phase == PhasePlaying {
			c.Stages = old.Stages
			c.CurrentStage = old.CurrentStage
			c.Finished = old.Finished
			c.FinishOrder = old.FinishOrder
		}
		sh.ContenderID = id
		next[id] = c
	}
	gs.Contenders = next
}

// teamKeyForRange returns "a" or "b". Runtime TeamPicks win over config, then odd/even.
func (gs *GameState) teamKeyForRange(rangeNum int, assignments map[string]any) string {
	key := itoa(rangeNum)
	if gs.TeamPicks != nil {
		if v, ok := gs.TeamPicks[key]; ok {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "b", "team-b", "teamb", "2":
				return "b"
			case "a", "team-a", "teama", "1":
				return "a"
			}
		}
	}
	if assignments != nil {
		if v, ok := assignments[key]; ok {
			switch strings.ToLower(strings.TrimSpace(fmt.Sprint(v))) {
			case "b", "team-b", "teamb", "2":
				return "b"
			case "a", "team-a", "teama", "1":
				return "a"
			}
		}
	}
	if rangeNum%2 == 0 {
		return "b"
	}
	return "a"
}

func (gs *GameState) contenderForRange(rangeNum int) *Contender {
	sh := gs.Shooters[itoa(rangeNum)]
	if sh == nil {
		return nil
	}
	return gs.Contenders[sh.ContenderID]
}

func (gs *GameState) ensure() {
	if gs.Config == nil {
		gs.Config = defaultConfig(gs.Mode)
	}
	if gs.Shooters == nil {
		gs.Shooters = map[string]*Shooter{}
	}
	if gs.NumRanges < 1 {
		gs.NumRanges = 6
	}
	for i := 1; i <= gs.NumRanges; i++ {
		if gs.Shooters[itoa(i)] == nil {
			gs.Shooters[itoa(i)] = &Shooter{
				RangeNum: i, Active: true, WasWarmup: true,
				Color: laneColors[(i-1)%len(laneColors)],
			}
		}
	}
	gs.applyMembership(gs.Config)
	if len(gs.Contenders) == 0 {
		gs.rebuildContenders()
	}
}

func (gs *GameState) allReady() bool {
	n := 0
	for _, sh := range gs.Shooters {
		if sh == nil || !sh.Active {
			continue
		}
		n++
		if !sh.Ready {
			return false
		}
	}
	return n > 0
}

func (gs *GameState) anyReady() bool {
	for _, sh := range gs.Shooters {
		if sh != nil && sh.Active && sh.Ready {
			return true
		}
	}
	return false
}

func (gs *GameState) blockReason() string {
	missing := []int{}
	for i := 1; i <= gs.NumRanges; i++ {
		sh := gs.Shooters[itoa(i)]
		if sh == nil || !sh.Active {
			continue
		}
		if !sh.Ready {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("Warte auf Bereitschaft: Stände %v", missing)
}

func (gs *GameState) refreshStatus() {
	if gs.Phase == PhaseFinished {
		return
	}
	if gs.Phase != PhasePlaying {
		return
	}
	gs.StatusLine = "Tannebaum läuft"
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
		gs.NumRanges = cfgInt(map[string]any{"n": n}, "n", gs.NumRanges)
		gs.ensure()
	}
	gs.applyMembership(params)
	live, _ := params["live"].(map[string]any)
	if live == nil {
		if gs.Mode == ModeTeam || gs.Phase != PhasePlaying {
			gs.rebuildContenders()
		}
		return
	}
	for k, raw := range live {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rn, _ := strconv.Atoi(k)
		if rn < 1 {
			rn = cfgInt(m, "rangeNum", 0)
		}
		if rn < 1 {
			continue
		}
		sh := gs.Shooters[itoa(rn)]
		if sh == nil {
			sh = &Shooter{RangeNum: rn, Active: true, WasWarmup: true, Color: laneColors[(rn-1)%len(laneColors)]}
			gs.Shooters[itoa(rn)] = sh
		}
		if name, ok := m["shooterName"].(string); ok && name != "" {
			sh.ShooterName = name
		}
		if disc, ok := m["discipline"].(string); ok && disc != "" {
			sh.Discipline = disc
		}
		if warmup, ok := m["isWarmup"].(bool); ok && warmup && !gs.FieldOpen {
			sh.WasWarmup = true
		}
		sh.Active = gameutil.LiveActive(m, sh.Active)
	}
	if gs.Mode == ModeTeam || gs.Phase != PhasePlaying {
		gs.rebuildContenders()
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
		sh := gs.Shooters[itoa(i)]
		if sh != nil {
			sh.Active = !skip[i]
		}
	}
}

func (gs *GameState) openCompetitionField() {
	gs.FieldOpen = true
	for _, sh := range gs.Shooters {
		if sh == nil || !sh.Active {
			continue
		}
		sh.WasWarmup = false
		sh.Ready = true
	}
	if gs.Phase == PhaseWarmup {
		gs.Phase = PhaseArming
		gs.StatusLine = "Bereit — Tannebaum starten"
	}
}

func contenderVM(c *Contender) map[string]any {
	if c == nil {
		return nil
	}
	stages := map[string]any{}
	for _, sp := range stageSpecs() {
		leaves := c.Stages[sp.ID]
		items := make([]map[string]any, 0)
		vals := make([]float64, 0, len(sp.Leaves))
		for k := range sp.Leaves {
			vals = append(vals, parseLeafKey(k))
		}
		sort.Float64s(vals)
		for _, v := range vals {
			k := leafKey(v)
			rem := 0
			if leaves != nil {
				rem = leaves[k]
			}
			max := sp.Leaves[k]
			items = append(items, map[string]any{
				"value": v, "label": fmtLeaf(v), "remaining": rem, "max": max,
				"cleared": rem <= 0,
			})
		}
		stages[sp.ID] = map[string]any{
			"id": sp.ID, "label": sp.Label,
			"remaining": stageRemaining(leaves),
			"cleared":   stageCleared(leaves),
			"leaves":    items,
			"active":    c.CurrentStage == sp.ID,
		}
	}
	return map[string]any{
		"id": c.ID, "label": c.Label, "color": c.Color,
		"rangeNums": c.RangeNums, "currentStage": c.CurrentStage,
		"finished": c.Finished, "finishOrder": c.FinishOrder,
		"remaining": totalRemaining(c), "stages": stages,
	}
}

func shooterVM(sh *Shooter) map[string]any {
	if sh == nil {
		return nil
	}
	team := ""
	if sh.ContenderID == "team-a" {
		team = "a"
	} else if sh.ContenderID == "team-b" {
		team = "b"
	}
	return map[string]any{
		"rangeNum": sh.RangeNum, "active": sh.Active, "ready": sh.Ready,
		"shooterName": sh.ShooterName, "discipline": sh.Discipline,
		"color": sh.Color, "contenderId": sh.ContenderID, "team": team,
	}
}

func shotVM(s ShotMark, gs *GameState) map[string]any {
	color := ""
	if sh := gs.Shooters[itoa(s.RangeNum)]; sh != nil {
		color = sh.Color
	}
	return map[string]any{
		"rangeNum": s.RangeNum, "contenderId": s.ContenderID, "targetId": s.TargetID,
		"raw": s.Raw, "mapped": s.Mapped, "stageId": s.StageID, "result": s.Result,
		"x": s.X, "y": s.Y, "distance": s.Distance, "fullValue": s.FullValue,
		"color": color,
	}
}

func fmtLeaf(v float64) string {
	return fmtFloat1(v)
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
		return &GameState{
			Shooters: map[string]*Shooter{}, Contenders: map[string]*Contender{},
			Config: defaultConfig(ModeEinzel), Mode: ModeEinzel,
		}, nil
	}
	if err := json.Unmarshal(sess, &gs); err != nil {
		return nil, err
	}
	if gs.Config == nil {
		gs.Config = defaultConfig(gs.Mode)
	}
	if gs.Shooters == nil {
		gs.Shooters = map[string]*Shooter{}
	}
	if gs.Contenders == nil {
		gs.Contenders = map[string]*Contender{}
	}
	if gs.TeamPicks == nil {
		gs.TeamPicks = map[string]string{}
	}
	return &gs, nil
}

func marshalWithEvents(gs *GameState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := marshalState(gs)
	return st, events, err
}

func cfgInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := strconv.Atoi(t)
		if err == nil {
			return i
		}
	}
	return def
}

func cfgString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func cfgBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
