package f1race

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

func init() {
	loader.RegisterBuiltin("f1-race", func(m *loader.Manifest) logicapi.Logic {
		return New(m)
	})
}

const (
	PhaseWarmup   = "warmup_collect"
	PhaseArming   = "arming"
	PhaseRacing   = "racing"
	PhaseFinished = "finished"

	StatusActive   = "active"
	StatusReady    = "ready"
	StatusRacing   = "racing"
	StatusFinished = "finished"
	StatusCrashed  = "crashed"

	MotionPush   = "push"
	MotionCruise = "cruise"
)

type Logic struct {
	manifest *loader.Manifest
}

func New(m *loader.Manifest) *Logic {
	return &Logic{manifest: m}
}

func (l *Logic) ID() string {
	if l.manifest != nil {
		return l.manifest.ID
	}
	return "f1-race"
}
func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "F1 Race"
}
func (l *Logic) Version() string {
	if l.manifest != nil && l.manifest.Version != "" {
		return l.manifest.Version
	}
	return "1.0.0"
}

func (l *Logic) DefaultConfig() map[string]any {
	return defaultConfig()
}

func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"circuitId":                map[string]any{"type": "string", "enum": []string{"spa", "nuerburgring", "melbourne", "nordschleife"}},
			"motionMode":               map[string]any{"type": "string", "enum": []string{"push", "cruise"}},
			"stintSize":                map[string]any{"type": "integer"},
			"roundDurationSec":         map[string]any{"type": "integer"},
			"skippedRoundsToCrash":     map[string]any{"type": "integer"},
			"overtakeRatio":            map[string]any{"type": "number"},
			"paceCompress":             map[string]any{"type": "number"},
			"pacePivot":                map[string]any{"type": "number"},
			"drsStackPerPlace":         map[string]any{"type": "number"},
			"drsSections":              map[string]any{"type": "string"},
			"gridGap":                  map[string]any{"type": "number"},
			"trackLength":              map[string]any{"type": "number"},
			"highShotThreshold":        map[string]any{"type": "number"},
			"streakBonus":              map[string]any{"type": "number"},
			"pitCueWindowMs":           map[string]any{"type": "integer"},
			"pitScoreWeight":           map[string]any{"type": "number"},
			"pitReactionWeight":        map[string]any{"type": "number"},
			"autoStartWhenAllReady":    map[string]any{"type": "boolean"},
			"requireEqualShotTotals":   map[string]any{"type": "boolean"},
			"fieldEventsEnabled":       map[string]any{"type": "boolean"},
			"fieldEventMinGapSec":      map[string]any{"type": "integer"},
			"fieldEventChancePerRound": map[string]any{"type": "number"},
			"holeInHoleMinOverlap":     map[string]any{"type": "number"},
			"holeInHoleBonus":          map[string]any{"type": "number"},
			"shotDiameterMm":           map[string]any{"type": "number"},
			"dsgPerMm":                 map[string]any{"type": "number"},
			"handicaps":                map[string]any{"type": "object"},
		},
	}
}

// Sweet spot A defaults: softer non-DRS overtake, compressed pace toward 8.0,
// wide DRS sections, and position-scaled DRS chase aid.
func defaultConfig() map[string]any {
	return map[string]any{
		"circuitId":              "spa",
		"motionMode":             MotionPush,
		"stintSize":              10,
		"roundDurationSec":       120,
		"skippedRoundsToCrash":   2,
		"overtakeRatio":          1.12,
		"paceCompress":           0.50,
		"pacePivot":              8.0,
		"drsStackPerPlace":       0.12,
		"drsSections":            "2,3,4,7,8",
		"gridGap":                0.08,
		"trackLength":            1.0,
		"highShotThreshold":      9.0,
		"streakBonus":            1.15,
		"pitCueWindowMs":         5000,
		"pitScoreWeight":         1.0,
		"pitReactionWeight":      0.0004,
		"autoStartWhenAllReady":  true,
		"requireEqualShotTotals": true,
		"fieldEventsEnabled":     true,
		"fieldEventMinGapSec":    90,
		"fieldEventChancePerRound": 0.08,
		"holeInHoleMinOverlap":   0.5,
		"holeInHoleBonus":        0.05,
		"shotDiameterMm":         4.5,
		"dsgPerMm":               100.0,
		"handicaps":              map[string]any{},
	}
}

type DRSZone struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type FieldEvent struct {
	Type     string       `json:"type"`
	CueAt    time.Time    `json:"cueAt"`
	WindowMs int          `json:"windowMs"`
	Targets  map[int]bool `json:"targets"` // cars that must take this pit
	Cleared  map[int]bool `json:"cleared"`
}

type CarState struct {
	RangeNum           int     `json:"rangeNum"`
	Status             string  `json:"status"`
	Ready              bool    `json:"ready"`
	Progress           float64 `json:"progress"`
	LastSpeed          float64 `json:"lastSpeed"`
	ShotsFired         int     `json:"shotsFired"`
	SkippedConsecutive int     `json:"skippedConsecutive"`
	ShotThisRound      bool    `json:"shotThisRound"`
	HighStreak         int     `json:"highStreak"`
	StreakBoostUntil   int     `json:"streakBoostUntil"`
	Handicap           float64 `json:"handicap"`
	Position           int     `json:"position"`
	ShooterName        string  `json:"shooterName"`
	TotalShots         int     `json:"totalShots"`
	Discipline         string  `json:"discipline"`
	WasWarmup          bool    `json:"wasWarmup"`
	LastShotX          int     `json:"lastShotX"`
	LastShotY          int     `json:"lastShotY"`
	LastShotValue      float64 `json:"lastShotValue"`
	HasLastShot        bool    `json:"hasLastShot"`
	Color              string  `json:"color"`
	PendingPit         bool    `json:"pendingPit"`
	PitKind            string  `json:"pitKind"` // stint | field
	LastNote           string  `json:"lastNote"`
	LastNoteKind       string  `json:"lastNoteKind"`
	PlaceReason        string  `json:"placeReason"`
	PlaceReasonKind    string  `json:"placeReasonKind"`
	LastBoostKind      string  `json:"lastBoostKind"` // miss|streak|hole_in_hole|drs|pit|""
	NextHint           string  `json:"nextHint"`
	NextHintKind       string  `json:"nextHintKind"`
}

type RaceState struct {
	Phase              string              `json:"phase"`
	CircuitID          string              `json:"circuitId"`
	MotionMode         string              `json:"motionMode"`
	CurrentRound       int                 `json:"currentRound"`
	RoundOpenedAt      *time.Time          `json:"roundOpenedAt,omitempty"`
	RoundDurationSec   int                 `json:"roundDurationSec"`
	ShotTotal          int                 `json:"shotTotal"`
	StartBlockedReason string              `json:"startBlockedReason,omitempty"`
	GridSet            bool                `json:"gridSet"`
	Cars               map[string]*CarState `json:"cars"`
	ActiveFieldEvent   *FieldEvent         `json:"activeFieldEvent,omitempty"`
	LastFieldEventAt   *time.Time          `json:"lastFieldEventAt,omitempty"`
	DRSZones           []DRSZone           `json:"drsZones"`
	Config             map[string]any      `json:"config"`
	NumRanges          int                 `json:"numRanges"`
	PitCueAt           *time.Time          `json:"pitCueAt,omitempty"`
	StintPitRound      int                 `json:"stintPitRound"`
}

var defaultColors = []string{
	"#e10600", "#1e5bc6", "#00d2be", "#ff8700",
	"#f596c8", "#239971", "#ffffff", "#5e8faa",
	"#b6babd", "#52e252",
}

