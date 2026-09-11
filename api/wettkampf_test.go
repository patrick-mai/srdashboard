package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"srdashboard/state"
	"srdashboard/wettkampf"
)

func TestWettkampfGetPutAndReset(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	store, err := wettkampf.Open(filepath.Join(t.TempDir(), "wettkampf.xml"))
	if err != nil {
		t.Fatal(err)
	}
	h.Wettkampf = store
	h.State.ApplyShot(1, &state.ShotPayload{
		FullValue: 10, DecValue: 10.2, Range: 1,
		Shooter: &state.ShotShooter{
			Firstname: "Anna", Lastname: "Müller",
			Club: &state.ShotClub{Name: "SV Adler"},
		},
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuItemName: "LG 40 Schuss"},
	})
	snap, _ := h.State.RangeSnapshot(1)
	h.ObserveWettkampfShot(snap)

	rec := httptest.NewRecorder()
	h.ServeWettkampf(rec, httptest.NewRequest(http.MethodGet, "/api/wettkampf", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d %s", rec.Code, rec.Body.String())
	}
	var view wettkampf.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Roster) != 1 || view.Roster[0].Name != "Anna Müller" {
		t.Fatalf("roster = %#v", view.Roster)
	}
	if len(view.Teams) != 1 || view.Teams[0].Name != "SV Adler" {
		t.Fatalf("teams = %#v", view.Teams)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/wettkampf", strings.NewReader(`{"reset":true}`))
	req.Header.Set("X-SR-Control-Token", "s3cret")
	rec = httptest.NewRecorder()
	h.ServeWettkampf(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reset status = %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Roster) != 0 {
		t.Fatalf("reset roster = %#v", view.Roster)
	}
	if len(view.Teams) == 0 {
		t.Fatal("reset dropped teams")
	}
}

func TestWettkampfPutRequiresToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	store, _ := wettkampf.Open("")
	h.Wettkampf = store
	rec := httptest.NewRecorder()
	h.ServeWettkampf(rec, httptest.NewRequest(http.MethodPut, "/api/wettkampf", strings.NewReader(`{"reset":true}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRuntimePutWettkampfVisibleDoesNotReset(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	h.State.ApplyShot(1, &state.ShotPayload{
		FullValue: 9, DecValue: 9.1, Range: 1,
		Shooter: &state.ShotShooter{Firstname: "A", Lastname: "One"},
	})
	if h.State.ShotNumber(1) != 1 {
		t.Fatalf("setup shotNumber = %d", h.State.ShotNumber(1))
	}
	off := false
	body, _ := json.Marshal(runtimeRequest{InactiveRanges: []int{}, WettkampfVisible: &off})
	req := httptest.NewRequest(http.MethodPut, "/api/runtime", strings.NewReader(string(body)))
	req.Header.Set("X-SR-Control-Token", "s3cret")
	rec := httptest.NewRecorder()
	h.ServeRuntime(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if h.State.ShotNumber(1) != 1 {
		t.Fatal("wettkampfVisible must not ResetRange")
	}
	if h.wettkampfVisible() {
		t.Fatal("expected hidden")
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.WettkampfVisible {
		t.Fatal("config response still visible")
	}
}
