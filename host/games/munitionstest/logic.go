package munitionstest

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"srdashboard/host/games/gameutil"
	"srdashboard/host/loader"
	"srdashboard/host/logicapi"
	"srdashboard/state"
)

// dsgPerMm matches OpticScore: 100 DSG = 1 mm.
const dsgPerMm = 100.0

// Ring widths are the ISSF radial step from one ring to the next.
// Luftgewehr is 2.5 mm, so 0.2 rings — a good 10-shot lot — is 0.5 mm
// from the first shot. Luftpistole and Kleinkaliber use 8 mm.
const lgRingWidthMm = 2.5
const lpRingWidthMm = 8.0

// ringStep is one tenth of a ring. The abort button grows once for the
// first lot that is worse, then again for each further tenth.
const ringStep = 0.1

func init() {
	loader.RegisterBuiltin("munitionstest", func(m *loader.Manifest) logicapi.Logic {
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
	return "munitionstest"
}

func (l *Logic) Label() string {
	if l.manifest != nil && l.manifest.Label != "" {
		return l.manifest.Label
	}
	return "Munitionstest"
}

func (l *Logic) Version() string {
	if l.manifest != nil && l.manifest.Version != "" {
		return l.manifest.Version
	}
	return "1.0.0"
}

func (l *Logic) DefaultConfig() map[string]any {
	return map[string]any{"shotsPerLot": 10}
}

func (l *Logic) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"shotsPerLot": map[string]any{"type": "integer", "minimum": 2, "maximum": 40},
		},
	}
}

// Lot is one ammunition (one Los) shared by every range.
type Lot struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ShotPt is a hole centre in DSG.
type ShotPt struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Result is one range's group for one lot.
// Name keeps the label after the lot leaves the catalog.
type Result struct {
	LotID  int      `json:"lotId"`
	Name   string   `json:"name,omitempty"`
	Shots  []ShotPt `json:"shots"`
	Closed bool     `json:"closed"`
	Weapon string   `json:"weapon"`
}

type session struct {
	Lots      []Lot    `json:"lots"`
	NextID    int      `json:"nextId"`
	Weapon    string   `json:"weapon"`
	ActiveLot int      `json:"activeLot"`
	Disc      string   `json:"disc"`
	Results   []Result `json:"results"`
	ShotsPer  int      `json:"shotsPer"`
}

func (st *session) ensure() {
	if st.ShotsPer < 2 {
		st.ShotsPer = 10
	}
	if st.NextID < 1 {
		st.NextID = 1
	}
	if st.Lots == nil {
		st.Lots = []Lot{}
	}
	if st.Results == nil {
		st.Results = []Result{}
	}
}

func (l *Logic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	shotsPer := int(gameutil.ToFloat(cfg["shotsPerLot"], 10))
	if shotsPer < 2 {
		shotsPer = 10
	}
	st := &session{
		Lots:     parseLots(cfg["lotsJson"]),
		NextID:   int(gameutil.ToFloat(cfg["nextId"], 1)),
		ShotsPer: shotsPer,
	}
	st.ensure()
	for _, lot := range st.Lots {
		if lot.ID >= st.NextID {
			st.NextID = lot.ID + 1
		}
	}
	return encodeState(st)
}

func (l *Logic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	return l.OnShotCtx(sess, logicapi.ShotContext{RangeNum: rangeNum, Shot: shot, ShotIndex: shotIndex})
}

func (l *Logic) OnShotCtx(sess logicapi.SessionState, ctx logicapi.ShotContext) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := decode(sess)
	if err != nil {
		return sess, nil, err
	}
	if st.ActiveLot == 0 {
		return done(st)
	}
	r := st.result(st.ActiveLot)
	if r == nil || r.Closed || len(r.Shots) >= st.ShotsPer {
		return done(st)
	}
	if st.Disc == "" {
		st.Disc = strings.TrimSpace(ctx.Live.DiscType)
		if st.Disc == "" {
			st.Disc = strings.TrimSpace(ctx.Live.Discipline)
		}
	}
	r.Shots = append(r.Shots, ShotPt{X: ctx.Shot.X, Y: ctx.Shot.Y})
	if len(r.Shots) >= st.ShotsPer {
		r.Closed = true
	}
	return done(st)
}

func (l *Logic) Tick(sess logicapi.SessionState, _ time.Time) (logicapi.SessionState, []logicapi.PluginEvent, bool, error) {
	return sess, nil, false, nil
}