var circuitDRS = map[string][]DRSZone{
	"spa":           {{Start: 0.72, End: 0.88}, {Start: 0.18, End: 0.28}},
	"nuerburgring":  {{Start: 0.65, End: 0.82}, {Start: 0.30, End: 0.40}},
	"melbourne":     {{Start: 0.70, End: 0.85}, {Start: 0.10, End: 0.22}},
	"nordschleife":  {{Start: 0.82, End: 0.96}, {Start: 0.08, End: 0.18}}, // Döttinger Höhe + Flugplatz
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	merged := defaultConfig()
	for k, v := range cfg {
		merged[k] = v
	}
	numRanges := cfgInt(merged, "numRanges", 6)
	circuit := cfgString(merged, "circuitId", "spa")
	stint := cfgInt(merged, "stintSize", 10)
	if stint < 2 {
		stint = 10
	}
	rs := &RaceState{
		Phase:            PhaseWarmup,
		CircuitID:        circuit,
		MotionMode:       cfgString(merged, "motionMode", MotionPush),
		CurrentRound:     1,
		RoundDurationSec: cfgInt(merged, "roundDurationSec", 120),
		Cars:             map[string]*CarState{},
		DRSZones:         resolveDRSZones(merged, circuit, stint),
		Config:           merged,
		NumRanges:        numRanges,
	}
	handicaps := map[string]any{}
	if h, ok := merged["handicaps"].(map[string]any); ok {
		handicaps = h
	}
	for i := 1; i <= numRanges; i++ {
		hc := 1.0
		if v, ok := handicaps[strconv.Itoa(i)]; ok {
			hc = toFloat(v, 1.0)
		}
		color := defaultColors[(i-1)%len(defaultColors)]
		rs.Cars[strconv.Itoa(i)] = &CarState{
			RangeNum:  i,
			Status:    StatusActive,
			Handicap:  hc,
			Color:     color,
			WasWarmup: true,
		}
	}
	rs.formStartingGrid()
	return marshalState(rs)
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
	rs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	var events []logicapi.PluginEvent
	car := rs.Cars[strconv.Itoa(ctx.RangeNum)]
	if car == nil {
		return sess, nil, nil
	}
	if ctx.Live.ShooterName != "" {
		car.ShooterName = ctx.Live.ShooterName
	}
	if ctx.Live.Discipline != "" {
		car.Discipline = ctx.Live.Discipline
	}
	if ctx.Live.TotalShotsToFire > 0 {
		car.TotalShots = ctx.Live.TotalShotsToFire
	}

	// Warmup tracking / ready
	if ctx.Live.IsWarmup || ctx.Shot.IsWarmup {
		car.WasWarmup = true
		return marshalWithEvents(rs, nil)
	}
	if car.WasWarmup && !ctx.Live.IsWarmup {
		car.WasWarmup = false
		car.Ready = true
		car.Status = StatusReady
		events = append(events, logicapi.PluginEvent{Type: "ready", Data: map[string]any{"rangeNum": ctx.RangeNum}})
		if rs.Phase == PhaseWarmup {
			rs.Phase = PhaseArming
		}
		if auto, reason := rs.canAutoStart(); auto {
			evs := rs.forceStart(ctx.Now)
			events = append(events, evs...)
			_ = reason
		} else if reason != "" {
			rs.StartBlockedReason = reason
		}
	}

	if rs.Phase != PhaseRacing {
		return marshalWithEvents(rs, events)
	}
	if car.Status == StatusCrashed || car.Status == StatusFinished {
		return marshalWithEvents(rs, events)
	}

	// Stint pit (every 10th). Field events convert the next budgeted shot into
	// a pit — never an extra shot, because the range hard-stops at TotalShots.
	stintSize := cfgInt(rs.Config, "stintSize", 10)
	nextShotNum := car.ShotsFired + 1
	isStintPit := stintSize > 0 && nextShotNum%stintSize == 0
	fieldPit := rs.ActiveFieldEvent != nil &&
		rs.ActiveFieldEvent.Targets[ctx.RangeNum] &&
		!rs.ActiveFieldEvent.Cleared[ctx.RangeNum]

	if !rs.GridSet {
		evs := rs.applyGridShot(car, ctx)
		events = append(events, evs...)
		return marshalWithEvents(rs, events)
	}

	// Round membership: lagging cars may catch up, but must not yank the
	// whole field back to an earlier section marker.
	expected := car.ShotsFired + 1
	snapTo := expected
	if rs.GridSet && expected < rs.CurrentRound {
		// Late catch-up shot — park the field at the current shared round.
		snapTo = rs.CurrentRound
	}

	if rs.RoundOpenedAt == nil {
		t := ctx.Now
		rs.RoundOpenedAt = &t
		// Prefer the shared pit cue armed when the round opened for everyone.
		// Only fall back to cue-on-first-shot if arming was missed.
		if isStintPit && !fieldPit && rs.PitCueAt == nil {
			events = append(events, rs.armPitRound(t)...)
		}
	}

	// Mark before applying pace so trySectionPass / resolveRoundPasses see this
	// round's result instead of comparing against a stale LastSpeed.
	car.ShotThisRound = true
	car.SkippedConsecutive = 0

	if fieldPit {
		// puncture → one car; oil_leak → all racing cars. Counts as this round's shot.
		evs := rs.applyPitShot(car, ctx, true)
		events = append(events, evs...)
		rs.ActiveFieldEvent.Cleared[ctx.RangeNum] = true
		car.PendingPit = false
		if rs.allFieldCleared() {
			rs.ActiveFieldEvent = nil
			if !rs.isPitRound(rs.CurrentRound) {
				rs.PitCueAt = nil
			}
		}
	} else if isStintPit {
		evs := rs.applyPitShot(car, ctx, false)
		events = append(events, evs...)
	} else {
		evs := rs.applyPowerShot(car, ctx)
		events = append(events, evs...)
	}

	car.ShotsFired++
	// Re-resolve section passes now that this car's pace is known, so a
	// trailing car that already shot can still overtake once the car ahead fires.
	section := rs.sectionOfShot(snapTo)
	rs.resolveRoundPasses(section)
	rs.snapFieldToSection(snapTo)
	if car.TotalShots > 0 && car.ShotsFired >= car.TotalShots {
		car.Status = StatusFinished
		events = append(events, logicapi.PluginEvent{Type: "finished", Data: map[string]any{"rangeNum": car.RangeNum}})
	}

	rs.recomputePositions()
	rs.refreshPlaceReasons()
	events = append(events, rs.maybeAdvanceRound(ctx.Now)...)
	rs.maybeFinish()
	rs.maybeRandomFieldEvent(ctx.Now, &events)

	return marshalWithEvents(rs, events)
}

func (l *Logic) Tick(sess logicapi.SessionState, now time.Time) (logicapi.SessionState, []logicapi.PluginEvent, bool, error) {
	rs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, false, err
	}
	if rs.Phase != PhaseRacing {
		return sess, nil, false, nil
	}
	var events []logicapi.PluginEvent
	changed := false

	// Field event timeout — only targeted cars are penalized.
	if rs.ActiveFieldEvent != nil {
		win := time.Duration(rs.ActiveFieldEvent.WindowMs) * time.Millisecond
		if now.After(rs.ActiveFieldEvent.CueAt.Add(win)) {
			for rangeNum := range rs.ActiveFieldEvent.Targets {
				car := rs.Cars[strconv.Itoa(rangeNum)]
				if car == nil || car.Status == StatusCrashed || car.Status == StatusFinished {
					continue
				}
				if !rs.ActiveFieldEvent.Cleared[rangeNum] {
					car.Progress = math.Max(0, car.Progress-0.03)
					car.SkippedConsecutive++
					setCarNote(car, "pit_fail", fmt.Sprintf("Feld-Event verpasst (%s)", fieldTypeLabel(rs.ActiveFieldEvent.Type)))
					if car.SkippedConsecutive >= cfgInt(rs.Config, "skippedRoundsToCrash", 2) {
						car.Status = StatusCrashed
						events = append(events, logicapi.PluginEvent{Type: "crash", Data: map[string]any{"rangeNum": car.RangeNum, "reason": "field_event"}})
					}
					changed = true
				}
			}
			rs.ActiveFieldEvent = nil
			// Keep stint pit cue if this is still a pit round; only clear the
			// field-event overlay. Clearing PitCueAt here made late pit shots
			// look like instant reactions (cue = now).
			if !rs.isPitRound(rs.CurrentRound) {
				rs.PitCueAt = nil
			}
			changed = true
		}
	}

	if rs.ActiveFieldEvent == nil && rs.RoundOpenedAt != nil {
		deadline := rs.RoundOpenedAt.Add(time.Duration(rs.RoundDurationSec) * time.Second)
		if now.After(deadline) || now.Equal(deadline) {
			for _, car := range rs.Cars {
				if car.Status == StatusCrashed || car.Status == StatusFinished {
					continue
				}
				if car.Status != StatusRacing {
					continue
				}
				if !car.ShotThisRound {
					car.SkippedConsecutive++
					changed = true
					events = append(events, logicapi.PluginEvent{Type: "round_skip", Data: map[string]any{
						"rangeNum": car.RangeNum, "round": rs.CurrentRound, "skipped": car.SkippedConsecutive,
					}})
					if car.SkippedConsecutive >= cfgInt(rs.Config, "skippedRoundsToCrash", 2) {
						car.Status = StatusCrashed
						events = append(events, logicapi.PluginEvent{Type: "crash", Data: map[string]any{"rangeNum": car.RangeNum, "reason": "skipped_rounds"}})
					}
				}
			}
			// advance round
			rs.CurrentRound++
			rs.RoundOpenedAt = nil
			rs.PitCueAt = nil
			for _, car := range rs.Cars {
				car.ShotThisRound = false
			}
			changed = true
			events = append(events, logicapi.PluginEvent{Type: "round_closed", Data: map[string]any{"nextRound": rs.CurrentRound}})
			events = append(events, rs.armPitRoundIfNeeded(now)...)
			rs.maybeFinish()
		}
	}

	if !changed {
		return sess, nil, false, nil
	}
	rs.recomputePositions()
	rs.refreshPlaceReasons()
	out, err := marshalState(rs)
	return out, events, true, err
}

