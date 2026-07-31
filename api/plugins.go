package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"srdashboard/config"
	"srdashboard/host/loader"
)

func saveActivePlugin(path string, cfg *config.Config) error {
	return config.Save(path, cfg)
}

type activateRequest struct {
	ID string `json:"id"`
}

// checkControlToken authorises a state-changing request. An empty configured
// token leaves the endpoints open, which is the documented default for isolated
// range networks; main.go warns about it at startup.
func (h *Handlers) checkControlToken(r *http.Request) bool {
	token := h.cfgSnapshot().Display.ControlToken
	if token == "" {
		return true
	}
	got := r.Header.Get("X-SR-Control-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func (h *Handlers) PluginsActiveList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	activeID := h.cfgSnapshot().Plugins.Active
	if h.PluginState != nil {
		if id := h.PluginState.ActivePluginID(); id != "" {
			activeID = id
		}
	}
	ap, err := h.Plugins.Get(activeID)
	if err != nil {
		// Fall back to listing nothing rather than 500 when misconfigured
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	item := map[string]any{
		"id":         ap.Manifest.ID,
		"label":      ap.Manifest.Label,
		"version":    ap.Manifest.Version,
		"description": ap.Manifest.Description,
		"kind":       ap.Manifest.Kind,
		"mode":       ap.Manifest.Mode,
		"defaults":   h.Plugins.MergedConfig(ap.Manifest.ID),
		"viewUrl":    pluginAssetURL(ap.Manifest.ID, ap.Manifest.Entrypoints.View),
		"themeUrl":   "",
		"assetsBase": pluginAssetURL(ap.Manifest.ID, ap.Manifest.AssetsDir),
	}
	if ap.Manifest.Entrypoints.Theme != "" {
		item["themeUrl"] = pluginAssetURL(ap.Manifest.ID, ap.Manifest.Entrypoints.Theme)
	}
	if ap.Logic != nil {
		item["configSchema"] = ap.Logic.ConfigSchema()
	} else {
		item["configSchema"] = map[string]any{"type": "object"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]any{item})
}

func (h *Handlers) PluginSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if rng := r.URL.Query().Get("range"); rng != "" {
		n, err := strconv.Atoi(rng)
		if err != nil {
			http.Error(w, "invalid range", http.StatusBadRequest)
			return
		}
		snap := h.PluginState.SnapshotRange(n)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
		return
	}
	snaps, active := h.PluginState.SnapshotAll()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessions": snaps,
		"active":   active,
		// Back-compat
		"match": map[string]any{"active": active.Active, "pluginId": active.PluginID},
	})
}

func (h *Handlers) PluginsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.Plugins.ListInstalled()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handlers) PluginByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 && parts[0] != "" {
		if r.Method == http.MethodDelete {
			if !h.checkControlToken(r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			id := parts[0]
			if h.PluginState != nil && h.PluginState.ActivePluginID() == id {
				http.Error(w, "cannot uninstall the active plugin", http.StatusBadRequest)
				return
			}
			if err := h.Plugins.Uninstall(id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ap, err := h.Plugins.Get(parts[0])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"id":          parts[0],
			"label":       ap.Manifest.Label,
			"version":     ap.Manifest.Version,
			"description": ap.Manifest.Description,
			"kind":        ap.Manifest.Kind,
			"mode":        ap.Manifest.Mode,
			"defaults":    h.Plugins.MergedConfig(parts[0]),
			"manifest":    ap.Manifest,
			"viewUrl":     pluginAssetURL(parts[0], ap.Manifest.Entrypoints.View),
			"assetsBase":  pluginAssetURL(parts[0], ap.Manifest.AssetsDir),
		}
		if ap.Logic != nil {
			resp["configSchema"] = ap.Logic.ConfigSchema()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	if len(parts) == 2 && parts[1] == "config" {
		h.PluginConfig(w, r, parts[0])
		return
	}
	http.NotFound(w, r)
}

func (h *Handlers) PluginInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	filename := r.Header.Get("X-Filename")
	if filename == "" {
		filename = "upload.srplugin.zip"
	}
	body := http.MaxBytesReader(w, r.Body, loader.MaxPluginUploadBytes)
	if _, err := h.Plugins.SaveToInbox(filename, body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "plugin package too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	list, err := h.Plugins.ScanInbox()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Plugins.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Re-bind active plugin after reload
	if h.PluginState != nil {
		h.syncRangeCount(h.cfgSnapshot().Ranges)
		_ = h.PluginState.EnsureActive()
	}
	if h.Hub != nil {
		h.Hub.BroadcastAll(map[string]any{"type": "plugins_changed"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handlers) PluginActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req activateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if _, err := h.Plugins.Get(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.syncRangeCount(h.cfgSnapshot().Ranges)
	if err := h.PluginState.Activate(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.mutateCfg(func(c *config.Config) { c.Plugins.Active = req.ID })
	if h.ConfigPath != "" {
		snap := h.cfgSnapshot()
		if err := saveActivePlugin(h.ConfigPath, &snap); err != nil {
			log.Printf("persist active plugin %q: %v", req.ID, err)
		}
	}
	if h.Hub != nil {
		h.Hub.BroadcastAll(map[string]any{"type": "plugins_changed"})
		h.PluginState.Notify()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "active": req.ID})
}

func (h *Handlers) PluginReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := h.Plugins.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.syncRangeCount(h.cfgSnapshot().Ranges)
	if err := h.PluginState.EnsureActive(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.Hub != nil {
		h.Hub.BroadcastAll(map[string]any{"type": "plugins_changed"})
		h.PluginState.Notify()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "active": h.PluginState.ActivePluginID()})
}

func (h *Handlers) PluginScanInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkControlToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	list, err := h.Plugins.ScanInbox()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Plugins.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.syncRangeCount(h.cfgSnapshot().Ranges)
	_ = h.PluginState.EnsureActive()
	if h.Hub != nil {
		h.Hub.BroadcastAll(map[string]any{"type": "plugins_changed"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func pluginAssetURL(id, rest string) string {
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return "/plugins/" + id
	}
	return "/plugins/" + id + "/" + rest
}

// ServePlugin serves plugin assets at /plugins/:id/* (never config.xml or wasm as editable config).
func (h *Handlers) ServePlugin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/plugins/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	id, rest := parts[0], parts[1]
	if rest == "config.xml" || strings.HasSuffix(rest, "/config.xml") {
		http.NotFound(w, r)
		return
	}
	// ServeMux and ServeFile already reject "..", but the containment check is
	// what actually guarantees we never read outside the plugin directory.
	root, err := filepath.Abs(h.Plugins.RootDir())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	filePath, err := filepath.Abs(filepath.Join(root, id, filepath.FromSlash(rest)))
	if err != nil || !isWithin(root, filePath) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filePath)
}

// isWithin reports whether abs path child lives under abs path root.
func isWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
