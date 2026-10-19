package main

import "testing"

func at(text string, x, y float64) Word {
	return Word{Text: text, X: x, Y: y, W: 36, H: 12}
}

func TestParseOpticScoreSheet(t *testing.T) {
	var words []Word
	add := func(y float64, items ...Word) {
		for _, w := range items {
			w.Y = y
			words = append(words, w)
		}
	}
	words = append(words, at("Dinkerl", 20, 30), at("vs.", 90, 39), at("SSC", 700, 30), at("Weslarn", 750, 31), at("19.10.2026", 860, 32))
	add(80, at("934.1", 420, 0), at("918.0", 540, 0))
	add(120,
		at("10712211", 20, 0), at("Wolfgang", 80, 0), at("Nölle", 150, 0),
		at("105.3", 240, 0), at("102.4", 290, 0), at("103.4", 340, 0), at("311.1", 420, 0),
		at("309.0", 540, 0), at("102.4", 600, 0), at("102.9", 650, 0), at("103.7", 700, 0),
		at("Julia", 780, 0), at("Deimel", 830, 0), at("11569297", 900, 0),
	)
	add(160,
		at("10712153", 20, 0), at("Christian", 80, 0), at("Pieper", 160, 0),
		at("103.3", 240, 0), at("103.5", 290, 0), at("101.3", 340, 0), at("308.1", 420, 0),
		at("303.6", 540, 0), at("100.3", 600, 0), at("102.1", 650, 0), at("101.2", 700, 0),
		at("Frederic", 760, 0), at("Schmidt", 830, 0), at("10733399", 900, 0),
	)
	add(200,
		at("30516879", 20, 0), at("Patrick", 80, 0), at("Mai", 150, 0),
		at("105.0", 240, 0), at("104.9", 290, 0), at("105.0", 340, 0), at("314.9", 420, 0),
		at("305.4", 540, 0), at("101.3", 600, 0), at("102.3", 650, 0), at("101.8", 700, 0),
		at("Maria", 770, 0), at("Wohlgemuth", 820, 0), at("29667497", 910, 0),
	)
	add(240,
		at("21472870", 20, 0), at("Daniel", 80, 0), at("Byrdeck", 140, 0), at("(AK)", 210, 0),
		at("102.4", 260, 0), at("103.5", 310, 0), at("102.3", 360, 0), at("308.2", 420, 0),
	)
	b := ParseSheet(words, 1000)
	if b.Datum != "19.10.2026" {
		t.Fatalf("datum %q", b.Datum)
	}
	if b.Heim.Verein != "5712 SV Dinker N. u. D." || b.Gast.Verein != "5741 SSC Weslarn" {
		t.Fatalf("clubs %q %q", b.Heim.Verein, b.Gast.Verein)
	}
	if b.Heim.Summe != "934.1" || b.Gast.Summe != "918.0" {
		t.Fatalf("sums %q %q", b.Heim.Summe, b.Gast.Summe)
	}
	if len(b.Heim.Members) != 3 || b.Heim.Members[0].Name != "Wolfgang Nölle" || b.Heim.Members[0].Passnr != "10712211" || b.Heim.Members[0].Ergebnis != "311.1" {
		t.Fatalf("heim %#v", b.Heim.Members)
	}
	if len(b.Heim.Einzel) != 1 || b.Heim.Einzel[0].Name != "Daniel Byrdeck" || b.Heim.Einzel[0].Passnr != "21472870" || b.Heim.Einzel[0].Ergebnis != "308.2" {
		t.Fatalf("einzel %#v", b.Heim.Einzel)
	}
	if len(b.Gast.Members) != 3 || b.Gast.Members[2].Name != "Maria Wohlgemuth" || b.Gast.Members[0].Ergebnis != "309.0" {
		t.Fatalf("gast %#v", b.Gast.Members)
	}
	if b.Heim.Disziplin != "" || b.Heim.Members[0].Jahrgang != "" {
		t.Fatal("sheet has no disziplin or jahrgang")
	}
}

func TestParseArgsVorlage(t *testing.T) {
	vorlage, files, ok := parseArgs([]string{"--vorlage", "neutral", "quelle.pdf", "ziel.pdf"})
	if !ok || vorlage != "neutral" || len(files) != 2 || files[1] != "ziel.pdf" {
		t.Fatalf("%q %#v %v", vorlage, files, ok)
	}
	if _, _, ok := parseArgs([]string{"--vorlage"}); ok {
		t.Fatal("missing vorlage value")
	}
	if _, _, ok := parseArgs([]string{"--check", "quelle.pdf"}); ok {
		t.Fatal("check is not a convert flag")
	}
}
