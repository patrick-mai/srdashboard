package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srdashboard/config"
	"srdashboard/state"
	"srdashboard/udp"
)

func TestAllowShotDropsOutsideMaxAndInactive(t *testing.T) {
	h, _ := testHandlers(t, "")
	if !h.AllowShot(1) || !h.AllowShot(2) {
		t.Fatal("configured active lanes should apply")
	}
	if h.AllowShot(0) || h.AllowShot(3) || h.AllowShot(7) {
		t.Fatal("outside 1..N must drop")
	}
	h.Runtime.SetInactiveList([]int{2}, 2)
	if h.AllowShot(2) {
		t.Fatal("inactive lane must drop")
	}
	if !h.AllowShot(1) {
		t.Fatal("other active lane still applies")
	}
}

func TestRuntimePutDeactivatesAndResets(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	body := `{"inactiveRanges":[2]}`
	req := httptest.NewRequest(http.MethodPut, "/api/runtime", strings.NewReader(body))
	req.Header.Set("X-SR-Control-Token", "s3cret")
	rec := httptest.NewRecorder()
	h.ServeRuntime(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !h.isRangeInactive(2) || h.isRangeInactive(1) {
		t.Fatalf("inactive = %v", h.inactiveRangeList())
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.InactiveRanges) != 1 || resp.InactiveRanges[0] != 2 {
		t.Fatalf("response = %#v", resp.InactiveRanges)
	}
	got, err := config.LoadRuntime(config.RuntimePath(h.ConfigPath))
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsInactive(2) {
		t.Fatal("runtime.xml not persisted")
	}
}

func TestRuntimePutRequiresToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	rec := httptest.NewRecorder()
	h.ServeRuntime(rec, httptest.NewRequest(http.MethodPut, "/api/runtime", strings.NewReader(`{"inactiveRanges":[1]}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestPipelineDropsBeforeApply(t *testing.T) {
	h, _ := testHandlers(t, "")
	h.Runtime.SetInactiveList([]int{1}, 2)
	data, err := udp.BuildShotPacket(udp.ShotPacketOpts{Range: 1, DecValue: 10.1})
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	p := udp.Pipeline{
		State:  h.State,
		Filter: h,
		OnShot: func(_ int, _ state.Shot, _ int) { applied++ },
	}
	p.Ingest(data)
	if applied != 0 {
		t.Fatalf("applied = %d", applied)
	}
	if n := h.State.ShotNumber(1); n != 0 {
		t.Fatalf("shotNumber = %d", n)
	}

	data7, err := udp.BuildShotPacket(udp.ShotPacketOpts{Range: 7, DecValue: 10.1})
	if err != nil {
		t.Fatal(err)
	}
	p.Ingest(data7)
	for _, s := range h.State.Snapshot() {
		if s.RangeNum == 7 {
			t.Fatal("must not grow live state for range 7")
		}
	}
}

func TestLiveGetOmitsInactive(t *testing.T) {
	h, _ := testHandlers(t, "")
	h.Runtime.SetInactiveList([]int{2}, 2)
	rec := httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodGet, "/api/live", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp LiveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Ranges) != 1 || resp.Ranges[0].RangeNum != 1 {
		t.Fatalf("ranges = %#v", resp.Ranges)
	}
	rec = httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodGet, "/api/live?range=2", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("inactive single range status = %d, want 404", rec.Code)
	}
}