func (l *Logic) Control(sess logicapi.SessionState, action string, params map[string]any) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	rs, err := unmarshalState(sess)
	if err != nil {
		return sess, nil, err
	}
	now := time.Now()
	if s, ok := params["now"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			now = t
		}
	}
	var events []logicapi.PluginEvent

	switch action {
	case "sync_live":
		rs.applyLive(params)
		if rs.Phase == PhaseWarmup || rs.Phase == PhaseArming {
			if auto, reason := rs.canAutoStart(); auto {
				events = append(events, rs.forceStart(now)...)
			} else {
				rs.StartBlockedReason = reason
			}
		}
	case "start":
		rs.applyLive(params)
		if ok, reason := rs.startGate(); !ok {
			rs.StartBlockedReason = reason
			return marshalWithEvents(rs, nil)
		}
		events = append(events, rs.forceStart(now)...)
	case "reset":
		num := rs.NumRanges
		cfg := rs.Config
		cfg["numRanges"] = num
		fresh, err := l.Init(cfg)
		if err != nil {
			return sess, nil, err
		}
		return fresh, []logicapi.PluginEvent{{Type: "reset"}}, nil
	case "field_event":
		if rs.Phase != PhaseRacing {
			return sess, nil, fmt.Errorf("race not running")
		}
		typ := "puncture"
		if t, ok := params["type"].(string); ok && t != "" {
			typ = t
		}
		target := 0
		if v, ok := params["rangeNum"]; ok {
			target = int(toFloat(v, 0))
		}
		events = append(events, rs.openFieldEvent(typ, now, target)...)
	default:
		return sess, nil, fmt.Errorf("unknown action %q", action)
	}
	return marshalWithEvents(rs, events)
}

func (l *Logic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	rs, err := unmarshalState(sess)
	if err != nil {
		return nil, err
	}
	rs.syncCarsToNumRanges()
	rs.refreshPlaceReasons()
	cars := make([]map[string]any, 0, rs.NumRanges)
	for i := 1; i <= rs.NumRanges; i++ {
		c := rs.Cars[strconv.Itoa(i)]
		if c == nil {
			continue
		}
		cars = append(cars, carVM(c))
	}
	me := rs.Cars[strconv.Itoa(rangeNum)]
	var meVM map[string]any
	if me != nil {
		meVM = carVM(me)
	}
	var roundEndsAt any
	var roundRemainingSec any
	if rs.RoundOpenedAt != nil {
		end := rs.RoundOpenedAt.Add(time.Duration(rs.RoundDurationSec) * time.Second)
		roundEndsAt = end.UTC().Format(time.RFC3339Nano)
		roundRemainingSec = math.Max(0, end.Sub(time.Now()).Seconds())
	}
	var field any
	if rs.ActiveFieldEvent != nil {
		targets := make([]int, 0, len(rs.ActiveFieldEvent.Targets))
		for rn := range rs.ActiveFieldEvent.Targets {
			targets = append(targets, rn)
		}
		sort.Ints(targets)
		affectsMe := rs.ActiveFieldEvent.Targets[rangeNum]
		field = map[string]any{
			"type":      rs.ActiveFieldEvent.Type,
			"cueAt":     rs.ActiveFieldEvent.CueAt.UTC().Format(time.RFC3339Nano),
			"windowMs":  rs.ActiveFieldEvent.WindowMs,
			"cleared":   rs.ActiveFieldEvent.Cleared[rangeNum],
			"affectsMe": affectsMe,
			"targets":   targets,
			"pending":   affectsMe && !rs.ActiveFieldEvent.Cleared[rangeNum],
		}
	}
	var pitCue any
	var pitRemainingSec any
	pitWindowMs := cfgInt(rs.Config, "pitCueWindowMs", 5000)
	if rs.PitCueAt != nil {
		pitCue = rs.PitCueAt.UTC().Format(time.RFC3339Nano)
		end := rs.PitCueAt.Add(time.Duration(pitWindowMs) * time.Millisecond)
		pitRemainingSec = math.Max(0, end.Sub(time.Now()).Seconds())
	}
	return map[string]any{
		"pluginId": l.ID(),
		"kind":     "game",
		"mode":     "shared",
		"label":    l.Label(),
		"rangeNum": rangeNum,
		"race": map[string]any{
			"phase":              rs.Phase,
			"circuitId":          rs.CircuitID,
			"motionMode":         rs.MotionMode,
			"currentRound":       rs.CurrentRound,
			"shotTotal":          rs.ShotTotal,
			"startBlockedReason": rs.StartBlockedReason,
			"gridSet":            rs.GridSet,
			"cars":               cars,
			"drsZones":           rs.DRSZones,
			"roundEndsAt":        roundEndsAt,
			"roundRemainingSec":  roundRemainingSec,
			"fieldEvent":         field,
			"pitCueAt":           pitCue,
			"pitWindowMs":        pitWindowMs,
			"pitRemainingSec":    pitRemainingSec,
			"stintPitRound":      rs.StintPitRound,
			"isPitRound":         rs.isPitRound(rs.CurrentRound),
			"currentSection":     rs.sectionOfShot(rs.CurrentRound),
			"sectionsPerLap":     rs.stintSize(),
			"powerSections":      rs.stintSize() - 1,
		},
		"me": meVM,
	}, nil
}

func carVM(c *CarState) map[string]any {
	return map[string]any{
		"rangeNum":           c.RangeNum,
		"status":             c.Status,
		"ready":              c.Ready,
		"progress":           c.Progress,
		"lastSpeed":          c.LastSpeed,
		"lastShotValue":      c.LastShotValue,
		"shotsFired":         c.ShotsFired,
		"skippedConsecutive": c.SkippedConsecutive,
		"shotThisRound":      c.ShotThisRound,
		"highStreak":         c.HighStreak,
		"position":           c.Position,
		"shooterName":        c.ShooterName,
		"totalShots":         c.TotalShots,
		"discipline":         c.Discipline,
		"color":              c.Color,
		"handicap":           c.Handicap,
		"pendingPit":         c.PendingPit,
		"pitKind":            c.PitKind,
		"lastNote":           c.LastNote,
		"lastNoteKind":       c.LastNoteKind,
		"placeReason":        c.PlaceReason,
		"placeReasonKind":    c.PlaceReasonKind,
		"lastBoostKind":      c.LastBoostKind,
		"nextHint":           c.NextHint,
		"nextHintKind":       c.NextHintKind,
	}
}

func (rs *RaceState) applyLive(params map[string]any) {
	live, _ := params["live"].(map[string]any)
	if live == nil {
		return
	}
	for k, v := range live {
		car := rs.Cars[k]
		if car == nil {
			continue
		}
		m, _ := v.(map[string]any)
		if m == nil {
			continue
		}
		if n := cfgInt(m, "totalShotsToFire", 0); n > 0 {
			car.TotalShots = n
		}
		if d, ok := m["discipline"].(string); ok {
			car.Discipline = d
		}
		if name, ok := m["shooterName"].(string); ok {
			car.ShooterName = name
		}
		warmup, _ := m["isWarmup"].(bool)
		if car.WasWarmup && !warmup {
			car.WasWarmup = false
			car.Ready = true
			if car.Status == StatusActive {
				car.Status = StatusReady
			}
			if rs.Phase == PhaseWarmup {
				rs.Phase = PhaseArming
			}
		}
		if warmup {
			car.WasWarmup = true
		}
	}
	if n := cfgInt(params, "numRanges", 0); n > 0 {
		rs.NumRanges = n
		rs.syncCarsToNumRanges()
	}
}

