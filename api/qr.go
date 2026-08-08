package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	qrcode "github.com/skip2/go-qrcode"

	"srdashboard/qrformat"
)

// QRFormats lists available result QR formats.
// GET /api/qr/formats
func (h *Handlers) QRFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type item struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	list := qrformat.List()
	out := make([]item, 0, len(list))
	for _, e := range list {
		out = append(out, item{ID: e.ID(), Label: e.Label()})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_ = json.NewEncoder(w).Encode(out)
}

// QR encodes a result QR for one range.
// GET /api/qr?range=N&fmt=rr → JSON { format, label, url }
// GET /api/qr.png?range=N&fmt=rr → PNG image (ECC M)
func (h *Handlers) QR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("range"))
	if err != nil || n < 1 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}
	fmtID := r.URL.Query().Get("fmt")
	if fmtID == "" {
		fmtID = "rr"
	}
	enc, ok := qrformat.Get(fmtID)
	if !ok {
		http.Error(w, "unknown format", http.StatusBadRequest)
		return
	}
	snap, ok := h.State.RangeSnapshot(n)
	if !ok {
		http.Error(w, "range not found", http.StatusNotFound)
		return
	}
	in := qrformat.FromRangeSnapshot(snap)
	url, err := enc.EncodeURL(in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	if r.URL.Path == "/api/qr.png" {
		png, err := qrcode.Encode(url, qrcode.Medium, 512)
		if err != nil {
			http.Error(w, "qr encode failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"format": enc.ID(),
		"label":  enc.Label(),
		"url":    url,
		"range":  n,
	})
}
