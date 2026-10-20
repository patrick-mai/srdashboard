package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"srdashboard/rwkbericht"
	"srdashboard/wettkampf"
)

// ServeWettkampfVorlagen handles GET /api/wettkampf/vorlagen.
func (h *Handlers) ServeWettkampfVorlagen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := rwkbericht.Vorlagen()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []rwkbericht.Vorlage{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// ServeWettkampfBericht handles GET /api/wettkampf/bericht.pdf?heim=&gast=.
func (h *Handlers) ServeWettkampfBericht(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	heim := r.URL.Query().Get("heim")
	gast := r.URL.Query().Get("gast")
	bericht, err := rwkbericht.FromSnapshot(h.wettkampfView(), heim, gast, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pdf, err := rwkbericht.Fill(bericht, r.URL.Query().Get("vorlage"))
	if err != nil {
		if errors.Is(err, rwkbericht.ErrUnknownVorlage) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", berichtDisposition(h.wettkampfView(), heim, gast))
	_, _ = w.Write(pdf)
}

// berichtDisposition names the file RWK_bericht_<Heim>-<Auswärts>.pdf.
func berichtDisposition(snap wettkampf.Snapshot, heimID, gastID string) string {
	name := berichtFilename(teamName(snap, heimID), teamName(snap, gastID))
	return `attachment; filename="` + asciiFilename(name) + `"; filename*=UTF-8''` + url.PathEscape(name)
}

func berichtFilename(heim, gast string) string {
	return "RWK_bericht_" + filenamePart(heim) + "-" + filenamePart(gast) + ".pdf"
}

func teamName(snap wettkampf.Snapshot, id string) string {
	for _, team := range snap.Teams {
		if team.ID == id {
			return team.Name
		}
	}
	return ""
}

func filenamePart(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteByte('_')
		default:
			if r < 32 {
				continue
			}
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "Mannschaft"
	}
	return s
}

func asciiFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
