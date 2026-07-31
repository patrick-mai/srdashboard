package state

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shot represents a single shot on the target
type Shot struct {
	X          int       `json:"x"`
	Y          int       `json:"y"`
	Distance   float64   `json:"distance"`
	FullValue  int       `json:"fullValue"`
	DecValue   float64   `json:"decValue"`
	IsWarmup   bool      `json:"isWarmup"`
	At         time.Time `json:"at,omitempty"`         // OpticScore ShotDateTime when available
	ReceivedAt time.Time `json:"receivedAt,omitempty"` // Server time when UDP packet was handled
}

// RangeState holds the live state for one shooting range
type RangeState struct {
	RangeNum         int       `json:"rangeNum"`
	ShooterName      string    `json:"shooterName"`
	ClubName         string    `json:"clubName"`
	Discipline       string    `json:"discipline"`
	DiscType         string    `json:"discType"`
	IsWarmup         bool      `json:"isWarmup"`
	Shots            []Shot    `json:"shots"`
	ShotNumber       int       `json:"shotNumber"`
	CurrentValue     float64   `json:"currentValue"`
	CurrentTeiler    float64   `json:"currentTeiler"`
	BestTeiler       float64   `json:"bestTeiler"`
	BestTeilerShot   int       `json:"bestTeilerShot"` // 0 = none yet; otherwise shot number of best Teiler
	OverallSumInt    int       `json:"overallSumInt"`
	OverallSumDec    float64   `json:"overallSumDecimal"`
	SeriesSumsInt    []int     `json:"seriesSumsInt"`
	SeriesSums       []float64 `json:"seriesSums"`
	Last10Values     []float64 `json:"last10Values"`
	TotalShotsToFire int       `json:"totalShotsToFire"`
}

// LiveState holds state for all ranges. Safe for concurrent use: UDP applies shots under mu, HTTP reads via Snapshot().
type LiveState struct {
	mu     sync.RWMutex
	Ranges map[int]*RangeState
}

// NewLiveState creates a new LiveState with the given number of ranges
func NewLiveState(numRanges int) *LiveState {
	rs := make(map[int]*RangeState)
	for i := 1; i <= numRanges; i++ {
		rs[i] = emptyRangeState(i)
	}
	return &LiveState{Ranges: rs}
}

// SetNumRanges grows or shrinks the live range map to match config.
// Existing ranges keep their state; new ranges start empty; removed ranges are dropped.
func (ls *LiveState) SetNumRanges(numRanges int) {
	if numRanges < 1 {
		numRanges = 1
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for i := 1; i <= numRanges; i++ {
		if _, ok := ls.Ranges[i]; !ok {
			ls.Ranges[i] = emptyRangeState(i)
		}
	}
	for k := range ls.Ranges {
		if k < 1 || k > numRanges {
			delete(ls.Ranges, k)
		}
	}
}

func emptyRangeState(rangeNum int) *RangeState {
	return &RangeState{
		RangeNum:      rangeNum,
		Shots:         make([]Shot, 0, 10),
		SeriesSumsInt: make([]int, 0),
		SeriesSums:    make([]float64, 0),
		Last10Values:  make([]float64, 0, 10),
	}
}

// ResetRange restores one range to the empty default (no shooter, shots, or sums).
func (ls *LiveState) ResetRange(rng int) bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if _, ok := ls.Ranges[rng]; !ok {
		return false
	}
	ls.Ranges[rng] = emptyRangeState(rng)
	return true
}

// ReplaceRange overwrites one range from a snapshot (e.g. restore after restart).
func (ls *LiveState) ReplaceRange(snap RangeSnapshot) bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if _, ok := ls.Ranges[snap.RangeNum]; !ok {
		return false
	}
	ls.Ranges[snap.RangeNum] = &RangeState{
		RangeNum:         snap.RangeNum,
		ShooterName:      snap.ShooterName,
		ClubName:         snap.ClubName,
		Discipline:       snap.Discipline,
		DiscType:         snap.DiscType,
		IsWarmup:         snap.IsWarmup,
		Shots:            append([]Shot(nil), snap.Shots...),
		ShotNumber:       snap.ShotNumber,
		CurrentValue:     snap.CurrentValue,
		CurrentTeiler:    snap.CurrentTeiler,
		BestTeiler:       snap.BestTeiler,
		BestTeilerShot:   snap.BestTeilerShot,
		OverallSumInt:    snap.OverallSumInt,
		OverallSumDec:    snap.OverallSumDec,
		SeriesSumsInt:    append([]int(nil), snap.SeriesSumsInt...),
		SeriesSums:       append([]float64(nil), snap.SeriesSums...),
		Last10Values:     append([]float64(nil), snap.Last10Values...),
		TotalShotsToFire: snap.TotalShotsToFire,
	}
	return true
}

