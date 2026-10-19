package rwkbericht

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"srdashboard/wettkampf"
)

func TestMatchVereinAndDisziplin(t *testing.T) {
	if got := MatchVerein("Dinker1"); got != "5712 SV Dinker N. u. D." {
		t.Fatalf("Dinker1 -> %q", got)
	}
	if got := MatchVerein("Dinker 1"); got != "5712 SV Dinker N. u. D." {
		t.Fatalf("Dinker 1 -> %q", got)
	}
	if got := MatchVerein("Dinkerl"); got != "5712 SV Dinker N. u. D." {
		t.Fatalf("Dinkerl -> %q", got)
	}
	if got := MatchVerein("SSC Weslarn"); got != "5741 SSC Weslarn" {
		t.Fatalf("Weslarn -> %q", got)
	}
	if got := MatchVerein("SSV"); got != "" {
		t.Fatalf("ambiguous SSV -> %q", got)
	}
	if got := MatchDisziplin("LG 30 Schuss Auflage"); got != "10m Luftgewehr Auflage" {
		t.Fatalf("LG auflage -> %q", got)
	}
	if got := MatchDisziplin("LP"); got != "10m Luftpistole" {
		t.Fatalf("LP -> %q", got)
	}
	if got := MatchDisziplin("KK 50m"); got != "KK 50 m Auflage" {
		t.Fatalf("KK -> %q", got)
	}
}

func TestFromSnapshotCapsAndSplitsEinzel(t *testing.T) {
	snap := wettkampf.Snapshot{
		Teams: []wettkampf.TeamView{
			{
				ID: "t1", Name: "Dinker1", Club: "Dinker1", Decimal: true, SumDec: 934.1, Count: 3,
				Members: []wettkampf.MemberView{
					{Name: "Wolfgang Nölle", Discipline: "LG 40 Schuss", HasWertung: true, SumDec: 311.1},
					{Name: "Christian Pieper", HasWertung: true, SumDec: 308.1},
					{Name: "Patrick Mai", HasWertung: true, SumDec: 314.9},
					{Name: "Zu viel", HasWertung: true, SumDec: 1},
				},
			},
			{
				ID: "t2", Name: "SSC Weslarn", Club: "SSC Weslarn", Decimal: true, SumDec: 918, Count: 3,
				Members: []wettkampf.MemberView{
					{Name: "Julia Deimel", HasWertung: true, SumDec: 309},
				},
			},
		},
		Roster: []wettkampf.RosterView{
			{Name: "Daniel Byrdeck", Club: "Dinker1", Excluded: true, HasWertung: true, SumDec: 308.2, TeamID: "t1"},
		},
	}
	b, err := FromSnapshot(snap, "t1", "t2", time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if b.Datum != "19.10.2026" {
		t.Fatalf("datum %q", b.Datum)
	}
	if b.Heim.Verein != "5712 SV Dinker N. u. D." || b.Gast.Verein != "5741 SSC Weslarn" {
		t.Fatalf("vereine %#v %#v", b.Heim.Verein, b.Gast.Verein)
	}
	if b.Heim.Disziplin != "10m Luftgewehr" {
		t.Fatalf("disziplin %q", b.Heim.Disziplin)
	}
	if len(b.Heim.Members) != 3 || b.Heim.Members[2].Name != "Patrick Mai" {
		t.Fatalf("members %#v", b.Heim.Members)
	}
	if b.Heim.Members[0].Ergebnis != "311,1" || b.Heim.Summe != "934,1" {
		t.Fatalf("scores %q %q", b.Heim.Members[0].Ergebnis, b.Heim.Summe)
	}
	if len(b.Heim.Einzel) != 1 || b.Heim.Einzel[0].Name != "Daniel Byrdeck" || b.Heim.Einzel[0].Ergebnis != "308,2" {
		t.Fatalf("einzel %#v", b.Heim.Einzel)
	}
	if b.Heim.Members[0].Passnr != "" || b.Heim.Einzel[0].Jahrgang != "" {
		t.Fatal("snapshot must not invent passnr or jahrgang")
	}
}

func TestFillRoundTrip(t *testing.T) {
	b := Bericht{
		Datum: "19.10.2026",
		Heim: Side{
			Verein:    "5712 SV Dinker N. u. D.",
			Disziplin: "10m Luftgewehr",
			Members: []Shooter{{
				Name: "Wolfgang Nölle", Passnr: "10712211", Ergebnis: "311,1",
			}},
			Summe: "934,1",
		},
		Gast: Side{
			Verein: "5741 SSC Weslarn",
			Members: []Shooter{{
				Name: "Julia Deimel", Ergebnis: "309,0",
			}},
		},
	}
	pdf, err := Fill(b, VorlageSoest)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := api.ExportFormJSON(bytes.NewReader(pdf), &buf, "filled", nil); err != nil {
		t.Fatal(err)
	}
	var group formBody
	var wrapped formGroup
	if err := json.Unmarshal(buf.Bytes(), &wrapped); err != nil {
		t.Fatal(err)
	}
	if len(wrapped.Forms) != 1 {
		t.Fatalf("forms %d", len(wrapped.Forms))
	}
	group = wrapped.Forms[0]
	got := map[string]string{}
	for _, tf := range append(append(group.Text, group.Date...), group.Combo...) {
		got[tf.Name] = tf.Value
	}
	want := map[string]string{
		"datum":                   "19.10.2026",
		"heim_verein":             "5712 SV Dinker N. u. D.",
		"heim_disziplin":          "10m Luftgewehr",
		"heim_mannschaft1_name":   "Wolfgang Nölle",
		"heim_mannschaft1_passnr": "10712211",
		"heim_summe":              "934,1",
		"gast_mannschaft1_name":   "Julia Deimel",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q want %q", k, got[k], v)
		}
	}
	if strings.TrimSpace(got["heim_durchgang"]) != "" {
		t.Fatalf("durchgang should stay empty, got %q", got["heim_durchgang"])
	}
	neutral, err := Fill(b, VorlageNeutral)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(neutral, []byte("DCTDecode")) {
		t.Fatal("neutral report still embeds the JPEG crest")
	}
	same, err := filledValues(neutral)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range want {
		if same[k] != v {
			t.Errorf("neutral %s = %q want %q", k, same[k], v)
		}
	}
	if _, err := Fill(b, "logo"); !errors.Is(err, ErrUnknownVorlage) {
		t.Fatalf("unknown vorlage: %v", err)
	}
}

func TestNeutralTemplateDropsLetterhead(t *testing.T) {
	if bytes.Contains(templateNeutral, []byte("DCTDecode")) {
		t.Fatal("neutral template still has the JPEG crest")
	}
	if !bytes.Contains(templateSoest, []byte("DCTDecode")) {
		t.Fatal("soest template lost its crest")
	}
	if len(templateNeutral) >= len(templateSoest)/2 {
		t.Fatalf("neutral template is not smaller: %d vs %d", len(templateNeutral), len(templateSoest))
	}
}

func filledValues(pdf []byte) (map[string]string, error) {
	var buf bytes.Buffer
	if err := api.ExportFormJSON(bytes.NewReader(pdf), &buf, "filled", nil); err != nil {
		return nil, err
	}
	var wrapped formGroup
	if err := json.Unmarshal(buf.Bytes(), &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Forms) != 1 {
		return nil, fmt.Errorf("forms %d", len(wrapped.Forms))
	}
	got := map[string]string{}
	group := wrapped.Forms[0]
	for _, tf := range append(append(group.Text, group.Date...), group.Combo...) {
		got[tf.Name] = tf.Value
	}
	return got, nil
}