// syncCarsToNumRanges adds missing cars and drops extras so the field
// always matches the configured lane count (avoids leftover cars 7–12
// after shrinking from a larger setup).
func (rs *RaceState) syncCarsToNumRanges() {
	if rs.NumRanges < 1 {
		rs.NumRanges = 1
	}
	if rs.Cars == nil {
		rs.Cars = map[string]*CarState{}
	}
	handicaps := map[string]any{}
	if rs.Config != nil {
		if h, ok := rs.Config["handicaps"].(map[string]any); ok {
			handicaps = h
		}
	}
	for i := 1; i <= rs.NumRanges; i++ {
		k := strconv.Itoa(i)
		if rs.Cars[k] != nil {
			continue
		}
		hc := 1.0
		if v, ok := handicaps[k]; ok {
			hc = toFloat(v, 1.0)
		}
		rs.Cars[k] = &CarState{
			RangeNum:  i,
			Status:    StatusActive,
			Handicap:  hc,
			Color:     defaultColors[(i-1)%len(defaultColors)],
			WasWarmup: true,
		}
	}
	for k, c := range rs.Cars {
		if c == nil || c.RangeNum < 1 || c.RangeNum > rs.NumRanges {
			delete(rs.Cars, k)
		}
	}
	// Keep a visible single-file formation whenever everyone is still bunched
	// (warmup / before the first scoring shot).
	allParked := true
	for _, c := range rs.Cars {
		if c.Status == StatusCrashed {
			continue
		}
		if c.Progress > 0.001 || c.ShotsFired > 0 {
			allParked = false
			break
		}
	}
	if allParked {
		rs.formStartingGrid()
	}
}

func (rs *RaceState) startGate() (bool, string) {
	requireEqual := cfgBool(rs.Config, "requireEqualShotTotals", true)
	var total int
	first := true
	readyCount := 0
	for i := 1; i <= rs.NumRanges; i++ {
		car := rs.Cars[strconv.Itoa(i)]
		if car == nil {
			continue
		}
		if car.TotalShots <= 0 {
			return false, fmt.Sprintf("Bahn %d: Disziplin/Schusszahl fehlt (OpticScore Programm setzen)", i)
		}
		if first {
			total = car.TotalShots
			first = false
		} else if requireEqual && car.TotalShots != total {
			return false, fmt.Sprintf("Unterschiedliche Schusszahlen (Bahn %d: %d vs %d)", i, car.TotalShots, total)
		}
		if car.Ready {
			readyCount++
		}
	}
	if first {
		return false, "Keine Bahnen konfiguriert"
	}
	rs.ShotTotal = total
	return true, ""
}

func (rs *RaceState) canAutoStart() (bool, string) {
	if !cfgBool(rs.Config, "autoStartWhenAllReady", true) {
		return false, ""
	}
	ok, reason := rs.startGate()
	if !ok {
		return false, reason
	}
	for i := 1; i <= rs.NumRanges; i++ {
		car := rs.Cars[strconv.Itoa(i)]
		if car == nil {
			continue
		}
		if !car.Ready {
			return false, "Warte auf alle Bahnen (Probe beenden)"
		}
	}
	return true, ""
}

func (rs *RaceState) forceStart(now time.Time) []logicapi.PluginEvent {
	rs.Phase = PhaseRacing
	rs.CurrentRound = 1
	rs.RoundOpenedAt = nil
	rs.GridSet = false
	rs.StartBlockedReason = ""
	ok, _ := rs.startGate()
	_ = ok
	for _, car := range rs.Cars {
		if car.Status == StatusCrashed {
			continue
		}
		car.Status = StatusRacing
		car.ShotsFired = 0
		car.SkippedConsecutive = 0
		car.ShotThisRound = false
		car.HighStreak = 0
	}
	rs.formStartingGrid()
	rs.refreshPlaceReasons()
	return []logicapi.PluginEvent{{Type: "race_start", Data: map[string]any{"at": now.UTC().Format(time.RFC3339Nano), "shotTotal": rs.ShotTotal}}}
}

func (rs *RaceState) applyGridShot(car *CarState, ctx logicapi.ShotContext) []logicapi.PluginEvent {
	var events []logicapi.PluginEvent
	if isZeroShot(ctx.Shot) {
		car.LastSpeed = 0
		car.LastBoostKind = "miss"
		setCarNote(car, "miss", "Nullschuss — kein Tempo")
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{"rangeNum": car.RangeNum, "grid": true}})
	} else {
		score := rs.compressPace(effectiveScore(ctx.Shot, car.Handicap))
		car.LastSpeed = score
		car.LastBoostKind = ""
	}
	car.ShotThisRound = true
	car.ShotsFired = 1
	car.SkippedConsecutive = 0
	car.HasLastShot = true
	car.LastShotX = ctx.Shot.X
	car.LastShotY = ctx.Shot.Y
	storeShotCoords(car, ctx.Shot)

	// When all racing cars have fired grid shot, set positions
	all := true
	type scored struct {
		car   *CarState
		score float64
	}
	var list []scored
	for _, c := range rs.Cars {
		if c.Status != StatusRacing {
			continue
		}
		if c.ShotsFired < 1 {
			all = false
		} else {
			list = append(list, scored{c, c.LastSpeed})
		}
	}
	if all && len(list) > 0 {
		sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
		for i, s := range list {
			s.car.Position = i + 1
		}
		rs.snapFieldToSection(1)
		rs.GridSet = true
		rs.CurrentRound = 2
		rs.RoundOpenedAt = nil
		for _, c := range rs.Cars {
			c.ShotThisRound = false
		}
		rs.refreshPlaceReasons()
		events = append(events, logicapi.PluginEvent{Type: "grid_set"})
	}
	return events
}

type overtakeResult struct {
	inDRS      bool
	passed     bool
	usedDRS    bool
	otherRange int
	otherPace  float64
}

func (rs *RaceState) applyPowerShot(car *CarState, ctx logicapi.ShotContext) []logicapi.PluginEvent {
	var events []logicapi.PluginEvent
	shotNum := car.ShotsFired + 1
	section := rs.sectionOfShot(shotNum)

	// Zero: pace 0 for this shot only — still counts as the shared round.
	if isZeroShot(ctx.Shot) {
		car.LastSpeed = 0
		car.HighStreak = 0
		car.LastBoostKind = "miss"
		setCarNote(car, "miss", "Nullschuss — kein Tempo")
		storeShotCoords(car, ctx.Shot)
		events = append(events, logicapi.PluginEvent{Type: "miss", Data: map[string]any{
			"rangeNum": car.RangeNum, "section": section,
		}})
		return events
	}

	score := rs.compressPace(effectiveScore(ctx.Shot, car.Handicap))
	boost := 1.0
	car.LastBoostKind = ""
	if car.StreakBoostUntil >= car.ShotsFired {
		boost = cfgFloat(rs.Config, "streakBonus", 1.15)
		car.LastBoostKind = "streak"
	}
	speed := score * boost
	inDRS := rs.sectionInDRS(section)

	// streak
	thresh := cfgFloat(rs.Config, "highShotThreshold", 9.0)
	if ctx.Shot.DecValue >= thresh || float64(ctx.Shot.FullValue) >= thresh {
		car.HighStreak++
		if car.HighStreak >= 3 {
			car.StreakBoostUntil = car.ShotsFired + 3
			events = append(events, logicapi.PluginEvent{Type: "streak_bonus", Data: map[string]any{"rangeNum": car.RangeNum}})
			car.HighStreak = 0
		}
	} else {
		car.HighStreak = 0
	}

	// hole-in-hole: temporary pace boost for this section's pass check
	if (ctx.Shot.FullValue == 9 || ctx.Shot.FullValue == 10) && car.HasLastShot {
		dsgPerMm := cfgFloat(rs.Config, "dsgPerMm", 100)
		pelletR := cfgFloat(rs.Config, "shotDiameterMm", 4.5) / 2 * dsgPerMm
		overlap := circleOverlapRatio(
			float64(car.LastShotX), float64(car.LastShotY),
			float64(ctx.Shot.X), float64(ctx.Shot.Y),
			pelletR,
		)
		minO := cfgFloat(rs.Config, "holeInHoleMinOverlap", 0.5)
		if overlap > minO {
			speed *= 1.0 + cfgFloat(rs.Config, "holeInHoleBonus", 0.05)
			car.LastBoostKind = "hole_in_hole"
			events = append(events, logicapi.PluginEvent{Type: "hole_in_hole", Data: map[string]any{
				"rangeNum": car.RangeNum, "overlap": overlap,
			}})
		}
	}
	storeShotCoords(car, ctx.Shot)
	car.LastSpeed = speed

	ot := rs.trySectionPass(car, speed, section)

	if inDRS {
		if ot.usedDRS {
			car.LastBoostKind = "drs"
		}
		events = append(events, logicapi.PluginEvent{Type: "drs_active", Data: map[string]any{
			"rangeNum": car.RangeNum,
			"section":  section,
			"passed":   ot.usedDRS,
		}})
	}
	if ot.passed {
		events = append(events, logicapi.PluginEvent{Type: "overtake", Data: map[string]any{
			"rangeNum":   car.RangeNum,
			"section":    section,
			"viaDRS":     ot.usedDRS,
			"otherRange": ot.otherRange,
			"myPace":     speed,
			"otherPace":  ot.otherPace,
		}})
	}
	return events
}

