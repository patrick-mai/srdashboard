package qrformat

import (
	"time"

	"srdashboard/state"
)

// ShotInput is one shot for QR export (domain-neutral).
type ShotInput struct {
	X        int
	Y        int
	DecValue float64
	Distance float64
	At       time.Time
	IsWarmup bool
}

// ResultInput is the full result snapshot needed by format encoders.
type ResultInput struct {
	RangeNum   int
	ShooterName string
	ClubName   string
	Discipline string
	DiscType   string
	IsWarmup   bool
	WarmupShots []ShotInput
	Series     [][]ShotInput // completed competition series (10 each)
	OpenShots  []ShotInput   // current incomplete series on the target
}

// FromRangeSnapshot maps live state into ResultInput for encoders.
func FromRangeSnapshot(snap state.RangeSnapshot) ResultInput {
	in := ResultInput{
		RangeNum:    snap.RangeNum,
		ShooterName: snap.ShooterName,
		ClubName:    snap.ClubName,
		Discipline:  snap.Discipline,
		DiscType:    snap.DiscType,
		IsWarmup:    snap.IsWarmup,
		WarmupShots: mapShots(snap.WarmupShots),
	}
	if snap.IsWarmup {
		// Still in Probe: SeriesShots would also be warmup — WarmupShots already has the flat list.
		return in
	}
	in.Series = make([][]ShotInput, 0, len(snap.SeriesShots))
	for _, ser := range snap.SeriesShots {
		in.Series = append(in.Series, mapShots(ser))
	}
	if len(snap.Shots) > 0 {
		in.OpenShots = mapShots(snap.Shots)
	}
	return in
}

func mapShots(in []state.Shot) []ShotInput {
	if len(in) == 0 {
		return nil
	}
	out := make([]ShotInput, len(in))
	for i, s := range in {
		out[i] = ShotInput{
			X:        s.X,
			Y:        s.Y,
			DecValue: s.DecValue,
			Distance: s.Distance,
			At:        s.At,
			IsWarmup: s.IsWarmup,
		}
	}
	return out
}

// HasExportableShots reports whether the result has anything worth encoding.
func (r ResultInput) HasExportableShots() bool {
	if len(r.WarmupShots) > 0 || len(r.OpenShots) > 0 {
		return true
	}
	for _, s := range r.Series {
		if len(s) > 0 {
			return true
		}
	}
	return false
}
