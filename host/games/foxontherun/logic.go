package foxontherun

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func init() {
	loader.RegisterBuiltin("fox-on-the-run", func(m *loader.Manifest) logicapi.Logic {
		return New(m)
	})
}

const (
	PhaseCalibrate    = "calibrate"
	PhaseArming       = "arming"
	PhaseOpening      = "opening"
	PhaseChase        = "chase"
	PhaseRoundResult  = "round_result"
	PhaseFinished     = "finished"

	OutcomeEscaped = "escaped"
	OutcomeCaught  = "caught"
	OutcomeNone    = ""
)

var laneColors = []string{
	"#e85d04", // fox-orange
	"#2a9d8f", // teal
	"#264653", // charcoal teal
	"#e9c46a", // sand
	"#9b2226", // crimson
	"#457b9d", // steel blue
}

type Logic struct {
	manifest *loader.Manifest
}

func New(m *loader.Manifest) *Logic {
	return &Logic{manifest: m}
}

func (l *Logic) ID() string {
	if l.manifest != nil && l.manifest.ID != "" {
		return l.manifest.ID
	}
	return "fox-on-the-run"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Fox on the Run"
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
			"calibrateShots":         map[string]any{"type": "integer"},
			"openingShots":           map[string]any{"type": "integer"},
			"escapeTarget":           map[string]any{"type": "number"},
			"equalizerEnabled":       map[string]any{"type": "boolean"},
			"equalizerMin":           map[string]any{"type": "number"},
			"equalizerMax":           map[string]any{"type": "number"},
			"startBonusFactor":       map[string]any{"type": "number"},
			"startBonusMin":          map[string]any{"type": "number"},
			"startBonusMax":          map[string]any{"type": "number"},
			"terrainEnabled":         map[string]any{"type": "boolean"},
			"terrainBand":            map[string]any{"type": "number"},
			"terrainDampMax":         map[string]any{"type": "number"},
			"trailAidMax":            map[string]any{"type": "number"},
			"maxChaseShots":          map[string]any{"type": "integer"},
			"autoStartWhenAllReady":  map[string]any{"type": "boolean"},
			"defaultTargetProfile":   map[string]any{"type": "string"},
			"skillOverrides":         map[string]any{"type": "object"},
			"equalizerOverrides":     map[string]any{"type": "object"},
		},
	}
}

func defaultConfig() map[string]any {
	return map[string]any{
		"calibrateShots":        5,
		"openingShots":          2,
		"escapeTarget":          30.0,
		"equalizerEnabled":      true,
		"equalizerMin":          -2.0,
		"equalizerMax":          2.0,
		"startBonusFactor":      1.0,
		"startBonusMin":         0.0,
		"startBonusMax":         8.0,
		"terrainEnabled":        true,
		"terrainBand":           5.0,
		"terrainDampMax":        1.0,
		"trailAidMax":           0.0,
		"maxChaseShots":         40,
		"autoStartWhenAllReady": true,
		"defaultTargetProfile":  "air_rifle_10m",
		"skillOverrides":        map[string]any{},
		"equalizerOverrides":    map[string]any{},
	}
}

type Player struct {
	RangeNum     int       `json:"rangeNum"`
	Active       bool      `json:"active"`
	Color        string    `json:"color"`
	ShooterName  string    `json:"shooterName"`
	Discipline   string    `json:"discipline"`
	Skill        float64   `json:"skill"`
	Equalizer    float64   `json:"equalizer"`
	CalValues    []float64 `json:"calValues"`
	Calibrated   bool      `json:"calibrated"`
	Ready        bool      `json:"ready"`
	WasWarmup    bool      `json:"wasWarmup"`
	FoxScore     float64   `json:"foxScore"`
	FoxChaseShots int      `json:"foxChaseShots"`
	Outcome      string    `json:"outcome"`
	HadFoxTurn   bool      `json:"hadFoxTurn"`
}