func (rs *RaceState) stintSize() int {
	n := cfgInt(rs.Config, "stintSize", 10)
	if n < 2 {
		return 10
	}
	return n
}

// sectionOfShot maps shot number → 1..stintSize (last section is the pit).
func (rs *RaceState) sectionOfShot(shotNum int) int {
	if shotNum < 1 {
		shotNum = 1
	}
	stint := rs.stintSize()
	return ((shotNum - 1) % stint) + 1
}

func (rs *RaceState) sectionInDRS(section int) bool {
	if sections := cfgIntList(rs.Config, "drsSections"); len(sections) > 0 {
		for _, s := range sections {
			if s == section {
				return true
			}
		}
		return false
	}
	stint := rs.stintSize()
	mid := (float64(section) - 0.5) / float64(stint)
	return inDRSZone(rs.DRSZones, mid)
}

func (rs *RaceState) carByPosition(pos int) *CarState {
	for _, c := range rs.Cars {
		if c == nil || c.Status == StatusCrashed {
			continue
		}
		if c.Status != StatusRacing && c.Status != StatusFinished {
			continue
		}
		if c.Position == pos {
			return c
		}
	}
	return nil
}

// trySectionPass compares this section's pace to the car ahead in race order.
// On success the positions swap; geographic progress is applied by snapFieldToSection.
// Only compares against cars that have already shot this round — otherwise LastSpeed
// is from the previous round and shot order would decide places.
// In DRS, chase pace is boosted by 1+min(placesBehind,3)*drsStackPerPlace.
func (rs *RaceState) trySectionPass(car *CarState, speed float64, section int) overtakeResult {
	res := overtakeResult{inDRS: rs.sectionInDRS(section)}
	if car.Position <= 1 {
		return res
	}
	ahead := rs.carByPosition(car.Position - 1)
	if ahead == nil || ahead.Status == StatusCrashed {
		return res
	}
	// Wait until the car ahead has posted this round's pace.
	if !ahead.ShotThisRound && ahead.Status == StatusRacing {
		return res
	}
	res.otherRange = ahead.RangeNum
	res.otherPace = ahead.LastSpeed
	ratio := cfgFloat(rs.Config, "overtakeRatio", 1.12)
	canPass := false
	viaDRS := false
	if res.inDRS {
		placesBehind := car.Position - 1
		if placesBehind > 3 {
			placesBehind = 3
		}
		boost := 1.0 + float64(placesBehind)*cfgFloat(rs.Config, "drsStackPerPlace", 0.12)
		if speed*boost >= ahead.LastSpeed {
			canPass = true
			viaDRS = true
		}
	}
	if !canPass && speed >= ahead.LastSpeed*ratio {
		canPass = true
	}
	if !canPass {
		reason := fmt.Sprintf("Kein Überholen — Bahn %d zu stark (%.1f vs %.1f)", ahead.RangeNum, ahead.LastSpeed, speed)
		if res.inDRS {
			reason = fmt.Sprintf("Bahn %d mit DRS nicht eingeholt (%.1f vs %.1f)", ahead.RangeNum, ahead.LastSpeed, speed)
		}
		setCarNote(car, "blocked", reason)
		return res
	}
	car.Position, ahead.Position = ahead.Position, car.Position
	res.passed = true
	res.usedDRS = viaDRS
	drsTag := ""
	if viaDRS {
		drsTag = " — DRS"
	}
	setCarNote(car, "overtake", fmt.Sprintf("Überholt Bahn %d%s", ahead.RangeNum, drsTag))
	loseTag := ""
	if viaDRS {
		loseTag = " — DRS aktiv"
	}
	setCarNote(ahead, "overtaken", fmt.Sprintf("Platz verloren an Bahn %d%s", car.RangeNum, loseTag))
	return res
}

// resolveRoundPasses repeatedly applies section passes among cars that have
// already shot this round, so order matches this round's paces regardless of
// who fired first.
func (rs *RaceState) resolveRoundPasses(section int) {
	for pass := 0; pass < rs.NumRanges; pass++ {
		moved := false
		for i := 1; i <= rs.NumRanges; i++ {
			car := rs.Cars[strconv.Itoa(i)]
			if car == nil || car.Status != StatusRacing || !car.ShotThisRound {
				continue
			}
			if car.Position <= 1 {
				continue
			}
			ot := rs.trySectionPass(car, car.LastSpeed, section)
			if ot.passed {
				moved = true
			}
		}
		if !moved {
			return
		}
	}
}

// snapFieldToSection parks the whole field at the given shot's section marker,
// ordered by race Position. Everyone shares the section so beginners are not
// left behind on the track — only order differs.
func (rs *RaceState) snapFieldToSection(shotNum int) {
	stint := rs.stintSize()
	if shotNum < 1 {
		shotNum = 1
	}
	section := rs.sectionOfShot(shotNum)
	lap := (shotNum - 1) / stint
	base := float64(lap) + float64(section)/float64(stint)

	var list []*CarState
	for _, c := range rs.Cars {
		if c == nil || c.Status == StatusCrashed {
			continue
		}
		if c.Status == StatusRacing || c.Status == StatusFinished {
			list = append(list, c)
		}
	}
	if len(list) == 0 {
		return
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Position != list[j].Position {
			return list[i].Position < list[j].Position
		}
		return list[i].RangeNum < list[j].RangeNum
	})
	// Pack inside ~65% of one section so sprites stay on this marker, not the next.
	sectionSpan := 1.0 / float64(stint)
	gap := (sectionSpan * 0.65) / float64(len(list))
	if gap < 0.006 {
		gap = 0.006
	}
	if gap > 0.02 {
		gap = 0.02
	}
	for i, c := range list {
		c.Position = i + 1
		c.Progress = base - float64(i)*gap
	}
}

// applyOvertake delegates to section pass logic (no continuous progress deltas).
func (rs *RaceState) applyOvertake(car *CarState, speed, desiredDelta float64) overtakeResult {
	_ = desiredDelta
	shotNum := car.ShotsFired
	if shotNum < 1 {
		shotNum = 1
	}
	section := rs.sectionOfShot(shotNum)
	ot := rs.trySectionPass(car, speed, section)
	rs.snapFieldToSection(shotNum)
	return ot
}

// enforceMinGap keeps racing cars in single file: nobody may sit on top of
// another car. Trailing cars are pulled back to maintain gridGap behind the
// car immediately ahead (by progress).
func (rs *RaceState) enforceMinGap() {
	gap := cfgFloat(rs.Config, "gridGap", 0.08)
	if gap < 0.06 {
		gap = 0.06
	}
	type pair struct {
		c *CarState
		p float64
	}
	var list []pair
	for _, c := range rs.Cars {
		if c == nil || c.Status == StatusCrashed {
			continue
		}
		list = append(list, pair{c, c.Progress})
	}
	if len(list) < 2 {
		return
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].p != list[j].p {
			return list[i].p > list[j].p
		}
		return list[i].c.RangeNum < list[j].c.RangeNum
	})
	for i := 1; i < len(list); i++ {
		maxAllowed := list[i-1].c.Progress - gap
		if list[i].c.Progress > maxAllowed {
			list[i].c.Progress = maxAllowed
		}
	}
}

