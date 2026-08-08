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

func TestRingReaderEmptyRejected(t *testing.T) {
	_, err := qrformat.MustGet("rr").EncodeURL(qrformat.ResultInput{RangeNum: 1})
	if err == nil {
		t.Fatal("expected error for empty result")
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

