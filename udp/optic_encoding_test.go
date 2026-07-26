package udp

import (
	"testing"

	"srdashboard/state"
)

func TestNormalizeOpticScoreJSON_UTF8Passthrough(t *testing.T) {
	in := []byte(`{"Shooter":{"Lastname":"Hütte"}}`)
	out := normalizeOpticScoreJSON(in)
	if string(out) != string(in) {
		t.Fatalf("utf8 changed: %q -> %q", in, out)
	}
}

func TestNormalizeOpticScoreJSON_CP1252Umlauts(t *testing.T) {
	// "Nölle" / "Hütte" with Windows-1252 ö (0xF6) and ü (0xFC)
	in := []byte{'"', 'N', 0xF6, 'l', 'l', 'e', '"', ' ', '"', 'H', 0xFC, 't', 't', 'e', '"'}
	out := normalizeOpticScoreJSON(in)
	if string(out) != `"Nölle" "Hütte"` {
		t.Fatalf("got %q", out)
	}
}

func TestHandlePacket_CP1252ShooterName(t *testing.T) {
	st := state.NewLiveState(1)
	l, err := NewListener(0, st)
	if err != nil {
		t.Fatal(err)
	}

	// Minimal Shot envelope; Lastname uses CP1252 ö (0xF6)
	payload := append([]byte(`{"MessageType":"Event","MessageVerb":"Shot","Ranges":1,"Objects":[{"X":1,"Y":2,"Distance":0.5,"FullValue":10,"DecValue":10.1,"Range":1,"IsWarmup":false,"Shooter":{"Firstname":"Christoph","Lastname":"N`), 0xF6)
	payload = append(payload, []byte(`lle"}}]}`)...)

	l.handlePacket(payload)

	snap := st.Snapshot()
	if len(snap) < 1 {
		t.Fatal("no range state")
	}
	name := snap[0].ShooterName
	if name != "Christoph Nölle" {
		t.Fatalf("ShooterName=%q want %q", name, "Christoph Nölle")
	}
	for _, r := range name {
		if r == '\uFFFD' {
			t.Fatalf("name still contains replacement char: %q", name)
		}
	}
}