type ShotMark struct {
	RangeNum   int     `json:"rangeNum"`
	Role       string  `json:"role"` // fox | hunter | opening | calibrate
	Raw        float64 `json:"raw"`
	Delta      float64 `json:"delta"`
	LeadAfter  float64 `json:"leadAfter"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Distance   float64 `json:"distance"`
	FullValue  int     `json:"fullValue"`
	TerrainID  string  `json:"terrainId,omitempty"`
	TerrainLbl string  `json:"terrainLabel,omitempty"`
	TerrainAmt float64 `json:"terrainAmount,omitempty"`
}

type RoundResult struct {
	FoxRange      int     `json:"foxRange"`
	Outcome       string  `json:"outcome"`
	FinalLead     float64 `json:"finalLead"`
	FoxChaseShots int     `json:"foxChaseShots"`
	OpeningSum    float64 `json:"openingSum"`
	StartBonus    float64 `json:"startBonus"`
}

type HuntState struct {
	Phase              string                  `json:"phase"`
	NumRanges          int                     `json:"numRanges"`
	Config             map[string]any          `json:"config"`
	Players            map[string]*Player      `json:"players"`
	FoxOrder           []int                   `json:"foxOrder"`
	FoxIndex           int                     `json:"foxIndex"`
	CurrentFox         int                     `json:"currentFox"`
	TurnRange          int                     `json:"turnRange"`
	HunterIdx          int                     `json:"hunterIdx"`
	ExpectFoxShot      bool                    `json:"expectFoxShot"` // after a hunter, next is fox
	Lead               float64                 `json:"lead"`
	OpeningCount       int                     `json:"openingCount"`
	OpeningSum         float64                 `json:"openingSum"`
	StartBonus         float64                 `json:"startBonus"`
	FieldAvg           float64                 `json:"fieldAvg"`
	ChaseShots         int                     `json:"chaseShots"`
	FoxChaseShots      int                     `json:"foxChaseShots"`
	HuntShots          []ShotMark              `json:"huntShots"`
	RoundResults       []RoundResult           `json:"roundResults"`
	LastTerrain        *terrainEffect          `json:"lastTerrain,omitempty"`
	StartBlockedReason string                  `json:"startBlockedReason"`
	StatusLine         string                  `json:"statusLine"`
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := defaultConfig()
	for k, v := range cfg {
		merged[k] = v
	}
	n := cfgInt(merged, "numRanges", 6)
	hs := &HuntState{
		Phase:     PhaseCalibrate,
		NumRanges: n,
		Config:    merged,
		Players:   map[string]*Player{},
		FoxOrder:  nil,
		HuntShots: []ShotMark{},
		StatusLine: "Einschießen — Kalibrierung der Meute",
	}
	for i := 1; i <= n; i++ {
		hs.Players[itoa(i)] = &Player{
			RangeNum:  i,
			Active:    true,
			Color:     laneColors[(i-1)%len(laneColors)],
			WasWarmup: true,
			CalValues: []float64{},
		}
	}
	return marshalState(hs)
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
	hs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	hs.ensurePlayers()
	var events []logicapi.PluginEvent
	p := hs.Players[itoa(ctx.RangeNum)]
	if p == nil || !p.Active {
		return marshalWithEvents(hs, nil)
	}
	if ctx.Live.ShooterName != "" {
		p.ShooterName = ctx.Live.ShooterName
	}
	if ctx.Live.Discipline != "" {
		p.Discipline = ctx.Live.Discipline
	}

	if ctx.Live.IsWarmup || ctx.Shot.IsWarmup {
		p.WasWarmup = true
		return marshalWithEvents(hs, nil)
	}
	if p.WasWarmup && !ctx.Live.IsWarmup {
		p.WasWarmup = false
		p.Ready = true
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
	}

	raw := rawShotValue(ctx.Shot.DecValue, ctx.Shot.FullValue)

	switch hs.Phase {
	case PhaseCalibrate:
		need := cfgInt(hs.Config, "calibrateShots", 5)
		if !p.Calibrated {
			p.CalValues = append(p.CalValues, raw)
			events = append(events, logicapi.PluginEvent{Type: "cal_shot", Data: map[string]any{
				"rangeNum": ctx.RangeNum, "raw": raw, "n": len(p.CalValues), "need": need,
			}})
			if len(p.CalValues) >= need {
				p.Calibrated = true
				p.Ready = true
			}
		}
		hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
		if hs.allCalibrated() {
			hs.Phase = PhaseArming
			hs.StatusLine = "Waidmannsheil — Meute bereit"
			if cfgBool(hs.Config, "autoStartWhenAllReady", true) && hs.allReady() {
				events = append(events, hs.beginMatch()...)
			} else {
				hs.StartBlockedReason = hs.blockReason()
			}
		} else {
			hs.StatusLine = fmt.Sprintf("Kalibrierung — noch nicht alle Stände fertig")
		}
		return marshalWithEvents(hs, events)

	case PhaseArming:
		// ignore scoring shots until start
		return marshalWithEvents(hs, events)

	case PhaseOpening:
		if ctx.RangeNum != hs.CurrentFox {
			events = append(events, logicapi.PluginEvent{Type: "wrong_turn", Data: map[string]any{"rangeNum": ctx.RangeNum, "expected": hs.CurrentFox}})
			return marshalWithEvents(hs, events)
		}
		need := cfgInt(hs.Config, "openingShots", 2)
		fp := hs.Players[itoa(hs.CurrentFox)]
		eq := 0.0
		if fp != nil {
			eq = fp.Equalizer
		}
		delta := effectiveDelta(raw, eq)
		hs.OpeningCount++
		hs.OpeningSum += delta
		hs.Lead = hs.OpeningSum // bonus applied when opening completes
		mark := ShotMark{
			RangeNum: ctx.RangeNum, Role: "opening", Raw: raw, Delta: delta,
			LeadAfter: hs.Lead, X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
		}
		hs.pushHuntShot(mark)
		events = append(events, logicapi.PluginEvent{Type: "opening_shot", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "raw": raw, "delta": delta, "lead": hs.Lead, "n": hs.OpeningCount,
		}})
		if hs.OpeningCount >= need {
			skill := 0.0
			if fp != nil {
				skill = fp.Skill
			}
			hs.StartBonus = startBonusForFox(skill, hs.FieldAvg, hs.Config)
			hs.Lead = hs.OpeningSum + hs.StartBonus
			escape := cfgFloat(hs.Config, "escapeTarget", 30)
			if hs.Lead >= escape {
				events = append(events, hs.finishHunt(OutcomeEscaped)...)
				return marshalWithEvents(hs, events)
			}
			hs.Phase = PhaseChase
			hs.ExpectFoxShot = false
			hs.HunterIdx = 0
			hs.setNextHunterTurn()
			hs.StatusLine = "Die Meute ist auf der Fährte"
			events = append(events, logicapi.PluginEvent{Type: "chase_start", Data: map[string]any{
				"fox": hs.CurrentFox, "lead": hs.Lead, "startBonus": hs.StartBonus,
			}})
			events = append(events, logicapi.PluginEvent{Type: "turn", Data: map[string]any{"rangeNum": hs.TurnRange}})
		} else {
			hs.StatusLine = fmt.Sprintf("Fuchs legt vor — Vorwurf %d/%d", hs.OpeningCount, need)
		}
		return marshalWithEvents(hs, events)

	case PhaseChase:
		if ctx.RangeNum != hs.TurnRange {
			events = append(events, logicapi.PluginEvent{Type: "wrong_turn", Data: map[string]any{"rangeNum": ctx.RangeNum, "expected": hs.TurnRange}})
			return marshalWithEvents(hs, events)
		}
		isFox := ctx.RangeNum == hs.CurrentFox
		fp := hs.Players[itoa(ctx.RangeNum)]
		eq := 0.0
		if fp != nil {
			eq = fp.Equalizer
		}
		escape := cfgFloat(hs.Config, "escapeTarget", 30)
		terr := computeTerrain(hs.Lead, escape, hs.ChaseShots, isFox, hs.Config)
		base := effectiveDelta(raw, eq)
		delta := applyTerrainToDelta(base, terr, isFox)
		if terr.Amount > 0 {
			hs.LastTerrain = &terr
		} else {
			hs.LastTerrain = nil
		}
		if isFox {
			hs.Lead += delta
			hs.FoxChaseShots++
			if fp != nil {
				fp.FoxChaseShots++
			}
		} else {
			hs.Lead -= delta
		}
		hs.ChaseShots++
		mark := ShotMark{
			RangeNum: ctx.RangeNum, Role: map[bool]string{true: "fox", false: "hunter"}[isFox],
			Raw: raw, Delta: delta, LeadAfter: hs.Lead,
			X: ctx.Shot.X, Y: ctx.Shot.Y, Distance: ctx.Shot.Distance, FullValue: ctx.Shot.FullValue,
			TerrainID: terr.FlavorID, TerrainLbl: terr.FlavorLabel, TerrainAmt: terr.Amount,
		}
		hs.pushHuntShot(mark)
		events = append(events, logicapi.PluginEvent{Type: "hunt_shot", Data: map[string]any{
			"rangeNum": ctx.RangeNum, "isFox": isFox, "raw": raw, "delta": delta, "lead": hs.Lead,
			"terrainId": terr.FlavorID, "terrainLabel": terr.FlavorLabel, "terrainAmount": terr.Amount,
		}})

		if hs.Lead >= escape {
			events = append(events, hs.finishHunt(OutcomeEscaped)...)
			return marshalWithEvents(hs, events)
		}
		if hs.Lead <= 0 {
			hs.Lead = hs.Lead // may be slightly negative
			events = append(events, hs.finishHunt(OutcomeCaught)...)
			return marshalWithEvents(hs, events)
		}
		maxChase := cfgInt(hs.Config, "maxChaseShots", 40)
		if hs.ChaseShots >= maxChase {
			hs.StatusLine = "Lange Jagd — Vorsprung hält sich in der Lichtung"
		}

		// Advance turn: hunter then fox then next hunter...
		if isFox {
			hs.ExpectFoxShot = false
			hs.setNextHunterTurn()
		} else {
			hs.ExpectFoxShot = true
			hs.TurnRange = hs.CurrentFox
		}
		events = append(events, logicapi.PluginEvent{Type: "turn", Data: map[string]any{"rangeNum": hs.TurnRange}})
		if hs.LastTerrain != nil && hs.LastTerrain.FlavorLabel != "" {
			hs.StatusLine = hs.LastTerrain.FlavorLabel
		} else if isFox {
			hs.StatusLine = "Der Fuchs zieht zum Bau"
		} else {
			hs.StatusLine = "Die Meute ist auf der Fährte"
		}
		return marshalWithEvents(hs, events)

	default:
		return marshalWithEvents(hs, events)
	}
}

func (l *Logic) Tick(sess logicapi.SessionState, now time.Time) (logicapi.SessionState, []logicapi.PluginEvent, bool, error) {
	return sess, nil, false, nil
}

func (l *Logic) Control(sess logicapi.SessionState, action string, params map[string]any) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	hs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	hs.ensurePlayers()
	var events []logicapi.PluginEvent
	switch action {
	case "sync_live":
		hs.applyLive(params)
		if hs.Phase == PhaseCalibrate || hs.Phase == PhaseArming {
			hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
			if hs.Phase == PhaseCalibrate && hs.allCalibrated() {
				hs.Phase = PhaseArming
			}
			if hs.Phase == PhaseArming && cfgBool(hs.Config, "autoStartWhenAllReady", true) && hs.allReady() && hs.allCalibrated() {
				events = append(events, hs.beginMatch()...)
			} else {
				hs.StartBlockedReason = hs.blockReason()
			}
		}
	case "start":
		if hs.Phase == PhaseCalibrate || hs.Phase == PhaseArming {
			// force equalizers from whatever we have
			hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
			events = append(events, hs.beginMatch()...)
		}
	case "reset":
		cfg := hs.Config
		n := hs.NumRanges
		fresh, _ := l.Init(cfg)
		hs2, _ := unmarshalState(fresh)
		hs2.NumRanges = n
		hs2.ensurePlayers()
		return marshalWithEvents(hs2, []logicapi.PluginEvent{{Type: "reset"}})
	case "set_skill":
		if hs.Config["skillOverrides"] == nil {
			hs.Config["skillOverrides"] = map[string]any{}
		}
		if m, ok := hs.Config["skillOverrides"].(map[string]any); ok {
			for k, v := range params {
				if k == "live" || k == "numRanges" {
					continue
				}
				m[k] = v
			}
		}
		hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
	case "set_equalizer":
		if hs.Config["equalizerOverrides"] == nil {
			hs.Config["equalizerOverrides"] = map[string]any{}
		}
		if m, ok := hs.Config["equalizerOverrides"].(map[string]any); ok {
			for k, v := range params {
				if k == "live" || k == "numRanges" {
					continue
				}
				m[k] = v
			}
		}
		hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
	}
	return marshalWithEvents(hs, events)
}

func (l *Logic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	hs, err := unmarshalState(sess)
	if err != nil {
		return nil, err
	}
	hs.ensurePlayers()
	escape := cfgFloat(hs.Config, "escapeTarget", 30)
	players := make([]map[string]any, 0, hs.NumRanges)
	for i := 1; i <= hs.NumRanges; i++ {
		p := hs.Players[itoa(i)]
		if p == nil {
			continue
		}
		players = append(players, playerVM(p))
	}
	me := hs.Players[itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = playerVM(me)
	}

	recent := hs.recentShots(3)
	recentVM := make([]map[string]any, 0, len(recent))
	for _, s := range recent {
		recentVM = append(recentVM, shotVM(s, hs))
	}

	var lastOwn, lastForeign map[string]any
	for i := len(hs.HuntShots) - 1; i >= 0; i-- {
		s := hs.HuntShots[i]
		if lastOwn == nil && s.RangeNum == rangeNum {
			lastOwn = shotVM(s, hs)
		}
		if lastForeign == nil && s.RangeNum != rangeNum {
			lastForeign = shotVM(s, hs)
		}
		if lastOwn != nil && lastForeign != nil {
			break
		}
	}

	role := "jäger"
	if hs.CurrentFox == rangeNum && (hs.Phase == PhaseOpening || hs.Phase == PhaseChase || hs.Phase == PhaseRoundResult) {
		role = "fuchs"
	}
	myTurn := hs.TurnRange == rangeNum && (hs.Phase == PhaseOpening || hs.Phase == PhaseChase)

	foxProg := 0.0
	if escape > 0 {
		foxProg = clampFloat(hs.Lead/escape, 0, 1)
	}
	packProg := 1.0 - foxProg

	var terrain any
	if hs.LastTerrain != nil && hs.LastTerrain.Amount > 0 {
		terrain = map[string]any{
			"id":     hs.LastTerrain.FlavorID,
			"label":  hs.LastTerrain.FlavorLabel,
			"kind":   hs.LastTerrain.Kind,
			"side":   hs.LastTerrain.Side,
			"amount": hs.LastTerrain.Amount,
		}
	}

	results := make([]map[string]any, 0, len(hs.RoundResults))
	for _, r := range hs.RoundResults {
		results = append(results, map[string]any{
			"foxRange": r.FoxRange, "outcome": r.Outcome, "finalLead": r.FinalLead,
			"foxChaseShots": r.FoxChaseShots, "openingSum": r.OpeningSum, "startBonus": r.StartBonus,
		})
	}

	return map[string]any{
		"pluginId": l.ID(),
		"kind":     "game",
		"mode":     "shared",
		"label":    l.Label(),
		"rangeNum": rangeNum,
		"hunt": map[string]any{
			"phase":              hs.Phase,
			"statusLine":         hs.StatusLine,
			"startBlockedReason": hs.StartBlockedReason,
			"currentFox":         hs.CurrentFox,
			"turnRange":          hs.TurnRange,
			"lead":               hs.Lead,
			"escapeTarget":       escape,
			"foxProgress":        foxProg,
			"packProgress":       packProg,
			"openingCount":       hs.OpeningCount,
			"openingShots":       cfgInt(hs.Config, "openingShots", 2),
			"openingSum":         hs.OpeningSum,
			"startBonus":         hs.StartBonus,
			"chaseShots":         hs.ChaseShots,
			"foxChaseShots":      hs.FoxChaseShots,
			"fieldAvg":           hs.FieldAvg,
			"foxOrder":           hs.FoxOrder,
			"foxIndex":           hs.FoxIndex,
			"players":            players,
			"recentShots":        recentVM,
			"terrain":            terrain,
			"roundResults":       results,
			"maxChaseShots":      cfgInt(hs.Config, "maxChaseShots", 40),
			"defaultTargetProfile": cfgString(hs.Config, "defaultTargetProfile", "air_rifle_10m"),
		},
		"me":       meVM,
		"myRole":   role,
		"myTurn":   myTurn,
		"lastOwn":  lastOwn,
		"lastForeign": lastForeign,
	}, nil
}

func playerVM(p *Player) map[string]any {
	return map[string]any{
		"rangeNum": p.RangeNum, "active": p.Active, "color": p.Color,
		"shooterName": p.ShooterName, "discipline": p.Discipline,
		"skill": p.Skill, "equalizer": p.Equalizer, "calibrated": p.Calibrated,
		"calCount": len(p.CalValues), "ready": p.Ready,
		"foxScore": p.FoxScore, "foxChaseShots": p.FoxChaseShots,
		"outcome": p.Outcome, "hadFoxTurn": p.HadFoxTurn,
	}
}

func shotVM(s ShotMark, hs *HuntState) map[string]any {
	color := ""
	if p := hs.Players[itoa(s.RangeNum)]; p != nil {
		color = p.Color
	}
	return map[string]any{
		"rangeNum": s.RangeNum, "role": s.Role, "raw": s.Raw, "delta": s.Delta,
		"leadAfter": s.LeadAfter, "x": s.X, "y": s.Y, "distance": s.Distance,
		"fullValue": s.FullValue, "color": color,
		"terrainId": s.TerrainID, "terrainLabel": s.TerrainLbl, "terrainAmount": s.TerrainAmt,
	}
}

func (hs *HuntState) pushHuntShot(m ShotMark) {
	hs.HuntShots = append(hs.HuntShots, m)
}

func (hs *HuntState) recentShots(n int) []ShotMark {
	if n <= 0 || len(hs.HuntShots) == 0 {
		return nil
	}
	if len(hs.HuntShots) <= n {
		out := make([]ShotMark, len(hs.HuntShots))
		copy(out, hs.HuntShots)
		return out
	}
	out := make([]ShotMark, n)
	copy(out, hs.HuntShots[len(hs.HuntShots)-n:])
	return out
}

func (hs *HuntState) beginMatch() []logicapi.PluginEvent {
	hs.FoxOrder = hs.activeRanges()
	hs.FoxIndex = 0
	hs.RoundResults = nil
	if len(hs.FoxOrder) == 0 {
		hs.StatusLine = "Keine aktiven Stände"
		return nil
	}
	hs.FieldAvg = computeEqualizers(hs.Players, hs.Config)
	return append([]logicapi.PluginEvent{{Type: "match_start"}}, hs.startFoxRound()...)
}

func (hs *HuntState) startFoxRound() []logicapi.PluginEvent {
	if hs.FoxIndex >= len(hs.FoxOrder) {
		hs.Phase = PhaseFinished
		hs.TurnRange = 0
		hs.StatusLine = "Jagd zu Ende — Waidmannsheil"
		return []logicapi.PluginEvent{{Type: "match_finished"}}
	}
	fox := hs.FoxOrder[hs.FoxIndex]
	hs.CurrentFox = fox
	hs.Phase = PhaseOpening
	hs.OpeningCount = 0
	hs.OpeningSum = 0
	hs.StartBonus = 0
	hs.Lead = 0
	hs.ChaseShots = 0
	hs.FoxChaseShots = 0
	hs.HuntShots = nil
	hs.LastTerrain = nil
	hs.ExpectFoxShot = false
	hs.HunterIdx = 0
	hs.TurnRange = fox
	hs.StartBlockedReason = ""
	if p := hs.Players[itoa(fox)]; p != nil {
		p.HadFoxTurn = true
		p.Outcome = OutcomeNone
		p.FoxChaseShots = 0
	}
	hs.StatusLine = fmt.Sprintf("Fuchs auf Stand %d — Vorwürfe", fox)
	return []logicapi.PluginEvent{
		{Type: "fox_round", Data: map[string]any{"fox": fox, "index": hs.FoxIndex}},
		{Type: "turn", Data: map[string]any{"rangeNum": fox}},
	}
}

func (hs *HuntState) finishHunt(outcome string) []logicapi.PluginEvent {
	fp := hs.Players[itoa(hs.CurrentFox)]
	finalLead := hs.Lead
	score := 0.0
	if outcome == OutcomeEscaped {
		score = finalLead
		hs.StatusLine = "Der Fuchs ist in den Bau"
	} else {
		finalLead = 0
		if hs.Lead < 0 {
			// keep raw for display in result but score 0
		}
		hs.StatusLine = "Der Fuchs ist gestellt"
	}
	if fp != nil {
		fp.Outcome = outcome
		fp.FoxScore = score
		fp.FoxChaseShots = hs.FoxChaseShots
	}
	res := RoundResult{
		FoxRange: hs.CurrentFox, Outcome: outcome, FinalLead: score,
		FoxChaseShots: hs.FoxChaseShots, OpeningSum: hs.OpeningSum, StartBonus: hs.StartBonus,
	}
	hs.RoundResults = append(hs.RoundResults, res)
	hs.Phase = PhaseRoundResult
	evType := "escaped"
	if outcome == OutcomeCaught {
		evType = "caught"
	}
	events := []logicapi.PluginEvent{{Type: evType, Data: map[string]any{
		"fox": hs.CurrentFox, "lead": hs.Lead, "score": score,
	}}}
	hs.FoxIndex++
	events = append(events, hs.startFoxRound()...)
	return events
}

func (hs *HuntState) setNextHunterTurn() {
	hunters := hs.hunters()
	if len(hunters) == 0 {
		hs.TurnRange = hs.CurrentFox
		return
	}
	if hs.HunterIdx >= len(hunters) {
		hs.HunterIdx = 0
	}
	hs.TurnRange = hunters[hs.HunterIdx]
	hs.HunterIdx++
}

func (hs *HuntState) hunters() []int {
	out := make([]int, 0, hs.NumRanges)
	for _, r := range hs.activeRanges() {
		if r != hs.CurrentFox {
			out = append(out, r)
		}
	}
	return out
}

func (hs *HuntState) activeRanges() []int {
	out := make([]int, 0, hs.NumRanges)
	for i := 1; i <= hs.NumRanges; i++ {
		p := hs.Players[itoa(i)]
		if p != nil && p.Active {
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}

func (hs *HuntState) allCalibrated() bool {
	n := 0
	for _, p := range hs.Players {
		if p == nil || !p.Active {
			continue
		}
		n++
		if !p.Calibrated {
			return false
		}
	}
	return n > 0
}

func (hs *HuntState) allReady() bool {
	n := 0
	for _, p := range hs.Players {
		if p == nil || !p.Active {
			continue
		}
		n++
		if !p.Ready {
			return false
		}
	}
	return n > 0
}

func (hs *HuntState) blockReason() string {
	missing := []int{}
	for _, r := range hs.activeRanges() {
		p := hs.Players[itoa(r)]
		if p == nil {
			continue
		}
		if !p.Calibrated || !p.Ready {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("Warte auf Stände %v", missing)
}

func (hs *HuntState) ensurePlayers() {
	if hs.Players == nil {
		hs.Players = map[string]*Player{}
	}
	if hs.NumRanges < 1 {
		hs.NumRanges = 6
	}
	for i := 1; i <= hs.NumRanges; i++ {
		k := itoa(i)
		if hs.Players[k] == nil {
			hs.Players[k] = &Player{
				RangeNum: i, Active: true, Color: laneColors[(i-1)%len(laneColors)],
				WasWarmup: true, CalValues: []float64{},
			}
		}
	}
}

func (hs *HuntState) applyLive(params map[string]any) {
	if n := cfgInt(params, "numRanges", 0); n > 0 {
		hs.NumRanges = n
		hs.ensurePlayers()
	}
	live, _ := params["live"].(map[string]any)
	if live == nil {
		return
	}
	for k, v := range live {
		rn, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		p := hs.Players[itoa(rn)]
		if p == nil {
			continue
		}
		if name, ok := m["shooterName"].(string); ok {
			p.ShooterName = name
		}
		if disc, ok := m["discipline"].(string); ok {
			p.Discipline = disc
		}
		warmup, _ := m["isWarmup"].(bool)
		if p.WasWarmup && !warmup {
			p.WasWarmup = false
			p.Ready = true
		}
		if warmup {
			p.WasWarmup = true
		}
		if active, ok := m["active"].(bool); ok {
			p.Active = active
		}
	}
}

func marshalState(hs *HuntState) (logicapi.SessionState, error) {
	b, err := json.Marshal(hs)
	if err != nil {
		return nil, err
	}
	return logicapi.SessionState(b), nil
}

func unmarshalState(sess logicapi.SessionState) (*HuntState, error) {
	var hs HuntState
	if len(sess) == 0 {
		return &HuntState{Players: map[string]*Player{}, Config: defaultConfig()}, nil
	}
	if err := json.Unmarshal(sess, &hs); err != nil {
		return nil, err
	}
	if hs.Config == nil {
		hs.Config = defaultConfig()
	}
	if hs.Players == nil {
		hs.Players = map[string]*Player{}
	}
	return &hs, nil
}

func marshalWithEvents(hs *HuntState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := marshalState(hs)
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

func cfgFloat(m map[string]any, key string, def float64) float64 {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	return toFloat(v, def)
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

func toFloat(v any, def float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err == nil {
			return f
		}
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func itoa(i int) string { return strconv.Itoa(i) }
