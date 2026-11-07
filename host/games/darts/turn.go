package darts

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"srdashboard/host/games/gameutil"
	"srdashboard/host/logicapi"
)

var cricketTargets = []string{"20", "19", "18", "17", "16", "15", "B"}

func newCricketMarks() map[string]int {
	m := make(map[string]int, len(cricketTargets))
	for _, key := range cricketTargets {
		m[key] = 0
	}
	return m
}

func (gs *GameState) player(rn int) *Player {
	if gs == nil || rn < 1 {
		return nil
	}
	return gs.Players[gameutil.Itoa(rn)]
}

func (gs *GameState) mode() string {
	m := gs.GameMode
	if m == "" && gs.Config != nil {
		m = gameutil.CfgString(gs.Config, "gameMode", "x01")
	}
	switch m {
	case "cricket", "around_clock":
		return m
	default:
		return "x01"
	}
}

func (gs *GameState) orderMode() string {
	switch gameutil.CfgString(gs.Config, "orderMode", "range") {
	case "beginner":
		return "beginner"
	default:
		return "range"
	}
}

func (gs *GameState) beginnerRange() int {
	n := gameutil.CfgInt(gs.Config, "beginnerRange", 1)
	if n < 1 {
		return 1
	}
	return n
}

func (gs *GameState) requireFunDart() bool {
	return gameutil.CfgBool(gs.Config, "requireFunDart", false)
}

func (gs *GameState) visitHold() time.Duration {
	ms := gameutil.CfgInt(gs.Config, "visitHoldMs", 1500)
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms) * time.Millisecond
}

func (gs *GameState) readOptions(params map[string]any) {
	if params == nil {
		return
	}
	if v, ok := params["gameMode"].(string); ok && v != "" {
		gs.Config["gameMode"] = v
		gs.GameMode = ""
	}
	if v, ok := params["startScore"]; ok {
		gs.Config["startScore"] = gameutil.CfgInt(map[string]any{"v": v}, "v", startScore(gs.Config))
	}
	if v, ok := params["doubleIn"].(bool); ok {
		gs.Config["doubleIn"] = v
	}
	if v, ok := params["doubleOut"].(bool); ok {
		gs.Config["doubleOut"] = v
	}
	if v, ok := params["orderMode"].(string); ok && v != "" {
		gs.Config["orderMode"] = v
	}
	if v, ok := params["beginnerRange"]; ok {
		gs.Config["beginnerRange"] = gameutil.CfgInt(map[string]any{"v": v}, "v", gs.beginnerRange())
	}
	if v, ok := params["requireFunDart"].(bool); ok {
		gs.Config["requireFunDart"] = v
	}
	if v, ok := params["visitHoldMs"]; ok {
		gs.Config["visitHoldMs"] = gameutil.CfgInt(map[string]any{"v": v}, "v", 1500)
	}
}

func (gs *GameState) seatOrder() []int {
	var order []int
	for _, p := range gs.seatNow() {
		order = append(order, p.RangeNum)
	}
	return order
}

func (gs *GameState) beginVisit(p *Player) {
	if p == nil {
		return
	}
	p.VisitStart = p.Remaining
	p.LastThree = nil
	p.OpenDarts = 3
	p.LastResult = ""
}

func (gs *GameState) pushVisit(p *Player, label string) {
	p.LastThree = append(p.LastThree, label)
	if len(p.LastThree) > 3 {
		p.LastThree = p.LastThree[len(p.LastThree)-3:]
	}
	p.OpenDarts = 3 - len(p.LastThree)
	if p.OpenDarts < 0 {
		p.OpenDarts = 0
	}
}

