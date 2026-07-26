package udp

import (
	"encoding/json"
	"testing"
	"time"

	"srdashboard/state"
)

func TestBuildShotPacketDISAGShape(t *testing.T) {
	at := time.Date(2026, 6, 17, 14, 30, 0, 200_000_000, time.Local)
	data, err := BuildShotPacket(ShotPacketOpts{
		Range: 2, X: 50, Y: -30, DecValue: 9.8, ShotAt: at, Shooter: "Tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.MessageType != "Event" || msg.MessageVerb != "Shot" || msg.Ranges != 2 {
		t.Fatalf("envelope: %+v", msg)
	}
	var shot state.ShotPayload
	if err := json.Unmarshal(msg.Objects[0], &shot); err != nil {
		t.Fatal(err)
	}
	if shot.Range != 2 || shot.DecValue != 9.8 || shot.FullValue != 9 {
		t.Fatalf("shot: %+v", shot)
	}
	parsed, ok := shot.EventTime()
	if !ok || !parsed.Equal(at) {
		t.Fatalf("ShotDateTime: got %v ok=%v", parsed, ok)
	}
}
