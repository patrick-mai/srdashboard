package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"srdashboard/state"
)

func TestAnalyseResultsSurviveShooterChange(t *testing.T) {
	h := portTestHandlers(t)
	h.State.ApplyShot(1, &state.ShotPayload{
		DecValue: 10.4, FullValue: 10,
		Shooter: &state.ShotShooter{Firstname: "Anna", Lastname: "Müller"},
	})
	h.State.ApplyShot(1, &state.ShotPayload{
		DecValue: 9.0, FullValue: 9,
		Shooter: &state.ShotShooter{Firstname: "Jonas", Lastname: "Becker"},
	})

	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, h, h.Hub, StaticOptions{AllowConfig: false})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d body %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Results []state.SessionResultSummary `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Results) != 2 {
		t.Fatalf("results = %+v", list.Results)
	}
	var archivedID string
	for _, row := range list.Results {
		if !row.Live && row.ShooterName == "Anna Müller" {
			archivedID = row.ID
		}
	}
	if archivedID == "" {
		t.Fatalf("missing archived Anna: %+v", list.Results)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results?id="+archivedID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status %d body %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		ID    string              `json:"id"`
		Live  bool                `json:"live"`
		Range state.RangeSnapshot `json:"range"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Live || detail.Range.ShooterName != "Anna Müller" || detail.Range.ShotNumber != 1 {
		t.Fatalf("detail = %+v", detail)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/qr?result="+archivedID+"&fmt=rr", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("qr status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyseResultsUnknownID(t *testing.T) {
	h := portTestHandlers(t)
	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, h, h.Hub, StaticOptions{AllowConfig: false})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results?id=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}