func (l *Logic) Control(sess logicapi.SessionState, action string, params map[string]any) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	st, err := decode(sess)
	if err != nil {
		return sess, nil, err
	}
	if params == nil {
		params = map[string]any{}
	}
	var events []logicapi.PluginEvent
	switch action {
	case "start", "sync_live":
		return done(st)
	case "addLot":
		name := strings.TrimSpace(strParam(params["name"]))
		if name == "" {
			return sess, nil, fmt.Errorf("Name fehlt")
		}
		lot := Lot{ID: st.NextID, Name: name}
		st.NextID++
		st.Lots = append(st.Lots, lot)
		if flag(params["select"]) {
			if err := st.selectLot(lot.ID); err != nil {
				return sess, nil, err
			}
		}
		events = catalogEvents(st)
	case "removeLot":
		id := int(gameutil.ToFloat(params["lotId"], 0))
		if st.lotName(id) == "" {
			return sess, nil, fmt.Errorf("Los nicht gefunden")
		}
		next := make([]Lot, 0, len(st.Lots)-1)
		for _, lot := range st.Lots {
			if lot.ID != id {
				next = append(next, lot)
			}
		}
		st.dropCatalogLots(next)
		st.Lots = next
		events = catalogEvents(st)
	case "selectLot":
		if err := st.selectLot(int(gameutil.ToFloat(params["lotId"], 0))); err != nil {
			return sess, nil, err
		}
	case "abort":
		st.closeActive(true)
	case "undo":
		if r := st.result(st.ActiveLot); r != nil && len(r.Shots) > 0 {
			r.Shots = r.Shots[:len(r.Shots)-1]
			if len(r.Shots) < st.ShotsPer {
				r.Closed = false
			}
		}
	case "reset":
		st.Results = nil
		st.ActiveLot = 0
	case "setWeapon":
		w := strings.TrimSpace(strParam(params["weapon"]))
		lotID := int(gameutil.ToFloat(params["lotId"], 0))
		st.Weapon = w
		if lotID == 0 {
			lotID = st.ActiveLot
		}
		if r := st.result(lotID); r != nil {
			r.Weapon = w
		}
	default:
		return sess, nil, fmt.Errorf("unknown action %q", action)
	}
	out, err := encodeState(st)
	if err != nil {
		return sess, nil, err
	}
	return out, events, nil
}

func (l *Logic) ApplyFanout(sess logicapi.SessionState, payload map[string]any) (logicapi.SessionState, error) {
	st, err := decode(sess)
	if err != nil {
		return sess, err
	}
	if payload == nil {
		return encodeState(st)
	}
	if _, ok := payload["lotsJson"]; ok {
		nextLots := parseLots(payload["lotsJson"])
		st.dropCatalogLots(nextLots)
		st.Lots = nextLots
	}
	next := int(gameutil.ToFloat(payload["nextId"], float64(st.NextID)))
	if next > st.NextID {
		st.NextID = next
	}
	st.ensure()
	if st.ActiveLot != 0 && st.lotName(st.ActiveLot) == "" {
		st.ActiveLot = 0
	}
	return encodeState(st)
}

func (l *Logic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	st, err := decode(sess)
	if err != nil {
		return nil, err
	}
	mode := "per-range"
	if l.manifest != nil && l.manifest.Mode != "" {
		mode = l.manifest.Mode
	}
	lots := make([]map[string]any, 0, len(st.Lots))
	for _, lot := range st.Lots {
		lots = append(lots, map[string]any{"id": lot.ID, "name": lot.Name})
	}
	results := make([]map[string]any, 0, len(st.Results))
	for i := range st.Results {
		results = append(results, st.resultView(&st.Results[i]))
	}
	vm := map[string]any{
		"pluginId":    l.ID(),
		"kind":        "game",
		"mode":        mode,
		"label":       l.Label(),
		"rangeNum":    rangeNum,
		"weapon":      st.Weapon,
		"shotsPer":    st.ShotsPer,
		"lots":        lots,
		"results":     results,
		"activeLotId": st.ActiveLot,
	}
	if st.ActiveLot != 0 {
		if r := st.result(st.ActiveLot); r != nil {
			vm["active"] = st.resultView(r)
		}
	}
	return vm, nil
}