// formStartingGrid parks every active car in a tight single-file pack just
// behind the start/finish, Bahn 1 at the front. Gaps match snapFieldToSection
// so the first scoring shot does not teleport the field backwards.
func (rs *RaceState) formStartingGrid() {
	order := make([]*CarState, 0, rs.NumRanges)
	for i := 1; i <= rs.NumRanges; i++ {
		c := rs.Cars[strconv.Itoa(i)]
		if c == nil || c.Status == StatusCrashed {
			continue
		}
		order = append(order, c)
	}
	n := len(order)
	if n == 0 {
		return
	}
	stint := rs.stintSize()
	sectionSpan := 1.0 / float64(stint)
	gap := (sectionSpan * 0.65) / float64(n)
	if gap < 0.006 {
		gap = 0.006
	}
	if gap > 0.02 {
		gap = 0.02
	}
	for i, car := range order {
		car.Progress = gap * float64(n-1-i)
		car.Position = i + 1
	}
	rs.recomputePositions()
}

func (rs *RaceState) applyPitShot(car *CarState, ctx logicapi.ShotContext, field bool) []logicapi.PluginEvent {
	var events []logicapi.PluginEvent
	// Both stint and field pits consume the upcoming budgeted shot index.
	shotNum := car.ShotsFired + 1
	section := rs.sectionOfShot(shotNum)

	// Pit zero: hard fail — lose one place, no reaction bonus. Still clears the pit.
	if isZeroShot(ctx.Shot) {
		car.LastSpeed = 0
		car.LastBoostKind = "miss"
		dropped := rs.dropOnePlace(car)
		note := "Pit-Null — einen Platz verloren"
		if !dropped {
			note = "Pit-Null — letzter Platz"
		}
		setCarNote(car, "pit_fail", note)
		events = append(events, logicapi.PluginEvent{Type: "pit_fail", Data: map[string]any{
			"rangeNum": car.RangeNum, "reason": "zero", "dropped": dropped, "field": field,
		}})
		storeShotCoords(car, ctx.Shot)
		car.PendingPit = false
		return events
	}

	cue := time.Time{}
	if field && rs.ActiveFieldEvent != nil {
		cue = rs.ActiveFieldEvent.CueAt
	} else if rs.PitCueAt != nil {
		cue = *rs.PitCueAt
	} else {
		cue = ctx.Now
	}
	reaction := ctx.Now.Sub(cue).Milliseconds()
	if reaction < 0 {
		reaction = 0
	}
	window := cfgInt(rs.Config, "pitCueWindowMs", 5000)
	scoreW := cfgFloat(rs.Config, "pitScoreWeight", 1.0)
	reactW := cfgFloat(rs.Config, "pitReactionWeight", 0.0004)
	pace := rs.compressPace(effectiveScore(ctx.Shot, car.Handicap))
	// Keep pit bonus on the same scale as normal section pace so LastSpeed
	// comparisons stay meaningful for the next round.
	bonus := scoreW*pace + reactW*math.Max(0, float64(window)-float64(reaction))
	car.LastBoostKind = "pit"
	if reaction > int64(window) {
		bonus = pace * 0.5
		car.LastBoostKind = ""
		events = append(events, logicapi.PluginEvent{Type: "pit_slow", Data: map[string]any{"rangeNum": car.RangeNum, "reactionMs": reaction}})
		setCarNote(car, "pit_ok", fmt.Sprintf("Pit langsam (%dms)", reaction))
	} else {
		events = append(events, logicapi.PluginEvent{Type: "pit_ok", Data: map[string]any{"rangeNum": car.RangeNum, "reactionMs": reaction, "bonus": bonus}})
		setCarNote(car, "pit_ok", fmt.Sprintf("Pit OK — Bonus (%.0fms)", float64(reaction)))
	}
	car.LastSpeed = bonus
	ot := rs.trySectionPass(car, bonus, section)
	if ot.passed {
		events = append(events, logicapi.PluginEvent{Type: "overtake", Data: map[string]any{
			"rangeNum":   car.RangeNum,
			"section":    section,
			"viaDRS":     false,
			"pit":        true,
			"otherRange": ot.otherRange,
			"myPace":     bonus,
			"otherPace":  ot.otherPace,
		}})
	}
	storeShotCoords(car, ctx.Shot)
	car.PendingPit = false
	return events
}

func (rs *RaceState) maybeAdvanceRound(now time.Time) []logicapi.PluginEvent {
	// If all racing cars have shot this round, close early
	pending := false
	for _, car := range rs.Cars {
		if car.Status != StatusRacing {
			continue
		}
		if !car.ShotThisRound {
			pending = true
			break
		}
	}
	if !pending && rs.GridSet {
		completed := rs.CurrentRound
		rs.snapFieldToSection(completed)
		rs.CurrentRound++
		rs.RoundOpenedAt = nil
		rs.PitCueAt = nil
		for _, car := range rs.Cars {
			car.ShotThisRound = false
		}
		return rs.armPitRoundIfNeeded(now)
	}
	return nil
}

func (rs *RaceState) isPitRound(round int) bool {
	stintSize := cfgInt(rs.Config, "stintSize", 10)
	return stintSize > 0 && round > 0 && round%stintSize == 0
}

// armPitRoundIfNeeded starts a shared pit countdown when the new round is a
// stint pit. All ranges see the same pitCueAt so countdowns stay in sync.
func (rs *RaceState) armPitRoundIfNeeded(now time.Time) []logicapi.PluginEvent {
	if rs.Phase != PhaseRacing || !rs.GridSet {
		return nil
	}
	if !rs.isPitRound(rs.CurrentRound) {
		return nil
	}
	if rs.PitCueAt != nil {
		return nil
	}
	return rs.armPitRound(now)
}

func (rs *RaceState) armPitRound(now time.Time) []logicapi.PluginEvent {
	rs.PitCueAt = &now
	rs.StintPitRound = rs.CurrentRound
	window := cfgInt(rs.Config, "pitCueWindowMs", 5000)
	return []logicapi.PluginEvent{{Type: "pit_cue", Data: map[string]any{
		"kind":     "stint",
		"round":    rs.CurrentRound,
		"at":       now.UTC().Format(time.RFC3339Nano),
		"windowMs": window,
	}}}
}

func (rs *RaceState) maybeFinish() {
	allDone := true
	any := false
	for _, car := range rs.Cars {
		if car.Status == StatusRacing {
			allDone = false
		}
		if car.Status == StatusFinished || car.Status == StatusCrashed || car.Status == StatusRacing {
			any = true
		}
	}
	if any && allDone {
		rs.Phase = PhaseFinished
	}
}

func (rs *RaceState) maybeRandomFieldEvent(now time.Time, events *[]logicapi.PluginEvent) {
	if !cfgBool(rs.Config, "fieldEventsEnabled", true) {
		return
	}
	if rs.ActiveFieldEvent != nil {
		return
	}
	minGap := cfgInt(rs.Config, "fieldEventMinGapSec", 90)
	if rs.LastFieldEventAt != nil && now.Sub(*rs.LastFieldEventAt) < time.Duration(minGap)*time.Second {
		return
	}
	chance := cfgFloat(rs.Config, "fieldEventChancePerRound", 0.08)
	// Mix wall-clock entropy so the same round numbers do not always trigger.
	seed := float64(now.UnixNano()%1000000)/1000000.0 + float64(rs.CurrentRound)*0.137
	if math.Mod(seed, 1.0) > chance {
		return
	}
	typ := "puncture"
	if (now.UnixNano() & 1) == 0 {
		typ = "oil_leak"
	}
	*events = append(*events, rs.openFieldEvent(typ, now, 0)...)
}

