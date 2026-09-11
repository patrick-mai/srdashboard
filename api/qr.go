package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	qrcode "github.com/skip2/go-qrcode"

	"srdashboard/qrformat"
	"srdashboard/state"
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

// QR encodes a result QR for one range or a frozen session result.
// GET /api/qr?range=N&fmt=rr → JSON { format, label, url, range, json? }
// GET /api/qr?result=ID&fmt=rr → same, from the session archive
// GET /api/qr.png?range=N|result=ID&fmt=rr → PNG image (ECC M)
func (h *Handlers) QR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	snap, n, ok := lookupQRSnapshot(h.State, w, r)
	if !ok {
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

	out := map[string]any{
		"format": enc.ID(),
		"label":  enc.Label(),
		"url":    url,
		"range":  n,
	}
	if resultID := r.URL.Query().Get("result"); resultID != "" {
		out["result"] = resultID
	}
	if pe, ok := enc.(qrformat.PayloadJSONExporter); ok {
		if payload, err := pe.EncodePayloadJSON(in); err == nil {
			out["json"] = string(payload)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_ = json.NewEncoder(w).Encode(out)
}

func lookupQRSnapshot(st *state.LiveState, w http.ResponseWriter, r *http.Request) (state.RangeSnapshot, int, bool) {
	if st == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return state.RangeSnapshot{}, 0, false
	}
	if resultID := r.URL.Query().Get("result"); resultID != "" {
		_, snap, found := st.SessionResult(resultID)
		if !found {
			http.Error(w, "result not found", http.StatusNotFound)
			return state.RangeSnapshot{}, 0, false
		}
		return snap, snap.RangeNum, true
	}
	n, err := strconv.Atoi(r.URL.Query().Get("range"))
	if err != nil || n < 1 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return state.RangeSnapshot{}, 0, false
	}
	snap, found := st.RangeSnapshot(n)
	if !found {
		http.Error(w, "range not found", http.StatusNotFound)
		return state.RangeSnapshot{}, 0, false
	}
	return snap, n, true
}
