package api

import (
	"errors"
	"net/http"
	"time"

	"srdashboard/rwkbericht"
)

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
	w.Header().Set("Content-Disposition", `attachment; filename="RWK-Bericht.pdf"`)
	_, _ = w.Write(pdf)
}
