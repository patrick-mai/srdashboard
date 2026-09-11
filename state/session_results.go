package state

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxSessionArchive = 200

type archivedSession struct {
	ID         string
	ArchivedAt time.Time
	Snap       RangeSnapshot
}

// SessionResultSummary is one start in the current server session (live or frozen).
type SessionResultSummary struct {
	ID               string     `json:"id"`
	Live             bool       `json:"live"`
	RangeNum         int        `json:"rangeNum"`
	ShooterName      string     `json:"shooterName"`
	ClubName         string     `json:"clubName"`
	TeamName         string     `json:"teamName"`
	Discipline       string     `json:"discipline"`
	DiscType         string     `json:"discType"`
	IsWarmup         bool       `json:"isWarmup"`
	ShotNumber       int        `json:"shotNumber"`
	OverallSumInt    int        `json:"overallSumInt"`
	OverallSumDec    float64    `json:"overallSumDecimal"`
	TotalShotsToFire int        `json:"totalShotsToFire"`
	StartedAt        time.Time  `json:"startedAt,omitempty"`
	ArchivedAt       *time.Time `json:"archivedAt,omitempty"`
}

func liveSessionID(rangeNum int) string {
	return "live-" + strconv.Itoa(rangeNum)
}

func parseLiveSessionID(id string) (int, bool) {
	if !strings.HasPrefix(id, "live-") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "live-"))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func rangeHasResult(rs *RangeState) bool {
	if rs == nil {
		return false
	}
	return rs.ShotNumber > 0 || len(rs.WarmupShots) > 0 || len(rs.Shots) > 0
}

func summaryFromSnap(id string, live bool, archivedAt time.Time, snap RangeSnapshot) SessionResultSummary {
	info := SessionResultSummary{
		ID:               id,
		Live:             live,
		RangeNum:         snap.RangeNum,
		ShooterName:      snap.ShooterName,
		ClubName:         snap.ClubName,
		TeamName:         snap.TeamName,
		Discipline:       snap.Discipline,
		DiscType:         snap.DiscType,
		IsWarmup:         snap.IsWarmup,
		ShotNumber:       snap.ShotNumber,
		OverallSumInt:    snap.OverallSumInt,
		OverallSumDec:    snap.OverallSumDec,
		TotalShotsToFire: snap.TotalShotsToFire,
		StartedAt:        snap.StartedAt,
	}
	if !live && !archivedAt.IsZero() {
		t := archivedAt
		info.ArchivedAt = &t
	}
	return info
}

// archiveRangeLocked freezes a result before the live lane is wiped. Caller holds ls.mu.
func archiveRangeLocked(ls *LiveState, rs *RangeState) {
	if ls == nil || !rangeHasResult(rs) {
		return
	}
	ls.nextArchive++
	entry := archivedSession{
		ID:         strconv.Itoa(ls.nextArchive),
		ArchivedAt: time.Now(),
		Snap:       snapshotFromRange(rs),
	}
	ls.archive = append(ls.archive, entry)
	if len(ls.archive) > maxSessionArchive {
		ls.archive = append([]archivedSession(nil), ls.archive[len(ls.archive)-maxSessionArchive:]...)
	}
}

// SessionResults lists current occupants with shots, then frozen starts (newest last in archive, shown newest-first).
func (ls *LiveState) SessionResults() []SessionResultSummary {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	keys := make([]int, 0, len(ls.Ranges))
	for k := range ls.Ranges {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]SessionResultSummary, 0, len(keys)+len(ls.archive))
	for _, k := range keys {
		rs := ls.Ranges[k]
		if !rangeHasResult(rs) {
			continue
		}
		snap := snapshotFromRange(rs)
		out = append(out, summaryFromSnap(liveSessionID(k), true, time.Time{}, snap))
	}
	for i := len(ls.archive) - 1; i >= 0; i-- {
		a := ls.archive[i]
		out = append(out, summaryFromSnap(a.ID, false, a.ArchivedAt, a.Snap))
	}
	return out
}

// SessionResult returns one live or frozen start.
func (ls *LiveState) SessionResult(id string) (SessionResultSummary, RangeSnapshot, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionResultSummary{}, RangeSnapshot{}, false
	}
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if n, ok := parseLiveSessionID(id); ok {
		rs, exists := ls.Ranges[n]
		if !exists || !rangeHasResult(rs) {
			return SessionResultSummary{}, RangeSnapshot{}, false
		}
		snap := snapshotFromRange(rs)
		return summaryFromSnap(id, true, time.Time{}, snap), snap, true
	}
	for i := range ls.archive {
		if ls.archive[i].ID == id {
			a := ls.archive[i]
			return summaryFromSnap(a.ID, false, a.ArchivedAt, a.Snap), a.Snap, true
		}
	}
	return SessionResultSummary{}, RangeSnapshot{}, false
}