// openFieldEvent starts a field pit on the next budgeted shot.
// puncture → single car (forceRange if >0, else pick one racing car).
// oil_leak (and unknown types) → all racing cars.
func (rs *RaceState) openFieldEvent(typ string, now time.Time, forceRange int) []logicapi.PluginEvent {
	win := cfgInt(rs.Config, "pitCueWindowMs", 5000)
	targets := rs.pickFieldTargets(typ, forceRange, now)
	rs.ActiveFieldEvent = &FieldEvent{
		Type:     typ,
		CueAt:    now,
		WindowMs: win,
		Targets:  targets,
		Cleared:  map[int]bool{},
	}
	rs.LastFieldEventAt = &now
	rs.PitCueAt = &now
	for rangeNum := range targets {
		car := rs.Cars[strconv.Itoa(rangeNum)]
		if car == nil || car.Status != StatusRacing {
			continue
		}
		car.PendingPit = true
		car.PitKind = "field"
		if typ == "puncture" {
			setCarNote(car, "pit_fail", "Reifenplatzer — PIT NOW")
		} else {
			setCarNote(car, "pit_fail", "Ölverlust — PIT NOW")
		}
	}
	targetList := make([]int, 0, len(targets))
	for rn := range targets {
		targetList = append(targetList, rn)
	}
	sort.Ints(targetList)
	return []logicapi.PluginEvent{{Type: "field_event", Data: map[string]any{
		"type": typ, "cueAt": now.UTC().Format(time.RFC3339Nano), "windowMs": win, "targets": targetList,
	}}}
}

func (rs *RaceState) pickFieldTargets(typ string, forceRange int, now time.Time) map[int]bool {
	targets := map[int]bool{}
	var racing []*CarState
	for i := 1; i <= rs.NumRanges; i++ {
		c := rs.Cars[strconv.Itoa(i)]
		if c != nil && c.Status == StatusRacing {
			racing = append(racing, c)
		}
	}
	if len(racing) == 0 {
		return targets
	}
	if typ == "puncture" {
		if forceRange > 0 {
			for _, c := range racing {
				if c.RangeNum == forceRange {
					targets[forceRange] = true
					return targets
				}
			}
		}
		idx := int(now.UnixNano()) % len(racing)
		if idx < 0 {
			idx = -idx
		}
		targets[racing[idx].RangeNum] = true
		return targets
	}
	// oil_leak: everyone
	for _, c := range racing {
		targets[c.RangeNum] = true
	}
	return targets
}

func (rs *RaceState) allFieldCleared() bool {
	if rs.ActiveFieldEvent == nil {
		return true
	}
	for rangeNum := range rs.ActiveFieldEvent.Targets {
		car := rs.Cars[strconv.Itoa(rangeNum)]
		if car == nil || car.Status != StatusRacing {
			continue
		}
		if !rs.ActiveFieldEvent.Cleared[rangeNum] {
			return false
		}
	}
	return true
}

func (rs *RaceState) recomputePositions() {
	type pair struct {
		c *CarState
		p float64
	}
	var list []pair
	var crashed []*CarState
	for _, c := range rs.Cars {
		if c == nil {
			continue
		}
		if c.Status == StatusCrashed {
			crashed = append(crashed, c)
			continue
		}
		if c.Status != StatusRacing && c.Status != StatusFinished &&
			c.Status != StatusReady && c.Status != StatusActive {
			continue
		}
		list = append(list, pair{c, c.Progress})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].p != list[j].p {
			return list[i].p > list[j].p
		}
		return list[i].c.RangeNum < list[j].c.RangeNum
	})
	for i, p := range list {
		p.c.Position = i + 1
	}
	// Crashed cars keep trailing positions so carByPosition never lands on them mid-pack.
	base := len(list)
	sort.Slice(crashed, func(i, j int) bool { return crashed[i].RangeNum < crashed[j].RangeNum })
	for i, c := range crashed {
		c.Position = base + i + 1
	}
}

func isZeroShot(shot state.Shot) bool {
	return shot.FullValue == 0 && shot.DecValue <= 0
}

func setCarNote(car *CarState, kind, note string) {
	if car == nil {
		return
	}
	car.LastNoteKind = kind
	car.LastNote = note
}

func fieldTypeLabel(typ string) string {
	switch typ {
	case "puncture":
		return "Reifenplatzer"
	case "oil_leak":
		return "Ölverlust"
	default:
		return typ
	}
}

func boostLabelDE(kind string) string {
	switch kind {
	case "streak":
		return "Streak-Bonus"
	case "hole_in_hole":
		return "Hole-in-Hole"
	case "pit":
		return "Pit-Bonus"
	case "drs":
		return "DRS"
	case "miss":
		return "Nullschuss"
	default:
		return ""
	}
}

// dropOnePlace swaps the car with the one immediately behind. Returns false if already last.
func (rs *RaceState) dropOnePlace(car *CarState) bool {
	if car == nil {
		return false
	}
	behind := rs.carByPosition(car.Position + 1)
	if behind == nil {
		return false
	}
	car.Position, behind.Position = behind.Position, car.Position
	setCarNote(behind, "overtake", fmt.Sprintf("Bahn %d Pit-Null — Platz gewonnen", car.RangeNum))
	return true
}

func (rs *RaceState) refreshPlaceReasons() {
	for _, car := range rs.Cars {
		if car == nil {
			continue
		}
		if car.Status != StatusRacing && car.Status != StatusFinished {
			if car.Status == StatusCrashed {
				car.PlaceReason = "DNF — ausgefallen"
				car.PlaceReasonKind = "crash"
				car.NextHint = ""
				car.NextHintKind = ""
			}
			continue
		}
		if car.Position <= 1 {
			car.PlaceReason = fmt.Sprintf("P1 — führend (Tempo %.1f)", car.LastSpeed)
			car.PlaceReasonKind = "lead"
		} else {
			ahead := rs.carByPosition(car.Position - 1)
			if ahead == nil {
				car.PlaceReason = fmt.Sprintf("P%d", car.Position)
				car.PlaceReasonKind = "place"
			} else if car.LastBoostKind == "miss" {
				car.PlaceReason = fmt.Sprintf("P%d — Nullschuss; Feld zieht vorbei", car.Position)
				car.PlaceReasonKind = "miss"
			} else if label := boostLabelDE(ahead.LastBoostKind); label != "" && ahead.LastBoostKind != "miss" {
				car.PlaceReason = fmt.Sprintf("P%d — Bahn %d hat %s", car.Position, ahead.RangeNum, label)
				car.PlaceReasonKind = "bonus"
			} else {
				car.PlaceReason = fmt.Sprintf("P%d — Bahn %d war besser (%.1f vs %.1f)",
					car.Position, ahead.RangeNum, ahead.LastSpeed, car.LastSpeed)
				car.PlaceReasonKind = "pace"
			}
		}
		rs.refreshNextHint(car)
	}
}

// refreshNextHint writes a concrete prep line for the shooter's next action
// (overtake target, DRS, pit cue, defend lead).
func (rs *RaceState) refreshNextHint(car *CarState) {
	if car == nil || car.Status != StatusRacing {
		if car != nil && car.Status == StatusFinished {
			car.NextHint = "Im Ziel"
			car.NextHintKind = "done"
		}
		return
	}

	// Field event: next budgeted shot is a pit for targeted cars only.
	// Others keep racing — field pits are not extra shots.
	if rs.ActiveFieldEvent != nil && rs.ActiveFieldEvent.Targets[car.RangeNum] && !rs.ActiveFieldEvent.Cleared[car.RangeNum] {
		if rs.ActiveFieldEvent.Type == "puncture" {
			car.NextHint = "Reifenplatzer — nächster Schuss ist PIT, schnell und treffsicher"
		} else {
			car.NextHint = "Ölverlust — nächster Schuss ist PIT, schnell und treffsicher"
		}
		car.NextHintKind = "pit_now"
		return
	}

	if !rs.GridSet {
		car.NextHint = "Startschuss — bester Schuss sichert die Pole"
		car.NextHintKind = "grid"
		return
	}

	nextRound := rs.CurrentRound
	if car.ShotThisRound {
		nextRound = rs.CurrentRound + 1
	}
	if nextRound < 1 {
		nextRound = 1
	}

	if rs.isPitRound(nextRound) {
		if car.ShotThisRound {
			car.NextHint = "Nächste Runde ist PIT — auf Signal reagieren"
		} else {
			car.NextHint = "PIT-Runde — auf Signal reagieren, Treffer + Tempo"
		}
		car.NextHintKind = "pit_prep"
		return
	}

	section := rs.sectionOfShot(nextRound)
	inDRS := rs.sectionInDRS(section)
	stint := rs.stintSize()
	shotsToPit := stint - ((nextRound-1) % stint) - 1
	if shotsToPit < 0 {
		shotsToPit = 0
	}

	if car.ShotThisRound {
		if shotsToPit == 0 {
			car.NextHint = "Warten — nächste Runde PIT"
		} else if inDRS {
			car.NextHint = fmt.Sprintf("Warten — nächste Sektion %d mit DRS", section)
		} else {
			car.NextHint = fmt.Sprintf("Warten auf Feld — noch %d bis PIT", shotsToPit)
		}
		car.NextHintKind = "wait"
		return
	}

	ahead := rs.carByPosition(car.Position - 1)
	behind := rs.carByPosition(car.Position + 1)

	if car.Position <= 1 {
		if behind != nil {
			need := rs.defendPace(car, behind, inDRS)
			car.NextHint = fmt.Sprintf("Führung halten — stärker als Bahn %d (Ziel Tempo ≥ %.1f)", behind.RangeNum, need)
			if inDRS {
				car.NextHint = fmt.Sprintf("P1 · DRS — Bahn %d lauert, Tempo ≥ %.1f halten", behind.RangeNum, behind.LastSpeed)
			}
			car.NextHintKind = "defend"
			return
		}
		car.NextHint = "Führung halten — stark und sicher schießen"
		car.NextHintKind = "defend"
		return
	}

	if ahead == nil {
		car.NextHint = "Bereit — nächster Schuss"
		car.NextHintKind = "ready"
		return
	}

	need := rs.overtakeTargetPace(ahead, car.Position, inDRS)
	if inDRS {
		car.NextHint = fmt.Sprintf("DRS aktiv — schieß mind. Tempo %.1f (Bahn %d hat %.1f) zum Überholen",
			need, ahead.RangeNum, ahead.LastSpeed)
		car.NextHintKind = "overtake_drs"
		return
	}
	car.NextHint = fmt.Sprintf("Schieß besser als Bahn %d — Ziel Tempo ≥ %.1f (aktuell %.1f) zum Überholen",
		ahead.RangeNum, need, ahead.LastSpeed)
	car.NextHintKind = "overtake"
}

