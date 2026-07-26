package state

import (
	"strings"
	"time"
)

// ParseOpticScoreTime parses DISAG OpticScore timestamp strings.
// Primary format per DISAG JSON Live: ShotDateTime as yyyy-MM-dd HH:mm:ss.fff (local range time).
func ParseOpticScoreTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	dsgLayouts := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
	}
	for _, layout := range dsgLayouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, true
		}
	}
	isoLayouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// EventTimeFromFields returns the first parseable OpticScore time field.
func EventTimeFromFields(values ...string) (time.Time, bool) {
	for _, v := range values {
		if t, ok := ParseOpticScoreTime(v); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// ShotEventTime picks OpticScore time from a shot, preferring device timestamp over server receive.
func ShotEventTime(shot Shot) (time.Time, bool) {
	if !shot.At.IsZero() {
		return shot.At, true
	}
	if !shot.ReceivedAt.IsZero() {
		return shot.ReceivedAt, true
	}
	return time.Time{}, false
}

// FormatOpticScoreTime formats t as DISAG ShotDateTime (yyyy-MM-dd HH:mm:ss.fff, local).
func FormatOpticScoreTime(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02 15:04:05.000")
}
