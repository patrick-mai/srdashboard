package api

import (
	"encoding/json"
	"net/http"
)

// HistoricStatus is returned by GET /api/historic (stub until ODBC layer exists).
type HistoricStatus struct {
	Available bool   `json:"available"`
	DSN       string `json:"dsn,omitempty"`
	Message   string `json:"message"`
}

// Historic returns whether historic scoring is configured. Data queries are not implemented yet.
func (h *Handlers) Historic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dsn := ""
	if h.Cfg != nil {
		dsn = h.Cfg.ODBCName
	}
	st := HistoricStatus{
		Available: false,
		DSN:       dsn,
		Message:   "Historic view is not implemented yet. ODBC DSN is stored in config for a future release.",
	}
	if dsn == "" {
		st.Message = "No ODBC DSN configured. Set odbcName in config.xml (Settings) when historic support ships."
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}