func (rs *RaceState) overtakeTargetPace(ahead *CarState, chasePos int, inDRS bool) float64 {
	if ahead == nil {
		return 0
	}
	if inDRS {
		placesBehind := chasePos - 1
		if placesBehind > 3 {
			placesBehind = 3
		}
		boost := 1.0 + float64(placesBehind)*cfgFloat(rs.Config, "drsStackPerPlace", 0.12)
		if boost < 0.01 {
			boost = 1
		}
		return ahead.LastSpeed / boost
	}
	return ahead.LastSpeed * cfgFloat(rs.Config, "overtakeRatio", 1.12)
}

// defendPace: pace the leader needs so the car behind cannot pass this section.
func (rs *RaceState) defendPace(leader, behind *CarState, inDRS bool) float64 {
	if behind == nil {
		return leader.LastSpeed
	}
	if inDRS {
		placesBehind := behind.Position - 1
		if placesBehind > 3 {
			placesBehind = 3
		}
		boost := 1.0 + float64(placesBehind)*cfgFloat(rs.Config, "drsStackPerPlace", 0.12)
		return behind.LastSpeed*boost + 0.05
	}
	ratio := cfgFloat(rs.Config, "overtakeRatio", 1.12)
	if ratio < 0.01 {
		ratio = 1.12
	}
	return behind.LastSpeed/ratio + 0.05
}

func effectiveScore(shot state.Shot, handicap float64) float64 {
	s := shot.DecValue
	if s <= 0 {
		s = float64(shot.FullValue)
	}
	if handicap <= 0 {
		handicap = 1
	}
	return s * handicap
}

// compressPace remaps Dec-based score toward pacePivot:
//
//	pace = pivot + (score - pivot) * paceCompress
//
// compress=1 keeps live Dec pace; 0.5 is Sweet spot A.
func (rs *RaceState) compressPace(score float64) float64 {
	compress := cfgFloat(rs.Config, "paceCompress", 0.50)
	if compress >= 0.999 {
		return score
	}
	if compress < 0 {
		compress = 0
	}
	pivot := cfgFloat(rs.Config, "pacePivot", 8.0)
	return pivot + (score-pivot)*compress
}

func storeShotCoords(car *CarState, shot state.Shot) {
	car.LastShotX = shot.X
	car.LastShotY = shot.Y
	car.HasLastShot = true
	if shot.DecValue > 0 {
		car.LastShotValue = shot.DecValue
	} else {
		car.LastShotValue = float64(shot.FullValue)
	}
}

func inDRSZone(zones []DRSZone, progress float64) bool {
	lap := progress - math.Floor(progress)
	for _, z := range zones {
		if lap >= z.Start && lap <= z.End {
			return true
		}
	}
	return false
}

func resolveDRSZones(cfg map[string]any, circuit string, stint int) []DRSZone {
	if sections := cfgIntList(cfg, "drsSections"); len(sections) > 0 {
		return zonesFromSections(sections, stint)
	}
	if z := circuitDRS[circuit]; z != nil {
		return z
	}
	return circuitDRS["spa"]
}

func zonesFromSections(sections []int, stint int) []DRSZone {
	if stint < 2 {
		stint = 10
	}
	set := map[int]bool{}
	for _, s := range sections {
		if s >= 1 && s < stint {
			set[s] = true
		}
	}
	list := make([]int, 0, len(set))
	for s := range set {
		list = append(list, s)
	}
	sort.Ints(list)
	var zones []DRSZone
	for i := 0; i < len(list); {
		start := list[i]
		end := start
		for i+1 < len(list) && list[i+1] == end+1 {
			i++
			end = list[i]
		}
		zStart := (float64(start) - 0.5) / float64(stint)
		zEnd := (float64(end) - 0.5) / float64(stint)
		pad := 0.04
		zones = append(zones, DRSZone{
			Start: math.Max(0, zStart-pad),
			End:   math.Min(0.999, zEnd+pad),
		})
		i++
	}
	return zones
}

func cfgIntList(m map[string]any, key string) []int {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	var out []int
	switch x := v.(type) {
	case string:
		for _, part := range strings.Split(x, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			n, err := strconv.Atoi(part)
			if err == nil {
				out = append(out, n)
			}
		}
	case []any:
		for _, item := range x {
			switch t := item.(type) {
			case float64:
				if int(t) > 0 {
					out = append(out, int(t))
				}
			case int:
				if t > 0 {
					out = append(out, t)
				}
			case int64:
				if t > 0 {
					out = append(out, int(t))
				}
			case json.Number:
				i, err := t.Int64()
				if err == nil && i > 0 {
					out = append(out, int(i))
				}
			case string:
				n, err := strconv.Atoi(strings.TrimSpace(t))
				if err == nil && n > 0 {
					out = append(out, n)
				}
			}
		}
	case []int:
		out = append(out, x...)
	}
	return out
}

// circleOverlapRatio returns intersection area / area of one circle.
func circleOverlapRatio(x1, y1, x2, y2, r float64) float64 {
	if r <= 0 {
		return 0
	}
	d := math.Hypot(x2-x1, y2-y1)
	if d >= 2*r {
		return 0
	}
	if d <= 0 {
		return 1
	}
	// intersection of two equal circles
	part := 2 * r * r * math.Acos(d/(2*r))
	part -= 0.5 * d * math.Sqrt(4*r*r-d*d)
	area := math.Pi * r * r
	if area <= 0 {
		return 0
	}
	return part / area
}

func marshalState(rs *RaceState) (logicapi.SessionState, error) {
	b, err := json.Marshal(rs)
	return logicapi.SessionState(b), err
}

func marshalWithEvents(rs *RaceState, events []logicapi.PluginEvent) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	b, err := marshalState(rs)
	return b, events, err
}

func unmarshalState(sess logicapi.SessionState) (*RaceState, error) {
	var rs RaceState
	if len(sess) == 0 {
		return nil, fmt.Errorf("empty state")
	}
	if err := json.Unmarshal(sess, &rs); err != nil {
		return nil, err
	}
	if rs.Cars == nil {
		rs.Cars = map[string]*CarState{}
	}
	return &rs, nil
}

func cfgString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func cfgInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(v)
		if i != 0 {
			return i
		}
	}
	return def
}

func cfgFloat(m map[string]any, key string, def float64) float64 {
	if m == nil {
		return def
	}
	return toFloat(m[key], def)
}

func toFloat(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, err := x.Float64()
		if err == nil {
			return f
		}
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func cfgBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	}
	return def
}

var _ logicapi.ExtendedLogic = (*Logic)(nil)
