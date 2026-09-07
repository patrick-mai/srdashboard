package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srdashboard/udp"
)

func TestRecoveryReplayConsoleGoesThroughValidation(t *testing.T) {
	h, _ := testHandlers(t, "")
	p := udp.Pipeline{State: h.State, Filter: h}
	h.ReplayLog = func(packets [][]byte) int {
		n := 0
		for _, pkt := range packets {
			n += p.IngestReplay(pkt)
		}
		return n
	}
	body := `{"clear":true,"log":"UDP: shot applied range=1 X=0 Y=0 DecValue=10.9 at=-\nUDP: shot applied range=1 X=800 Y=0 DecValue=10.3 at=-\n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/recovery/replay", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.RecoveryReplay(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp recoveryReplayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Lines != 2 {
		t.Fatalf("lines=%d want 2", resp.Lines)
	}
	if resp.Applied != 1 {
		t.Fatalf("applied=%d want 1 (inconsistent 10.3 dropped)", resp.Applied)
	}
	shots := h.State.Snapshot()[0].Shots
	if len(shots) != 1 || shots[0].DecValue != 10.9 {
		t.Fatalf("shots=%+v", shots)
	}
}

func TestRecoveryReplayRequiresToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	h.ReplayLog = func([][]byte) int { return 0 }
	req := httptest.NewRequest(http.MethodPost, "/api/recovery/replay", strings.NewReader(`{"log":"x"}`))
	rec := httptest.NewRecorder()
	h.RecoveryReplay(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func TestRecoveryReplayEmptyLog(t *testing.T) {
	h, _ := testHandlers(t, "")
	called := false
	h.ReplayLog = func(packets [][]byte) int {
		called = true
		if len(packets) != 0 {
			t.Fatalf("packets=%d", len(packets))
		}
		return 0
	}
	req := httptest.NewRequest(http.MethodPost, "/api/recovery/replay", strings.NewReader(`{"log":"not a shot line"}`))
	rec := httptest.NewRecorder()
	h.RecoveryReplay(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !called {
		t.Fatal("ReplayLog not called")
	}
	var resp recoveryReplayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Lines != 0 || resp.Applied != 0 {
		t.Fatalf("resp=%+v", resp)
	}
}