// ShotPayload is the parsed DISAG OpticScore shot from JSON
type ShotPayload struct {
	X            int     `json:"X"`
	Y            int     `json:"Y"`
	Distance     float64 `json:"Distance"`
	FullValue    int     `json:"FullValue"`
	DecValue     float64 `json:"DecValue"`
	Range        int     `json:"Range"`
	IsWarmup     bool    `json:"IsWarmup"`
	IsHot        bool    `json:"IsHot"`
	ShotDateTime string  `json:"ShotDateTime"` // DISAG: yyyy-MM-dd HH:mm:ss.fff
	// TLStatus / LastTLChange exist in DISAG but are not available on all ranges — not used by game logic.
	TLStatus     string `json:"TLStatus"`
	LastTLChange int    `json:"LastTLChange"`
	// Legacy/alternate timestamp keys seen in some exports.
	Timestamp string `json:"Timestamp"`
	DateTime  string `json:"DateTime"`
	Time      string `json:"Time"`
	DATETIME  string `json:"DATETIME"`
	Shooter   *struct {
		Firstname string `json:"Firstname"`
		Lastname  string `json:"Lastname"`
		Club      *struct {
			Name string `json:"Name"`
		} `json:"Club"`
	} `json:"Shooter"`
	// DiscType is OpticScore's short discipline code (e.g. LG, LP, KK).
	DiscType    string `json:"DiscType"`
	DiscTypeRaw string `json:"DiscTypeRaw"`
	MenuItem    *struct {
		MenuPointName string `json:"MenuPointName"`
		MenuItemName  string `json:"MenuItemName"`
	} `json:"MenuItem"`
}

// EventTime returns the OpticScore timestamp on this shot object, if present.
func (sp *ShotPayload) EventTime() (time.Time, bool) {
	if sp == nil {
		return time.Time{}, false
	}
	return EventTimeFromFields(sp.ShotDateTime, sp.Timestamp, sp.DateTime, sp.Time, sp.DATETIME)
}

var menuItemShotCountRe = regexp.MustCompile(`(\d+)\s*Schuss`)

// resetRangeFooter clears target and all footer stats for a range (e.g. new shooter or warmup->competition).
func resetRangeFooter(rs *RangeState) {
	rs.Shots = nil
	rs.ShotNumber = 0
	rs.SeriesSumsInt = nil
	rs.SeriesSums = nil
	rs.Last10Values = nil
	rs.OverallSumInt = 0
	rs.OverallSumDec = 0
	rs.CurrentValue = 0
	rs.CurrentTeiler = 0
	rs.BestTeiler = 0
	rs.BestTeilerShot = 0
	rs.TotalShotsToFire = 0
}

// ParseTotalShotsFromMenuItem extracts total shots from MenuItemName (e.g. "40 Schuss" -> 40).
// If the name contains "unbegrenzt" (unlimited) and no number, returns 100.
func ParseTotalShotsFromMenuItem(name string) int {
	if name == "" {
		return 0
	}
	matches := menuItemShotCountRe.FindStringSubmatch(name)
	if len(matches) >= 2 {
		n, _ := strconv.Atoi(matches[1])
		return n
	}
	if strings.Contains(strings.ToLower(name), "unbegrenzt") {
		return 100
	}
	return 0
}

// disciplineLabelFromShot prefers MenuPointName (e.g. "KK-Gewehr", "Luftgewehr").
// MenuItemName is ignored when it is only a shot-count label ("10 Schuss").
// Falls back to DiscType / DiscTypeRaw (LG, LP, KK).
func disciplineLabelFromShot(sp *ShotPayload) string {
	if sp == nil {
		return ""
	}
	if sp.MenuItem != nil {
		if sp.MenuItem.MenuPointName != "" {
			return sp.MenuItem.MenuPointName
		}
		if name := strings.TrimSpace(sp.MenuItem.MenuItemName); name != "" {
			if ParseTotalShotsFromMenuItem(name) == 0 && !strings.Contains(strings.ToLower(name), "schuss") {
				return name
			}
		}
	}
	if sp.DiscType != "" {
		return sp.DiscType
	}
	return sp.DiscTypeRaw
}

