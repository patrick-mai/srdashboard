package recovery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"srdashboard/state"
	"srdashboard/udp"
)

// Same format as srdashboard.exe stdout and log/<start>.txt.
const sessionConsoleLog = `
2026/09/07 11:13:08 session log: log\2026-09-07_11-13-08.txt
2026/09/07 11:13:08 UDP listener started on port 30169
2026/09/07 11:15:05 UDP: shot applied range=1 X=0 Y=0 DecValue=10.9 at=-
2026/09/07 11:15:05 UDP: shot applied range=1 X=17 Y=19 DecValue=10.8 at=-
2026/09/07 11:15:05 UDP: shot applied range=1 X=34 Y=37 DecValue=10.7 at=-
2026/09/07 11:15:06 UDP: shot applied range=1 X=80 Y=0 DecValue=10.6 at=-
2026/09/07 11:15:06 UDP: shot applied range=1 X=68 Y=73 DecValue=10.5 at=-
2026/09/07 11:15:06 UDP: shot applied range=1 X=86 Y=92 DecValue=10.4 at=-
2026/09/07 11:15:06 UDP: shot applied range=1 X=103 Y=110 DecValue=10.3 at=-
2026/09/07 11:15:07 UDP: shot applied range=1 X=120 Y=128 DecValue=10.2 at=-
2026/09/07 11:15:07 UDP: shot applied range=1 X=137 Y=147 DecValue=10.1 at=-
2026/09/07 11:15:07 UDP: shot applied range=1 X=154 Y=165 DecValue=10.0 at=-
`

func TestUserConsoleLogReplay(t *testing.T) {
	lines := ParseLog(sessionConsoleLog)
	if len(lines) != 10 {
		t.Fatalf("parsed %d shot lines, want 10", len(lines))
	}

	st := state.NewLiveState(6)
	notified := 0
	p := udp.Pipeline{State: st, OnShot: func(int, state.Shot, int) { notified++ }}
	applied := 0
	for _, pkt := range lines {
		applied += p.IngestReplay(pkt)
	}
	if applied != 10 {
		t.Fatalf("applied %d shots, want 10", applied)
	}
	if notified != 10 {
		t.Fatalf("replay notified %d times, want 10 (games consume recovered shots)", notified)
	}
	got := st.Snapshot()[0]
	if len(got.Shots) != 10 {
		t.Fatalf("range 1 has %d shots, want 10", len(got.Shots))
	}
	if got.Shots[0].X != 0 || got.Shots[0].Y != 0 || got.Shots[0].DecValue != 10.9 {
		t.Fatalf("first shot = %+v", got.Shots[0])
	}
	if got.Shots[9].X != 154 || got.Shots[9].Y != 165 || got.Shots[9].DecValue != 10.0 {
		t.Fatalf("last shot = %+v", got.Shots[9])
	}
	if got.Shots[0].At.IsZero() {
		t.Fatal("console log prefix time should be restored onto the shot")
	}
	wantAt := time.Date(2026, 9, 7, 11, 15, 5, 0, time.Local)
	if !got.Shots[0].At.Equal(wantAt) {
		t.Fatalf("first shot At=%s want %s", got.Shots[0].At, wantAt)
	}
}

func TestConsoleReplayDropsInconsistentKeepsKKBand(t *testing.T) {
	text := strings.Join([]string{
		"UDP: shot applied range=1 X=800 Y=-200 DecValue=10.3 at=-",
		"UDP: shot applied range=2 X=-274 Y=468 DecValue=10.3 at=-",
	}, "\n")
	lines := ParseLog(text)
	if len(lines) != 2 {
		t.Fatalf("parsed %d lines, want 2", len(lines))
	}
	st := state.NewLiveState(6)
	p := udp.Pipeline{State: st}
	applied := 0
	for _, pkt := range lines {
		applied += p.IngestReplay(pkt)
	}
	if applied != 1 {
		t.Fatalf("applied %d, want 1 (inconsistent 10.3 at 7-ring coords dropped)", applied)
	}
	if n := len(st.Snapshot()[0].Shots); n != 0 {
		t.Fatalf("range 1 leaked %d inconsistent shots", n)
	}
	r2 := st.Snapshot()[1]
	if len(r2.Shots) != 1 {
		t.Fatalf("range 2 shots=%d, want KK-band hole kept", len(r2.Shots))
	}
	if r2.Shots[0].X != -274 || r2.Shots[0].Y != 468 {
		t.Fatalf("KK coordinates rewritten: %+v", r2.Shots[0])
	}
}

func TestConsoleShotOmitsDiscType(t *testing.T) {
	pkt, ok := consoleShot(1, 17, 19, 10.8, time.Time{})
	if !ok {
		t.Fatal("consoleShot failed")
	}
	if strings.Contains(string(pkt), `"DiscType"`) {
		t.Fatalf("DiscType must be omitted so validation can try LG and LP/KK bands: %s", pkt)
	}
	var msg map[string]any
	if err := json.Unmarshal(pkt, &msg); err != nil {
		t.Fatal(err)
	}
}

func TestParseLog_ConsoleAtTimestamp(t *testing.T) {
	text := `2026/09/01 19:21:30 UDP: shot applied range=2 X=68 Y=73 DecValue=10.5 at=2026-09-01T19:21:30+02:00`
	lines := ParseLog(text)
	if len(lines) != 1 {
		t.Fatalf("got %d packets", len(lines))
	}
	st := state.NewLiveState(6)
	p := udp.Pipeline{State: st}
	if p.IngestReplay(lines[0]) != 1 {
		t.Fatal("valid console shot dropped by validation")
	}
	got := st.Snapshot()[1].Shots[0]
	if got.X != 68 || got.Y != 73 {
		t.Fatalf("coords %+v", got)
	}
	if got.At.IsZero() {
		t.Fatal("at= timestamp not restored")
	}
}
