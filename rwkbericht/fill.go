package rwkbericht

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ErrUnknownVorlage is returned when Fill is asked for an id that is not in the plugin folder.
var ErrUnknownVorlage = errors.New("unbekannte Vorlage")

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

// Fill writes bericht into the selected PDF from the Classic Range Condensed plugin and returns the PDF bytes.
// An empty id uses the first form in that folder.
func Fill(b Bericht, vorlage string) ([]byte, error) {
	tpl, err := templateByID(vorlage)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(formGroup{Forms: []formBody{formFrom(b)}})
	if err != nil {
		return nil, err
	}
	var filled bytes.Buffer
	if err := api.FillForm(bytes.NewReader(tpl), bytes.NewReader(raw), &filled, nil); err != nil {
		return nil, err
	}
	// pdfcpu drops empty combo entries when it writes /I, so the index points
	// at the previous choice. The template lists a blank first, and viewers
	// that paint /I then show "10m Luftgewehr" for the Auflage value.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(filled.Bytes()), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if err := fixChoiceIndexes(ctx); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func fixChoiceIndexes(ctx *model.Context) error {
	xref := ctx.XRefTable
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
		value = strings.TrimSpace(value)
		opts, err := choiceOptions(xref, d)
		if err != nil {
			return err
		}
		idx := -1
		for i, opt := range opts {
			if opt == value {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		d["I"] = types.Array{types.Integer(idx)}
	}
	return nil
}

func choiceOptions(xref *model.XRefTable, d types.Dict) ([]string, error) {
	o, ok := d.Find("Opt")
	if !ok {
		return nil, nil
	}
	arr, err := xref.DereferenceArray(o)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		label, ok, err := choiceLabel(item)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("rwkbericht: unreadable choice option")
		}
		out = append(out, strings.TrimSpace(label))
	}
	return out, nil
}

func choiceLabel(o types.Object) (string, bool, error) {
	switch o := o.(type) {
	case types.StringLiteral:
		s, err := types.StringLiteralToString(o)
		return s, err == nil, err
	case types.Array:
		if len(o) != 2 {
			return "", false, nil
		}
		return choiceLabel(o[1])
	default:
		return "", false, nil
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
	// Shipped forms keep Verein as a text field, so the Soest-Lippstadt club
	// list is not a dropdown. A form that still has a combo accepts the same value.
	text(prefix+"_verein", side.Verein)
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
