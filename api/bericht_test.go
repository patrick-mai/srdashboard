package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"srdashboard/state"
	"srdashboard/wettkampf"
)

func TestBerichtFilenameUsesTeamNames(t *testing.T) {
	got := berichtFilename("Adler I", "Mitte I")
	if got != "RWK_bericht_Adler I-Mitte I.pdf" {
		t.Fatalf("filename %q", got)
	}
	got = berichtFilename("SSV Öchtringhausen", "A/B")
	if got != "RWK_bericht_SSV Öchtringhausen-A_B.pdf" {
		t.Fatalf("filename %q", got)
	}
	if berichtFilename("", "  ") != "RWK_bericht_Mannschaft-Mannschaft.pdf" {
		t.Fatalf("empty names: %q", berichtFilename("", "  "))
	}
}

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
	if strings.Contains(rec.Body.String(), "DCTDecode") {
		t.Fatal("default report still embeds the JPEG crest")
	}
	wantName := "RWK_bericht_" + view.Teams[0].Name + "-SV Dinker.pdf"
	if disp := rec.Header().Get("Content-Disposition"); !strings.Contains(disp, wantName) {
		t.Fatalf("disposition %q, want %q", disp, wantName)
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=neutral", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("neutral status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=Ohne%20Kopf", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("ohne kopf status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Lippstadt") || strings.Contains(rec.Body.String(), "Soest") {
		t.Fatal("report still contains the Soest-Lippstadt club list")
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=soest", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("soest vorlage status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeWettkampfBericht(rec, httptest.NewRequest(http.MethodGet, url+"&vorlage=logo", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown vorlage status %d", rec.Code)
	}
}
