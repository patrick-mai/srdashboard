package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srdashboard/state"
	"srdashboard/wettkampf"
)

func TestWettkampfBerichtRequiresTeams(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	store, err := wettkampf.Open("")
	if err != nil {
		t.Fatal(err)
	}
	h.Wettkampf = store
	rec := httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, "/api/wettkampf/bericht.pdf", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestWettkampfBerichtPDF(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	store, err := wettkampf.Open("")
	if err != nil {
		t.Fatal(err)
	}
	h.Wettkampf = store
	h.State.ApplyShot(1, &state.ShotPayload{
		FullValue: 10, DecValue: 10.4, Range: 1,
		Shooter: &state.ShotShooter{
			Firstname: "Anna", Lastname: "Müller",
			Club: &state.ShotClub{Name: "SSC Weslarn"},
		},
		MenuItem: &struct {
			MenuPointName string `json:"MenuPointName"`
			MenuItemName  string `json:"MenuItemName"`
		}{MenuItemName: "LG 40 Schuss"},
	})
	snap, _ := h.State.RangeSnapshot(1)
	h.ObserveWettkampfShot(snap)
	view := store.View()
	if len(view.Teams) != 1 {
		t.Fatalf("teams %#v", view.Teams)
	}
	// A second team with no shooters still has to be addressable. Create it via the menu patch.
	view = store.Apply(wettkampf.Patch{Teams: []wettkampf.Team{
		{ID: view.Teams[0].ID, Name: view.Teams[0].Name},
		{ID: "", Name: "SV Dinker"},
	}})
	var gast string
	for _, team := range view.Teams {
		if team.ID != view.Teams[0].ID && strings.Contains(team.Name, "Dinker") {
			gast = team.ID
		}
	}
	if gast == "" || view.Teams[0].ID == "" {
		t.Fatalf("ids heim %q gast %q teams %#v", view.Teams[0].ID, gast, view.Teams)
	}
	url := "/api/wettkampf/bericht.pdf?heim=" + view.Teams[0].ID + "&gast=" + gast
	rec := httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content-type %q", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatal("body is not a pdf")
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=neutral", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("neutral status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "DCTDecode") {
		t.Fatal("neutral download still embeds the JPEG crest")
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=logo", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown vorlage status %d", rec.Code)
	}
}