func (st *session) resultView(r *Result) map[string]any {
	width := ringWidthForDisc(st.Disc)
	raw, hasGroup := diameterMm(r.Shots)
	best, bestTeiler, bestName, hasBest := st.bestOther(r.LotID)
	steps := 0
	if hasGroup && hasBest {
		steps = AbortSteps(raw, best, width)
	}
	full := len(r.Shots) >= st.ShotsPer
	open := !r.Closed && !full
	shots, groupTeiler := shotViews(r.Shots)
	view := map[string]any{
		"lotId":       r.LotID,
		"name":        st.displayName(r),
		"weapon":      r.Weapon,
		"shots":       shots,
		"shotCount":   len(r.Shots),
		"closed":      r.Closed || full,
		"full":        full,
		"open":        open,
		"hasGroup":    hasGroup,
		"hasAbort":    open,
		"abortSteps":  steps,
		"abortScale":  AbortScale(steps),
		"bestName":    bestName,
		"hasBest":     hasBest,
		"ringWidthMm": width,
	}
	if hasGroup {
		view["diameterMm"] = raw
		view["rings"] = spreadRings(raw, width)
		view["teiler"] = groupTeiler
		view["radiusMm"] = groupTeiler / dsgPerMm
	}
	if hasBest {
		view["bestMm"] = best
		view["bestRings"] = spreadRings(best, width)
		view["bestTeiler"] = bestTeiler
	}
	if len(r.Shots) > 0 {
		view["hasCenter"] = true
		view["centerMmX"] = float64(r.Shots[0].X) / dsgPerMm
		view["centerMmY"] = float64(r.Shots[0].Y) / dsgPerMm
	}
	return view
}

func (st *session) selectLot(id int) error {
	if st.lotName(id) == "" {
		return fmt.Errorf("Los nicht gefunden")
	}
	if st.ActiveLot != 0 && st.ActiveLot != id {
		st.closeActive(false)
	}
	r := st.ensureResult(id)
	if len(r.Shots) >= st.ShotsPer {
		r.Closed = true
	} else {
		r.Closed = false
	}
	if r.Weapon == "" {
		r.Weapon = st.Weapon
	}
	st.ActiveLot = id
	return nil
}

// closeActive ends the open lot. dropEmpty removes a lot that has no shots,
// so an abort before the first shot does not leave a blank row.
func (st *session) closeActive(dropEmpty bool) {
	if st.ActiveLot == 0 {
		return
	}
	id := st.ActiveLot
	r := st.result(id)
	st.ActiveLot = 0
	if r == nil {
		return
	}
	if len(r.Shots) == 0 && dropEmpty {
		st.dropResult(id)
		return
	}
	if len(r.Shots) == 0 {
		st.dropResult(id)
		return
	}
	r.Closed = true
}

func (st *session) ensureResult(id int) *Result {
	if r := st.result(id); r != nil {
		return r
	}
	st.Results = append(st.Results, Result{LotID: id, Weapon: st.Weapon})
	return &st.Results[len(st.Results)-1]
}

func (st *session) result(id int) *Result {
	if id == 0 {
		return nil
	}
	for i := range st.Results {
		if st.Results[i].LotID == id {
			return &st.Results[i]
		}
	}
	return nil
}

func (st *session) dropResult(id int) {
	out := st.Results[:0]
	for _, r := range st.Results {
		if r.LotID != id {
			out = append(out, r)
		}
	}
	st.Results = out
}

func (st *session) lotName(id int) string {
	for _, lot := range st.Lots {
		if lot.ID == id {
			return lot.Name
		}
	}
	return ""
}

func (st *session) displayName(r *Result) string {
	if name := st.lotName(r.LotID); name != "" {
		return name
	}
	return r.Name
}

// dropCatalogLots keeps shot rows for lots that leave the catalog.
// Empty series are dropped. A series with shots is closed and keeps its name.
func (st *session) dropCatalogLots(next []Lot) {
	keep := map[int]struct{}{}
	for _, lot := range next {
		keep[lot.ID] = struct{}{}
	}
	kept := st.Results[:0]
	for i := range st.Results {
		r := st.Results[i]
		if _, ok := keep[r.LotID]; ok {
			kept = append(kept, r)
			continue
		}
		if len(r.Shots) == 0 {
			continue
		}
		if r.Name == "" {
			r.Name = st.lotName(r.LotID)
		}
		r.Closed = true
		kept = append(kept, r)
	}
	st.Results = kept
	if _, ok := keep[st.ActiveLot]; !ok {
		st.ActiveLot = 0
	}
}

func (st *session) bestOther(exclude int) (mm, teiler float64, name string, ok bool) {
	for i := range st.Results {
		r := &st.Results[i]
		if r.LotID == exclude {
			continue
		}
		d, has := diameterMm(r.Shots)
		if !has {
			continue
		}
		if !ok || d < mm {
			ok = true
			mm = d
			_, teiler = shotViews(r.Shots)
			name = st.displayName(r)
		}
	}
	return mm, teiler, name, ok
}

