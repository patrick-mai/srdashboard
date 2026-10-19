package rwkbericht

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

//go:embed template.pdf
var templateSoest []byte

//go:embed template_neutral.pdf
var templateNeutral []byte

const (
	// VorlageSoest is the form with the Schützenkreis Soest-Lippstadt letterhead.
	VorlageSoest = "soest"
	// VorlageNeutral is the same form without that letterhead.
	VorlageNeutral = "neutral"
)

// Vorlage is one selectable RWK form.
type Vorlage struct {
	ID   string
	Name string
}

// ErrUnknownVorlage is returned when Fill is asked for an id that is not embedded.
var ErrUnknownVorlage = errors.New("unbekannte Vorlage")

// Vorlagen lists the embedded forms in selection order. The first entry is the default.
func Vorlagen() []Vorlage {
	return []Vorlage{
		{ID: VorlageSoest, Name: "Schützenkreis Soest-Lippstadt"},
		{ID: VorlageNeutral, Name: "Ohne Kopf"},
	}
}

type formGroup struct {
	Forms []formBody `json:"forms"`
}

type formBody struct {
	Text  []formField `json:"textfield,omitempty"`
	Date  []formField `json:"datefield,omitempty"`
	Combo []formField `json:"combobox,omitempty"`
}

type formField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Fill writes bericht into the selected embedded template and returns the PDF bytes.
// An empty id uses the Schützenkreis Soest-Lippstadt form.
func Fill(b Bericht, vorlage string) ([]byte, error) {
	tpl, err := templateByID(vorlage)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(formGroup{Forms: []formBody{formFrom(b)}})
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.FillForm(bytes.NewReader(tpl), bytes.NewReader(raw), &out, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func templateByID(id string) ([]byte, error) {
	switch id {
	case "", VorlageSoest:
		return templateSoest, nil
	case VorlageNeutral:
		return templateNeutral, nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnknownVorlage, id)
	}
}

func formFrom(b Bericht) formBody {
	var f formBody
	text := func(name, value string) {
		if value == "" {
			return
		}
		f.Text = append(f.Text, formField{Name: name, Value: value})
	}
	combo := func(name, value string) {
		if value == "" {
			return
		}
		f.Combo = append(f.Combo, formField{Name: name, Value: value})
	}
	text("datum", b.Datum)
	addSide(&f, "heim", b.Heim, text, combo)
	addSide(&f, "gast", b.Gast, text, combo)
	return f
}

func addSide(f *formBody, prefix string, side Side, text func(string, string), combo func(string, string)) {
	combo(prefix+"_disziplin", side.Disziplin)
	combo(prefix+"_verein", side.Verein)
	text(prefix+"_durchgang", side.Durchgang)
	text(prefix+"_gruppe", side.Gruppe)
	text(prefix+"_mannschaft", side.Mannschaft)
	text(prefix+"_summe", side.Summe)
	addRows(text, prefix+"_mannschaft", side.Members)
	addRows(text, prefix+"_einzel", side.Einzel)
}

func addRows(text func(string, string), prefix string, rows []Shooter) {
	if len(rows) > formRows {
		rows = rows[:formRows]
	}
	for i, row := range rows {
		n := prefix + string(rune('1'+i))
		text(n+"_name", row.Name)
		text(n+"_passnr", row.Passnr)
		text(n+"_jahrgang", row.Jahrgang)
		text(n+"_ergebnis", row.Ergebnis)
	}
}
