package qrformat

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

func init() {
	Register(RingReader{})
}

const (
	ringReaderSrc     = "srdashboard"
	ringReaderBaseURL = "https://ringreader.app/import/qr#"
)

// RingReader encodes fmt "rr" per docs/QR_import_RingReader.txt.
type RingReader struct{}

func (RingReader) ID() string    { return "rr" }
func (RingReader) Label() string { return "Ring Reader" }

// EncodeURL builds https://ringreader.app/import/qr#<deflateRaw+base64url>.
func (RingReader) EncodeURL(snap ResultInput) (string, error) {
	if !snap.HasExportableShots() {
		return "", fmt.Errorf("no shots to export")
	}
	env := buildRingReaderEnvelope(snap)
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return ringReaderBaseURL + base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

type rrEnvelope struct {
	V       int             `json:"v"`
	Src     string          `json:"src"`
	Fmt     string          `json:"fmt"`
	Payload rrPayload       `json:"payload"`
}

type rrPayload struct {
	Name       string     `json:"name,omitempty"`
	Discipline string     `json:"discipline,omitempty"`
	Date       string     `json:"date"`
	Series     []rrSeries `json:"series"`
}

type rrSeries struct {
	ID    string   `json:"id,omitempty"`
	Trial bool     `json:"trial,omitempty"`
	Shots []rrShot `json:"shots"`
}

type rrShot struct {
	S float64  `json:"s"`
	X *float64 `json:"x,omitempty"`
	Y *float64 `json:"y,omitempty"`
	O *float64 `json:"o,omitempty"`
}

func buildRingReaderEnvelope(snap ResultInput) rrEnvelope {
	date := resultDate(snap)
	day := date.Format("2006-01-02")
	disc := dsbDiscipline(snap.DiscType, snap.Discipline)
	slug := discSlug(snap.DiscType, snap.Discipline)

	series := make([]rrSeries, 0, 1+len(snap.Series)+1)
	scale := dsgPerMm(snap.DiscType)

	if len(snap.WarmupShots) > 0 {
		series = append(series, rrSeries{
			ID:    fmt.Sprintf("%s-%s-bahn%d-probe", day, slug, snap.RangeNum),
			Trial: true,
			Shots: mapRRShots(snap.WarmupShots, scale),
		})
	}

	for i, ser := range snap.Series {
		series = append(series, rrSeries{
			ID:    fmt.Sprintf("%s-%s-bahn%d-s%d", day, slug, snap.RangeNum, i+1),
			Shots: mapRRShots(ser, scale),
		})
	}
	if len(snap.OpenShots) > 0 {
		n := len(snap.Series) + 1
		series = append(series, rrSeries{
			ID:    fmt.Sprintf("%s-%s-bahn%d-s%d", day, slug, snap.RangeNum, n),
			Shots: mapRRShots(snap.OpenShots, scale),
		})
	}

	name := strings.TrimSpace(snap.Discipline)
	if name == "" {
		name = strings.TrimSpace(snap.ShooterName)
	}

	return rrEnvelope{
		V:   1,
		Src: ringReaderSrc,
		Fmt: "rr",
		Payload: rrPayload{
			Name:       name,
			Discipline: disc,
			Date:       date.Format(time.RFC3339),
			Series:     series,
		},
	}
}

func mapRRShots(shots []ShotInput, dsgPerMm float64) []rrShot {
	out := make([]rrShot, 0, len(shots))
	var seriesStart time.Time
	for i, s := range shots {
		sh := rrShot{S: round1(s.DecValue)}
		x := round2(float64(s.X) / dsgPerMm)
		y := round2(float64(s.Y) / dsgPerMm)
		sh.X = &x
		sh.Y = &y
		if !s.At.IsZero() {
			if i == 0 || seriesStart.IsZero() {
				seriesStart = s.At
				o := 0.0
				sh.O = &o
			} else {
				o := math.Round(s.At.Sub(seriesStart).Seconds())
				if o < 0 {
					o = 0
				}
				sh.O = &o
			}
		}
		out = append(out, sh)
	}
	return out
}

func resultDate(snap ResultInput) time.Time {
	candidates := [][]ShotInput{snap.WarmupShots}
	candidates = append(candidates, snap.Series...)
	candidates = append(candidates, snap.OpenShots)
	var earliest time.Time
	for _, ser := range candidates {
		for _, s := range ser {
			if s.At.IsZero() {
				continue
			}
			if earliest.IsZero() || s.At.Before(earliest) {
				earliest = s.At
			}
		}
	}
	if earliest.IsZero() {
		return time.Now()
	}
	return earliest
}

// dsgPerMm is DISAG OpticScore units per millimetre (LG/KK log-verified at 100).
// LP uses the same factor: its face is ~KK-sized; OpticScore ±9000 still maps to ±90 mm.
func dsgPerMm(discType string) float64 {
	_ = discType
	return 100
}

func dsbDiscipline(discType, discipline string) string {
	auflage := isAuflageLabel(discipline)
	switch strings.ToUpper(strings.TrimSpace(discType)) {
	case "LG":
		if auflage {
			return "1.11"
		}
		return "1.10"
	case "LP":
		if auflage {
			return "2.11"
		}
		return "2.10"
	case "KK":
		if auflage {
			return "1.41" // KK-Sportgewehr Auflage
		}
		return "1.40" // KK-Sportgewehr (freihändig / freistehend)
	default:
		// DiscType missing: try infer from label text alone.
		l := strings.ToLower(discipline)
		switch {
		case strings.Contains(l, "luftpistole") || strings.Contains(l, "lp "):
			if auflage {
				return "2.11"
			}
			return "2.10"
		case strings.Contains(l, "luftgewehr") || strings.Contains(l, "lg "):
			if auflage {
				return "1.11"
			}
			return "1.10"
		case strings.Contains(l, "kleinkaliber") || strings.Contains(l, "kk"):
			if auflage {
				return "1.41"
			}
			return "1.40"
		default:
			return ""
		}
	}
}

func isAuflageLabel(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "auflage") || strings.Contains(l, "aufgelegt")
}

func discSlug(discType, discipline string) string {
	dt := strings.ToLower(strings.TrimSpace(discType))
	if dt != "" {
		return sanitizeID(dt)
	}
	d := strings.ToLower(strings.TrimSpace(discipline))
	if d == "" {
		return "result"
	}
	return sanitizeID(d)
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "result"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