// shotViews measures every hole from shot 1, not from the bullseye.
// dx/dy are millimetres. Teiler is hypot of that offset in DSG, rounded
// to 0.1 like OpticScore Distance. Shot 1 itself is the origin.
func shotViews(shots []ShotPt) ([]map[string]any, float64) {
	out := make([]map[string]any, len(shots))
	if len(shots) == 0 {
		return out, 0
	}
	cx, cy := float64(shots[0].X), float64(shots[0].Y)
	group := 0.0
	for i, s := range shots {
		dx := float64(s.X) - cx
		dy := float64(s.Y) - cy
		teiler := 0.0
		if i > 0 {
			teiler = math.Round(math.Hypot(dx, dy)*10) / 10
			if teiler > group {
				group = teiler
			}
		}
		out[i] = map[string]any{
			"x":      s.X,
			"y":      s.Y,
			"dxMm":   dx / dsgPerMm,
			"dyMm":   dy / dsgPerMm,
			"teiler": teiler,
		}
	}
	return out, group
}

func catalogEvents(st *session) []logicapi.PluginEvent {
	raw, err := json.Marshal(st.Lots)
	if err != nil {
		return nil
	}
	payload := map[string]any{
		"lotsJson": string(raw),
		"nextId":   st.NextID,
	}
	return []logicapi.PluginEvent{
		{Type: logicapi.EventFanout, Data: payload},
		{Type: "persist_config", Data: payload},
	}
}

// diameterMm is the streukreis diameter around shot 1, in mm.
// Shot 1 is the centre and does not itself add radius.
func diameterMm(shots []ShotPt) (float64, bool) {
	if len(shots) < 2 {
		return 0, false
	}
	cx, cy := float64(shots[0].X), float64(shots[0].Y)
	maxR := 0.0
	for _, s := range shots[1:] {
		r := math.Hypot(float64(s.X)-cx, float64(s.Y)-cy) / dsgPerMm
		if r > maxR {
			maxR = r
		}
	}
	return 2 * maxR, true
}

// spreadRings is how many ring widths the farthest shot sits from shot 1.
// A good lot is about 0.2.
func spreadRings(diameterMm, ringWidth float64) float64 {
	if ringWidth <= 0 {
		ringWidth = lgRingWidthMm
	}
	return (diameterMm / 2) / ringWidth
}

func ringWidthForDisc(disc string) float64 {
	d := strings.ToLower(strings.TrimSpace(disc))
	switch {
	case strings.Contains(d, "lp"), strings.Contains(d, "pistole"),
		strings.Contains(d, "kk"), strings.Contains(d, "klein"):
		return lpRingWidthMm
	default:
		return lgRingWidthMm
	}
}

// AbortSteps is 0 while the group is no larger than the best other lot.
// The first worse result is one step. Each further tenth of a ring — the
// scale a 0.2-ring lot is judged on — adds another step.
func AbortSteps(diameterMm, bestMm, ringWidth float64) int {
	if diameterMm <= 0 || bestMm <= 0 || ringWidth <= 0 {
		return 0
	}
	excess := spreadRings(diameterMm, ringWidth) - spreadRings(bestMm, ringWidth)
	if excess <= 1e-9 {
		return 0
	}
	tenths := int(math.Floor(excess/ringStep + 1e-9))
	return tenths + 1
}

// AbortScale is 1 until the group is worse than another lot, then grows
// by half a step per tenth of a ring. Capped so the button stays on the stand.
func AbortScale(steps int) float64 {
	if steps < 1 {
		return 1
	}
	s := 1 + float64(steps)*0.5
	if s > 4 {
		return 4
	}
	return s
}

func parseLots(v any) []Lot {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		var lots []Lot
		if err := json.Unmarshal([]byte(t), &lots); err != nil {
			return nil
		}
		return lots
	case []Lot:
		return append([]Lot(nil), t...)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		var lots []Lot
		if err := json.Unmarshal(b, &lots); err != nil {
			return nil
		}
		return lots
	}
}

func done(st *session) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	sess, err := encodeState(st)
	return sess, nil, err
}

func encodeState(st *session) (logicapi.SessionState, error) {
	st.ensure()
	b, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	return logicapi.SessionState(b), nil
}

func decode(sess logicapi.SessionState) (*session, error) {
	st := &session{}
	if len(bytesOf(sess)) == 0 || string(bytesOf(sess)) == "null" {
		st.ensure()
		return st, nil
	}
	if err := json.Unmarshal(bytesOf(sess), st); err != nil {
		return nil, err
	}
	st.ensure()
	return st, nil
}

func bytesOf(sess logicapi.SessionState) []byte {
	return []byte(sess)
}

func strParam(v any) string {
	s, _ := v.(string)
	return s
}

func flag(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}
