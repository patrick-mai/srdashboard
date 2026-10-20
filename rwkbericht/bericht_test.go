package rwkbericht

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

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
	if got := MatchVereinID("5712"); got != "5712 SV Dinker N. u. D." {
		t.Fatalf("5712 -> %q", got)
	}
	if got := MatchVereinID("57"); got != "" {
		t.Fatalf("short id -> %q", got)
	}
	if got := MatchVereinID(""); got != "" {
		t.Fatalf("empty id -> %q", got)
	}
	if got := ClubLabel("TSV Beispiel"); got != "TSV Beispiel" {
		t.Fatalf("unknown club -> %q", got)
	}
}

func TestFromSnapshotFillsPassAndVereinNumber(t *testing.T) {
	snap := wettkampf.Snapshot{
		Teams: []wettkampf.TeamView{
			{
				ID: "t1", Name: "Dinker 1", Club: "SSV", ClubNumber: "5739", Decimal: true, SumDec: 303.5,
				Members: []wettkampf.MemberView{
					{Name: "Anna Müller", HasWertung: true, SumDec: 303.5, PassNumber: "10712211", ClubNumber: "5739"},
				},
			},
			{
				ID: "t2", Name: "Weslarn", Club: "SSC Weslarn", ClubNumber: "9999", Decimal: true, SumDec: 300.5,
				Members: []wettkampf.MemberView{
					{Name: "Jonas Becker", HasWertung: true, SumDec: 300.5, PassNumber: "20823322"},
				},
			},
		},
		Roster: []wettkampf.RosterView{
			{Name: "Ina Berg", Club: "SSV", Excluded: true, HasWertung: true, SumDec: 297.5, TeamID: "t1", PassNumber: "30934433"},
		},
	}
	b, err := FromSnapshot(snap, "t1", "t2", time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if b.Heim.Verein != "5739 SSV Eikeloh" {
		t.Fatalf("heim verein %q", b.Heim.Verein)
	}
	if b.Gast.Verein != "5741 SSC Weslarn" {
		t.Fatalf("unknown club number should fall back to the name, got %q", b.Gast.Verein)
	}
	if b.Heim.Members[0].Passnr != "10712211" || b.Heim.Einzel[0].Passnr != "30934433" {
		t.Fatalf("pass %#v %#v", b.Heim.Members[0], b.Heim.Einzel[0])
	}
	if b.Gast.Members[0].Passnr != "20823322" {
		t.Fatalf("gast pass %q", b.Gast.Members[0].Passnr)
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
	pdf, err := Fill(b, "Ohne Kopf")
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
	if _, err := Fill(b, "logo"); !errors.Is(err, ErrUnknownVorlage) {
		t.Fatalf("unknown vorlage: %v", err)
	}
	if _, err := Fill(b, "soest"); !errors.Is(err, ErrUnknownVorlage) {
		t.Fatalf("soest vorlage must not ship: %v", err)
	}
}

func TestFillChoiceIndexMatchesValue(t *testing.T) {
	b := Bericht{
		Heim: Side{
			Disziplin: "10m Luftgewehr Auflage",
			Verein:    "5712 SV Dinker N. u. D.",
		},
		Gast: Side{Disziplin: "10m Luftgewehr"},
	}
	for _, id := range []string{"", "Ohne Kopf"} {
		pdf, err := Fill(b, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := assertChoiceIndexes(pdf); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

func assertChoiceIndexes(pdf []byte) error {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return err
	}
	xref := ctx.XRefTable
	seen := 0
	for n := 1; n < *xref.Size; n++ {
		entry, ok := xref.FindTableEntry(n, 0)
		if !ok || entry == nil || entry.Object == nil {
			continue
		}
		d, ok := entry.Object.(types.Dict)
		if !ok {
			continue
		}
		if _, found := d.Find("Opt"); !found {
			continue
		}
		sl := d.StringLiteralEntry("V")
		if sl == nil {
			continue
		}
		value, err := types.StringLiteralToString(*sl)
		if err != nil {
			return err
		}
		opts, err := choiceOptions(xref, d)
		if err != nil {
			return err
		}
		ind, ok := d["I"].(types.Array)
		if !ok || len(ind) != 1 {
			return fmt.Errorf("%q has no choice index", strings.TrimSpace(value))
		}
		i, ok := ind[0].(types.Integer)
		if !ok || int(i) < 0 || int(i) >= len(opts) {
			return fmt.Errorf("%q index %v", value, ind[0])
		}
		if opts[int(i)] != strings.TrimSpace(value) {
			return fmt.Errorf("index %d is %q, value is %q", i, opts[int(i)], strings.TrimSpace(value))
		}
		seen++
	}
	if seen < 2 {
		return fmt.Errorf("checked %d choice fields, want at least 2", seen)
	}
	return nil
}

func TestShippedVorlageDropsLetterheadAndClubs(t *testing.T) {
	files, err := vorlageFiles()
	if err != nil {
		t.Fatal(err)
	}
	var shipped *vorlageFile
	for i := range files {
		id := strings.ToLower(files[i].ID)
		if strings.Contains(id, "soest") || strings.Contains(id, "lippstadt") {
			t.Fatalf("Soest-Lippstadt template must not ship: %s", files[i].path)
		}
		if files[i].ID == "Ohne Kopf" {
			shipped = &files[i]
		}
	}
	if shipped == nil || !strings.Contains(filepath.ToSlash(shipped.path), "plugins/classic-range-condensed/") {
		t.Fatalf("template must live in the plugin, got %#v", shipped)
	}
	raw, err := os.ReadFile(shipped.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("DCTDecode")) {
		t.Fatal("shipped template still has the JPEG crest")
	}
	for _, club := range []string{"Lippstadt", "Soest", "Böckum", "Geseke"} {
		if bytes.Contains(raw, []byte(club)) || bytes.Contains(raw, utf16BE(club)) {
			t.Fatalf("shipped template still contains %q", club)
		}
	}
	f, err := os.Open(shipped.path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fields, err := api.FormFields(f, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, field := range fields {
		if field.Name != "heim_verein" && field.Name != "gast_verein" {
			continue
		}
		seen++
		if field.Typ.String() != "Textfield" {
			t.Fatalf("%s is %s, want a text field without the club list", field.Name, field.Typ)
		}
		if field.Opts != "" {
			t.Fatalf("%s still has options %q", field.Name, field.Opts)
		}
	}
	if seen != 2 {
		t.Fatalf("verein fields %d", seen)
	}
}

func utf16BE(s string) []byte {
	enc := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(enc)*2)
	for _, r := range enc {
		b = append(b, byte(r>>8), byte(r))
	}
	return b
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