// maxSeriesSums bounds the per-range series history. A 100-series range day is
// already 1000 shots; beyond that the oldest series drop off.
const maxSeriesSums = 100

// ApplyShot updates range state with a new shot, timestamped from the payload
// when it carries a ShotDateTime. Call from UDP handler only.
// Reports whether the range exists.
func (ls *LiveState) ApplyShot(rng int, sp *ShotPayload) bool {
	at, _ := sp.EventTime()
	return ls.ApplyShotAt(rng, sp, at, time.Now())
}

// ApplyShotAt is ApplyShot with explicit timestamps, for callers that resolve
// the shot time from the enclosing UDP message.
func (ls *LiveState) ApplyShotAt(rng int, sp *ShotPayload, at, receivedAt time.Time) bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	rs, ok := ls.Ranges[rng]
	if !ok {
		return false
	}

	shot := Shot{
		X:          sp.X,
		Y:          sp.Y,
		Distance:   sp.Distance,
		FullValue:  sp.FullValue,
		DecValue:   sp.DecValue,
		IsWarmup:   sp.IsWarmup,
		At:         at,
		ReceivedAt: receivedAt,
	}

	// Mode switch: Warmup -> Competition clears target and resets footer
	wasWarmup := rs.IsWarmup
	rs.IsWarmup = sp.IsWarmup
	if wasWarmup && !sp.IsWarmup {
		resetRangeFooter(rs)
	}

	// Shooter change: reset footer and target for the new shooter
	var newShooterName string
	if sp.Shooter != nil {
		newShooterName = sp.Shooter.Firstname + " " + sp.Shooter.Lastname
		if newShooterName != rs.ShooterName {
			rs.ShooterName = newShooterName
			resetRangeFooter(rs)
		} else {
			rs.ShooterName = newShooterName
		}
		if sp.Shooter.Club != nil && sp.Shooter.Club.Name != "" {
			rs.ClubName = sp.Shooter.Club.Name
		}
	}

	// Parse total shots from MenuItemName (e.g. "40 Schuss"); prefer MenuPointName for discipline
	// (e.g. "KK-Gewehr") so target mapping is not stuck on shot-count labels like "10 Schuss".
	// DiscType (LG/LP/KK) is the OpticScore short code and is always kept for face resolution.
	if sp.DiscType != "" {
		rs.DiscType = sp.DiscType
	} else if sp.DiscTypeRaw != "" {
		rs.DiscType = sp.DiscTypeRaw
	}
	if sp.MenuItem != nil {
		if n := ParseTotalShotsFromMenuItem(sp.MenuItem.MenuItemName); n > 0 {
			rs.TotalShotsToFire = n
		}
	}
	if d := disciplineLabelFromShot(sp); d != "" {
		rs.Discipline = d
	}

	rs.ShotNumber++
	rs.CurrentValue = sp.DecValue
	rs.CurrentTeiler = sp.Distance
	// Best Teiler = lowest Distance (closest to centre) across the series.
	if rs.BestTeilerShot == 0 || sp.Distance < rs.BestTeiler {
		rs.BestTeiler = sp.Distance
		rs.BestTeilerShot = rs.ShotNumber
	}
	rs.OverallSumInt += sp.FullValue
	rs.OverallSumDec += sp.DecValue

	// If we already have 10 shots, this shot is the first of a new series: clear target first.
	// Series sums are now calculated when the last shot of a series is placed, not during target cleanup.
	if len(rs.Shots) == 10 {
		rs.Shots = nil
	}
	rs.Shots = append(rs.Shots, shot)

	// After placing the shot, if we have exactly 10 shots on the target, record the series sums.
	if len(rs.Shots) == 10 {
		var sumInt int
		var sumDec float64
		for _, s := range rs.Shots {
			sumInt += s.FullValue
			sumDec += s.DecValue
		}
		rs.SeriesSumsInt = appendCapped(rs.SeriesSumsInt, sumInt, maxSeriesSums)
		rs.SeriesSums = appendCapped(rs.SeriesSums, sumDec, maxSeriesSums)
	}

	// Last 10 values
	rs.Last10Values = appendCapped(rs.Last10Values, sp.DecValue, 10)
	return true
}

