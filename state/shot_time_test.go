package state

import (
	"testing"
	"time"
)

func TestParseOpticScoreTimeDISAGShotDateTime(t *testing.T) {
	tm, ok := ParseOpticScoreTime("2018-08-15 18:25:43.511")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if tm.Format("2006-01-02 15:04:05.000") != "2018-08-15 18:25:43.511" {
		t.Fatalf("got %s", tm.Format("2006-01-02 15:04:05.000"))
	}
}

func TestParseOpticScoreTimeRFC3339(t *testing.T) {
	tm, ok := ParseOpticScoreTime("2018-08-15T18:25:43.511Z")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if tm.UTC().Format(time.RFC3339Nano) != "2018-08-15T18:25:43.511Z" {
		t.Fatalf("got %s", tm.UTC().Format(time.RFC3339Nano))
	}
}

func TestEventTimeFromFields(t *testing.T) {
	_, ok := EventTimeFromFields("", "  ", "not-a-time")
	if ok {
		t.Fatal("expected false")
	}
	tm, ok := EventTimeFromFields("", "2018-08-15T18:25:43.511Z")
	if !ok || tm.IsZero() {
		t.Fatal("expected parsed time")
	}
}

func TestShotEventTimePrefersAt(t *testing.T) {
	at := time.Date(2026, 6, 16, 12, 0, 0, 200_000_000, time.UTC)
	recv := at.Add(50 * time.Millisecond)
	got, ok := ShotEventTime(Shot{At: at, ReceivedAt: recv})
	if !ok || !got.Equal(at) {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}
