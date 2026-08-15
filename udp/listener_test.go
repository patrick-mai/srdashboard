package udp

import (
	"fmt"
	"testing"
	"time"

	"srdashboard/state"
)

func TestHandlePacketShotDateTime(t *testing.T) {
	st := state.NewLiveState(1)
	var got state.Shot
	l, err := NewListener(0, st)
	if err != nil {
		t.Fatal(err)
	}
	l.SetShotNotifier(func(_ int, shot state.Shot, _ int) {
		got = shot
	})

	at := time.Date(2018, 8, 15, 18, 25, 43, 511000000, time.Local)
	data, err := BuildShotPacket(ShotPacketOpts{Range: 1, DecValue: 10.1, ShotAt: at})
	if err != nil {
		t.Fatal(err)
	}
	l.handlePacket(data)

	if got.At.IsZero() {
		t.Fatal("expected shot At from ShotDateTime")
	}
	if got.At.Format("2006-01-02 15:04:05.000") != "2018-08-15 18:25:43.511" {
		t.Fatalf("At=%s", got.At.Format("2006-01-02 15:04:05.000"))
	}
	if got.ReceivedAt.IsZero() {
		t.Fatal("expected ReceivedAt")
	}
}

func TestHandlePacketShotTimestampLegacyISO(t *testing.T) {
	st := state.NewLiveState(1)
	var got state.Shot
	l, err := NewListener(0, st)
	if err != nil {
		t.Fatal(err)
	}
	l.SetShotNotifier(func(_ int, shot state.Shot, _ int) {
		got = shot
	})

	x, y, d := PlaceShot(10.1, 1)
	payload := fmt.Sprintf(
		`{"MessageType":"Event","MessageVerb":"Shot","Ranges":1,"Objects":[{"X":%d,"Y":%d,"Distance":%.1f,"FullValue":10,"DecValue":10.1,"Range":1,"IsWarmup":false,"Timestamp":"2018-08-15T18:25:43.511Z"}]}`,
		x, y, d,
	)
	l.handlePacket([]byte(payload))

	if got.At.UTC().Format(time.RFC3339Nano) != "2018-08-15T18:25:43.511Z" {
		t.Fatalf("At=%s shots=%d", got.At.UTC().Format(time.RFC3339Nano), len(st.Snapshot()[0].Shots))
	}
}