// appendCapped appends v, dropping the oldest entries past max.
func appendCapped[T any](s []T, v T, max int) []T {
	s = append(s, v)
	if len(s) > max {
		s = append(s[:0], s[len(s)-max:]...)
	}
	return s
}

// Prediction returns the extrapolated totals to match the sum display (integer sum / decimal sum).
// Same format as Summe: first = predicted integer (ring) sum, second = predicted decimal sum.
// Formula: predInt = (overallSumInt / shotsFired) * totalShotsToFire, predDec = (overallSumDecimal / shotsFired) * totalShotsToFire.
func (rs *RangeState) Prediction() (int, float64) {
	if rs.ShotNumber == 0 || rs.TotalShotsToFire == 0 {
		return 0, 0
	}
	n := float64(rs.ShotNumber)
	t := float64(rs.TotalShotsToFire)
	predInt := (float64(rs.OverallSumInt) / n) * t
	predDec := (rs.OverallSumDec / n) * t
	return int(predInt), predDec
}

// RangeSnapshot is a copy of one range's state for safe use by HTTP handlers.
type RangeSnapshot struct {
	RangeNum         int       `json:"rangeNum"`
	ShooterName      string    `json:"shooterName"`
	ClubName         string    `json:"clubName"`
	Discipline       string    `json:"discipline"`
	DiscType         string    `json:"discType"`
	IsWarmup         bool      `json:"isWarmup"`
	Shots            []Shot    `json:"shots"`
	ShotNumber       int       `json:"shotNumber"`
	CurrentValue     float64   `json:"currentValue"`
	CurrentTeiler    float64   `json:"currentTeiler"`
	BestTeiler       float64   `json:"bestTeiler"`
	BestTeilerShot   int       `json:"bestTeilerShot"`
	OverallSumInt    int       `json:"overallSumInt"`
	OverallSumDec    float64   `json:"overallSumDecimal"`
	PredictionInt    int       `json:"predictionInt"`
	PredictionDec    float64   `json:"predictionDecimal"`
	SeriesSumsInt    []int     `json:"seriesSumsInt"`
	SeriesSums       []float64 `json:"seriesSums"`
	Last10Values     []float64 `json:"last10Values"`
	TotalShotsToFire int       `json:"totalShotsToFire"`
}

// Snapshot returns a consistent copy of all ranges for API responses. Safe to call from HTTP handlers.
func (ls *LiveState) Snapshot() []RangeSnapshot {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	keys := make([]int, 0, len(ls.Ranges))
	for k := range ls.Ranges {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]RangeSnapshot, 0, len(keys))
	for _, k := range keys {
		rs := ls.Ranges[k]
		predInt, predDec := rs.Prediction()
		// Copy slices so caller can use result after unlock
		shots := append([]Shot(nil), rs.Shots...)
		seriesSumsInt := append([]int(nil), rs.SeriesSumsInt...)
		seriesSums := append([]float64(nil), rs.SeriesSums...)
		last10 := append([]float64(nil), rs.Last10Values...)
		out = append(out, RangeSnapshot{
			RangeNum:         rs.RangeNum,
			ShooterName:      rs.ShooterName,
			ClubName:         rs.ClubName,
			Discipline:       rs.Discipline,
			DiscType:         rs.DiscType,
			IsWarmup:         rs.IsWarmup,
			Shots:            shots,
			ShotNumber:       rs.ShotNumber,
			CurrentValue:     rs.CurrentValue,
			CurrentTeiler:    rs.CurrentTeiler,
			BestTeiler:       rs.BestTeiler,
			BestTeilerShot:   rs.BestTeilerShot,
			OverallSumInt:    rs.OverallSumInt,
			OverallSumDec:    rs.OverallSumDec,
			PredictionInt:    predInt,
			PredictionDec:    predDec,
			SeriesSumsInt:    seriesSumsInt,
			SeriesSums:       seriesSums,
			Last10Values:     last10,
			TotalShotsToFire: rs.TotalShotsToFire,
		})
	}
	return out
}
