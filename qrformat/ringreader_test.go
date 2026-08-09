package qrformat_test

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"srdashboard/qrformat"
	"srdashboard/state"
)

func TestRingReaderEncodeURLRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 6, 18, 15, 0, 0, time.FixedZone("CEST", 2*3600))
	snap := state.RangeSnapshot{
		RangeNum:   3,
		Discipline: "Training LP",
		DiscType:   "LP",
		WarmupShots: []state.Shot{
			{X: 310, Y: -240, DecValue: 8.7, At: at, IsWarmup: true},
			{X: -120, Y: 180, DecValue: 9.6, At: at.Add(30 * time.Second), IsWarmup: true},
		},
		SeriesShots: [][]state.Shot{
			{
				{X: 140, Y: 60, DecValue: 9.8, At: at.Add(2 * time.Minute)},
				{X: -30, Y: 50, DecValue: 10.2, At: at.Add(2*time.Minute + 40*time.Second)},
			},
		},
	}
	url, err := qrformat.MustGet("rr").EncodeURL(qrformat.FromRangeSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "https://ringreader.app/import/qr#") {
		t.Fatalf("url prefix: %s", url)
	}
	b64 := strings.TrimPrefix(url, "https://ringreader.app/import/qr#")
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64url: %v", err)
	}
	fr := flate.NewReader(bytes.NewReader(raw))
	plain, err := io.ReadAll(fr)
	_ = fr.Close()
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(plain, &env); err != nil {
		t.Fatalf("json: %v\n%s", err, plain)
	}
	if env["v"] != float64(1) || env["src"] != "srdashboard" || env["fmt"] != "rr" {
		t.Fatalf("envelope: %+v", env)
	}
	payload, _ := env["payload"].(map[string]any)
	series, _ := payload["series"].([]any)
	if len(series) != 2 {
		t.Fatalf("series len=%d want 2", len(series))
	}
	probe, _ := series[0].(map[string]any)
	if probe["trial"] != true {
		t.Fatalf("first series should be trial: %+v", probe)
	}
	if payload["discipline"] != "2.10" {
		t.Fatalf("discipline=%v want 2.10", payload["discipline"])
	}
}

func TestRingReaderWarmupSplitEvery10(t *testing.T) {
	at := time.Date(2026, 4, 28, 18, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	warm := make([]state.Shot, 22)
	for i := range warm {
		warm[i] = state.Shot{
			X: i, Y: i, DecValue: 10.0, At: at.Add(time.Duration(i) * time.Second), IsWarmup: true,
		}
	}
	comp := make([]state.Shot, 10)
	for i := range comp {
		comp[i] = state.Shot{X: i, Y: i, DecValue: 10.5, At: at.Add(time.Duration(100+i) * time.Second)}
	}
	url, err := qrformat.MustGet("rr").EncodeURL(qrformat.FromRangeSnapshot(state.RangeSnapshot{
		RangeNum:    1,
		DiscType:    "LG",
		Discipline:  "LG 30 Schuss Auflage",
		WarmupShots: warm,
		SeriesShots: [][]state.Shot{comp},
	}))
	if err != nil {
		t.Fatal(err)
	}
	b64 := strings.TrimPrefix(url, "https://ringreader.app/import/qr#")
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	fr := flate.NewReader(bytes.NewReader(raw))
	plain, err := io.ReadAll(fr)
	_ = fr.Close()
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Payload struct {
			Series []struct {
				ID    string `json:"id"`
				Trial bool   `json:"trial"`
				Shots []any  `json:"shots"`
			} `json:"series"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(plain, &env); err != nil {
		t.Fatal(err)
	}
	// 22 probe → probe1(10)+probe2(10)+probe3(2), then 1 competition series
	if len(env.Payload.Series) != 4 {
		t.Fatalf("series blocks=%d want 4", len(env.Payload.Series))
	}
	want := []struct {
		trial bool
		n     int
		idSub string
	}{
		{true, 10, "probe1"},
		{true, 10, "probe2"},
		{true, 2, "probe3"},
		{false, 10, "-s1"},
	}
	for i, w := range want {
		s := env.Payload.Series[i]
		if s.Trial != w.trial || len(s.Shots) != w.n || !strings.Contains(s.ID, w.idSub) {
			t.Fatalf("series[%d]=id=%q trial=%v shots=%d want id~%q trial=%v n=%d",
				i, s.ID, s.Trial, len(s.Shots), w.idSub, w.trial, w.n)
		}
	}
}


func TestRingReaderDSBDisciplineCodes(t *testing.T) {
	cases := []struct {
		discType, discipline, want string
	}{
		{"LG", "Luftgewehr", "1.10"},
		{"LG", "LG 30 Schuss Auflage", "1.11"},
		{"LP", "Training LP", "2.10"},
		{"LP", "LP 40 Schuss Auflage", "2.11"},
		{"KK", "KK 40 Schuss", "1.40"},
		{"KK", "KK Sportgewehr Auflage", "1.41"},
		{"", "LG 30 Schuss Auflage", "1.11"},
		{"", "Luftpistole freistehend", "2.10"},
	}
	for _, tc := range cases {
		in := qrformat.ResultInput{
			RangeNum:   1,
			DiscType:   tc.discType,
			Discipline: tc.discipline,
			OpenShots:  []qrformat.ShotInput{{DecValue: 10.0, X: 0, Y: 0}},
		}
		url, err := qrformat.MustGet("rr").EncodeURL(in)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.discType, tc.discipline, err)
		}
		b64 := strings.TrimPrefix(url, "https://ringreader.app/import/qr#")
		raw, err := base64.RawURLEncoding.DecodeString(b64)
		if err != nil {
			t.Fatal(err)
		}
		fr := flate.NewReader(bytes.NewReader(raw))
		plain, err := io.ReadAll(fr)
		_ = fr.Close()
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Payload struct {
				Discipline string `json:"discipline"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(plain, &env); err != nil {
			t.Fatal(err)
		}
		if env.Payload.Discipline != tc.want {
			t.Fatalf("%s / %q → %q, want %q", tc.discType, tc.discipline, env.Payload.Discipline, tc.want)
		}
	}
}

