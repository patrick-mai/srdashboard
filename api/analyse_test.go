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

func TestAnalyseResultsSameShooterNewStart(t *testing.T) {
	h := portTestHandlers(t)
	sh := &state.ShotShooter{Firstname: "Anna", Lastname: "Müller"}
	menu := &struct {
		MenuPointName string `json:"MenuPointName"`
		MenuItemName  string `json:"MenuItemName"`
	}{MenuItemName: "LG 2 Schuss"}
	h.State.ApplyShot(1, &state.ShotPayload{DecValue: 10.4, FullValue: 10, Shooter: sh, MenuItem: menu})
	h.State.ApplyShot(1, &state.ShotPayload{DecValue: 9.1, FullValue: 9, Shooter: sh, MenuItem: menu})

	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, h, h.Hub, StaticOptions{AllowConfig: false})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d body %s", rec.Code, rec.Body.String())
	}
	var first struct {
		Results []state.SessionResultSummary `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 1 || !first.Results[0].Live {
		t.Fatalf("first start = %+v", first.Results)
	}
	oldID := first.Results[0].ID

	h.State.ApplyShot(1, &state.ShotPayload{DecValue: 8.0, FullValue: 8, Shooter: sh, MenuItem: menu})

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results", nil))
	var list struct {
		Results []state.SessionResultSummary `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Results) != 2 {
		t.Fatalf("same shooter new start: %+v", list.Results)
	}
	var liveID, archivedID string
	for _, row := range list.Results {
		if row.Live {
			liveID = row.ID
		} else if row.ShooterName == "Anna Müller" {
			archivedID = row.ID
		}
	}
	if archivedID != oldID || liveID == "" || liveID == oldID {
		t.Fatalf("ids live=%s archived=%s old=%s rows=%+v", liveID, archivedID, oldID, list.Results)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results?id="+archivedID, nil))
	var detail struct {
		ID    string              `json:"id"`
		Live  bool                `json:"live"`
		Range state.RangeSnapshot `json:"range"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Live || detail.Range.ShotNumber != 2 || detail.Range.ResultID != oldID {
		t.Fatalf("archived detail = %+v", detail)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results?id="+liveID, nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.Live || detail.Range.ShotNumber != 1 || detail.Range.ResultID != liveID {
		t.Fatalf("live detail = %+v", detail)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/analyse/results?id=live-1", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.Live || detail.ID != liveID || detail.Range.ShotNumber != 1 {
		t.Fatalf("live-1 alias should be the new start, got %+v", detail)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/qr?result="+archivedID+"&fmt=rr", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("qr archived status %d body %s", rec.Code, rec.Body.String())
	}
	var archivedQR, liveQR map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &archivedQR); err != nil {
		t.Fatal(err)
	}
	if archivedQR["result"] != archivedID {
		t.Fatalf("qr ?result= archived = %+v", archivedQR)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/qr?range=1&fmt=rr", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("qr live range status %d body %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &liveQR); err != nil {
		t.Fatal(err)
	}
	if liveQR["result"] != nil && liveQR["result"] != "" {
		t.Fatalf("qr ?range=1 must be the current Bahn, not a frozen id: %+v", liveQR)
	}
	if liveQR["url"] == archivedQR["url"] {
		t.Fatalf("live Bahn QR must not reuse the archived payload: %v", liveQR["url"])
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
