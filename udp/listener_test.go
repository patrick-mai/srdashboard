package udp

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"srdashboard/state"
)

func TestHandlePacketShotDateTime(t *testing.T) {
	st := state.NewLiveState(1)
	var got state.Shot
	l, err := NewListener(0, st, "")
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
	l, err := NewListener(0, st, "")
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

type denyAll struct{}

func (denyAll) AllowShot(int) bool { return false }

func TestForwardSendsRawBeforeFilter(t *testing.T) {
	recv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer recv.Close()
	_ = recv.SetReadDeadline(time.Now().Add(3 * time.Second))

	st := state.NewLiveState(2)
	src, err := NewListener(0, st, recv.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	src.SetShotFilter(denyAll{})
	src.Start()
	defer src.Stop()
	time.Sleep(50 * time.Millisecond)

	data, err := BuildShotPacket(ShotPacketOpts{Range: 1, DecValue: 10.1})
	if err != nil {
		t.Fatal(err)
	}
	if err := SendRawPacket(src.LocalAddr().String(), data); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 65535)
	n, _, err := recv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("downstream did not receive forwarded datagram: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Fatalf("forwarded %q, want original %q", buf[:n], data)
	}
	if n := st.ShotNumber(1); n != 0 {
		t.Fatalf("filter should drop locally, shotNumber=%d", n)
	}
}