func (gs *GameState) finishVisit(p *Player, now time.Time) {
	if p == nil {
		return
	}
	p.OpenDarts = 0
	if p.Finished || gs.Phase == gameutil.PhaseFinished || gs.WinnerRange != 0 {
		gs.ActiveRange = 0
		gs.TurnPending = false
		gs.NextRange = 0
		gs.HoldUntil = time.Time{}
		return
	}
	gs.rememberPos(p.RangeNum)
	gs.NextRange = gs.peekNext()
	hold := gs.visitHold()
	if hold <= 0 {
		gs.advanceTurn()
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	gs.ActiveRange = 0
	gs.TurnPending = true
	gs.HoldUntil = now.Add(hold)
}

func (gs *GameState) rememberPos(rn int) {
	for i, n := range gs.Order {
		if n == rn {
			gs.CurrentPos = i
			return
		}
	}
}

func (gs *GameState) eligible(rn int) bool {
	p := gs.player(rn)
	return p != nil && p.Active && p.Seated && !p.Finished
}

func (gs *GameState) peekNext() int {
	if len(gs.Order) == 0 {
		return 0
	}
	for step := 1; step <= len(gs.Order); step++ {
		rn := gs.Order[(gs.CurrentPos+step)%len(gs.Order)]
		if gs.eligible(rn) {
			return rn
		}
	}
	return 0
}

func (gs *GameState) advanceTurn() {
	gs.TurnPending = false
	gs.HoldUntil = time.Time{}
	if len(gs.Order) == 0 {
		gs.ActiveRange = 0
		gs.NextRange = 0
		return
	}
	for step := 1; step <= len(gs.Order); step++ {
		pos := (gs.CurrentPos + step) % len(gs.Order)
		rn := gs.Order[pos]
		if !gs.eligible(rn) {
			continue
		}
		gs.CurrentPos = pos
		gs.ActiveRange = rn
		gs.beginVisit(gs.player(rn))
		gs.NextRange = gs.peekNext()
		return
	}
	gs.ActiveRange = 0
	gs.NextRange = 0
}

func (gs *GameState) acceptThrow(rangeNum int, now time.Time) bool {
	if gs.TurnPending {
		if rangeNum != gs.NextRange || gs.NextRange == 0 {
			return false
		}
		// The next lane's shot starts that visit immediately. The hold only
		// waits when nobody throws, so a dark hall does not stall the leg.
		gs.advanceTurn()
		return gs.ActiveRange == rangeNum
	}
	return rangeNum == gs.ActiveRange && gs.eligible(rangeNum)
}

func funDartShot(ctx logicapi.ShotContext) bool {
	for _, raw := range []string{ctx.Shot.DiscTypeRaw, ctx.Shot.DiscType, ctx.Live.DiscType} {
		if isFunDartID(raw) {
			return true
		}
	}
	return false
}

func isFunDartID(raw string) bool {
	n := strings.ToLower(strings.TrimSpace(raw))
	n = strings.NewReplacer("-", "", "_", "", " ", "").Replace(n)
	return n == "fundart" || n == "fun"
}

func (gs *GameState) applyCricket(p *Player, hit Hit, now time.Time) string {
	if p.CricketMarks == nil {
		p.CricketMarks = newCricketMarks()
	}
	gs.pushVisit(p, hit.Label)
	key, hits := cricketHit(hit)
	if key == "" {
		if len(p.LastThree) >= 3 {
			p.LastResult = "Runde"
			gs.finishVisit(p, now)
		}
		return "0"
	}
	before := p.CricketMarks[key]
	room := 3 - before
	if room < 0 {
		room = 0
	}
	p.CricketMarks[key] = before + hits
	if p.CricketMarks[key] > 3 {
		p.CricketMarks[key] = 3
	}
	points := 0
	if hits > room && gs.opponentsOpen(p.RangeNum, key) {
		points = (hits - room) * cricketValue(key)
	}
	p.CricketPoints += points
	p.LastResult = hit.Label
	if gs.cricketWon(p) {
		p.Finished = true
		p.OpenDarts = 0
		gs.finishVisit(p, now)
		return "Checkout"
	}
	if len(p.LastThree) >= 3 {
		gs.finishVisit(p, now)
	}
	if points == 0 {
		return hit.Label
	}
	return hit.Label
}

func cricketHit(hit Hit) (string, int) {
	switch hit.Points {
	case 50:
		return "B", 2
	case 25:
		return "B", 1
	}
	if hit.Number >= 15 && hit.Number <= 20 && hit.Multiplier > 0 {
		return strconv.Itoa(hit.Number), hit.Multiplier
	}
	return "", 0
}

func cricketValue(key string) int {
	if key == "B" {
		return 25
	}
	n, _ := strconv.Atoi(key)
	return n
}

func (gs *GameState) opponentsOpen(rangeNum int, key string) bool {
	open := false
	for _, other := range gs.Players {
		if other == nil || other.RangeNum == rangeNum || !other.Active || !other.Seated {
			continue
		}
		marks := 0
		if other.CricketMarks != nil {
			marks = other.CricketMarks[key]
		}
		if marks < 3 {
			open = true
		}
	}
	return open
}

func (gs *GameState) cricketWon(p *Player) bool {
	for _, key := range cricketTargets {
		if p.CricketMarks[key] < 3 {
			return false
		}
	}
	for _, other := range gs.Players {
		if other == nil || !other.Active || !other.Seated || other.RangeNum == p.RangeNum {
			continue
		}
		if p.CricketPoints < other.CricketPoints {
			return false
		}
	}
	return true
}

func (gs *GameState) applyClock(p *Player, hit Hit, now time.Time) string {
	if p.ClockTarget < 1 {
		p.ClockTarget = 1
	}
	gs.pushVisit(p, hit.Label)
	want := p.ClockTarget
	got := clockNumber(hit)
	if got == want {
		p.ClockTarget++
		p.LastResult = hit.Label
	} else {
		p.LastResult = "vorbei"
	}
	if p.ClockTarget > 21 {
		p.Finished = true
		p.OpenDarts = 0
		gs.finishVisit(p, now)
		return "Checkout"
	}
	if len(p.LastThree) >= 3 {
		gs.finishVisit(p, now)
	}
	if got == want {
		return hit.Label
	}
	return "0"
}

func clockNumber(hit Hit) int {
	if hit.Points == 50 || hit.Points == 25 {
		return 21
	}
	if hit.Number >= 1 && hit.Number <= 20 && hit.Points > 0 {
		return hit.Number
	}
	return 0
}

func clockTargetLabel(target int) string {
	if target > 21 {
		return "fertig"
	}
	if target == 21 {
		return "Bull"
	}
	if target < 1 {
		return "1"
	}
	return strconv.Itoa(target)
}

func (gs *GameState) marksLine(p *Player) string {
	if p == nil || p.CricketMarks == nil {
		return ""
	}
	parts := make([]string, 0, len(cricketTargets))
	for _, key := range cricketTargets {
		n := p.CricketMarks[key]
		if n > 3 {
			n = 3
		}
		parts = append(parts, fmt.Sprintf("%s:%d", key, n))
	}
	return strings.Join(parts, " ")
}
