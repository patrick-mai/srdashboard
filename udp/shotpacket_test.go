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

func TestBuildShotPacketClubAndTeam(t *testing.T) {
	data, err := BuildShotPacket(ShotPacketOpts{
		Range: 1, DecValue: 10.2, Shooter: "Anna", Lastname: "Müller",
		Club: "SV Adler", Team: "Adler I", MenuItem: "LG 40 Schuss",
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	var shot state.ShotPayload
	if err := json.Unmarshal(msg.Objects[0], &shot); err != nil {
		t.Fatal(err)
	}
	if shot.Shooter == nil || shot.Shooter.Lastname != "Müller" {
		t.Fatalf("name %#v", shot.Shooter)
	}
	if shot.Shooter.Club == nil || shot.Shooter.Club.Name != "SV Adler" {
		t.Fatalf("club %#v", shot.Shooter.Club)
	}
	if shot.Shooter.Team == nil || shot.Shooter.Team.Name != "Adler I" {
		t.Fatalf("team %#v", shot.Shooter.Team)
	}
	if shot.MenuItem == nil || shot.MenuItem.MenuItemName != "LG 40 Schuss" {
		t.Fatalf("menu %#v", shot.MenuItem)
	}
}

func TestBuildShotPacketLPUsesPistolBand(t *testing.T) {
	x, y, d := PlaceShotForDisc("LP", 9.6, 7)
	data, err := BuildShotPacket(ShotPacketOpts{
		Range: 1, X: x, Y: y, Distance: d, DecValue: 9.6, DiscType: "LP", MenuItem: "LP 40 Schuss",
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	var shot state.ShotPayload
	if err := json.Unmarshal(msg.Objects[0], &shot); err != nil {
		t.Fatal(err)
	}
	if err := ValidateShot(&shot); err != nil {
		t.Fatalf("LP packet invalid: %v D=%.1f", err, shot.Distance)
	}
	if shot.DiscType != "LP" {
		t.Fatalf("disc %q", shot.DiscType)
	}
}
