package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"

	"srdashboard/config"
)

// ConfigResponse is the JSON body for GET and PUT /api/config.
//
// ControlToken is write-only: GET never returns it (it is the credential for
// every state-changing endpoint), and a PUT that omits it leaves the stored
// token untouched. Clients read ControlTokenSet to know whether one is
// configured. Sending an explicit empty string clears the token.
type ConfigResponse struct {
	UDPPort         int                `json:"udpPort"`
	ODBCName        string             `json:"odbcName"`
	Ranges          int                `json:"ranges"`
	LayoutColumns   int                `json:"layoutColumns"`
	Footer          config.Footer      `json:"footer"`
	PluginsDir      string             `json:"pluginsDir"`
	ActivePlugin    string             `json:"activePlugin"`
	PluginPins      []config.PluginRef `json:"pluginPins"`
	ControlToken    *string            `json:"controlToken,omitempty"`
	ControlTokenSet bool               `json:"controlTokenSet"`
	DefaultMode     string             `json:"defaultDisplayMode"`
	ShotStrokeWidth float64            `json:"shotStrokeWidth"`
}

type configSaveResponse struct {
	Status          string   `json:"status"`
	HotReload       bool     `json:"hotReload"`
	RestartRequired bool     `json:"restartRequired"`
	RestartFields   []string `json:"restartFields,omitempty"`
}

func (h *Handlers) configResponse() ConfigResponse {
	cfg := h.cfgSnapshot()
	active := cfg.Plugins.Active
	if h.PluginState != nil {
		if id := h.PluginState.ActivePluginID(); id != "" {
			active = id
		}
	}
	stroke := cfg.Display.ShotStrokeWidth
	if stroke <= 0 {
		stroke = 0.1
	}
	return ConfigResponse{
		UDPPort:         cfg.UDPPort,
		ODBCName:        cfg.ODBCName,
		Ranges:          cfg.Ranges,
		LayoutColumns:   cfg.LayoutColumns,
		Footer:          cfg.Footer,
		PluginsDir:      cfg.Plugins.Dir,
		ActivePlugin:    active,
		PluginPins:      cfg.Plugins.Plugin,
		ControlTokenSet: cfg.Display.ControlToken != "",
		DefaultMode:     cfg.Display.DefaultMode,
		ShotStrokeWidth: stroke,
	}
}

// responseToConfig builds the config to persist. Fields the client cannot see
// (the control token) are carried over from old unless explicitly supplied.
func (h *Handlers) responseToConfig(resp ConfigResponse, old *config.Config) *config.Config {
	pins := make([]config.PluginRef, 0, len(resp.PluginPins))
	for _, p := range resp.PluginPins {
		if p.ID != "" {
			pins = append(pins, p)
		}
	}
	active := resp.ActivePlugin
	if active == "" {
		active = "classic-range"
	}
	token := old.Display.ControlToken
	if resp.ControlToken != nil {
		token = *resp.ControlToken
	}
	return &config.Config{
		UDPPort:       resp.UDPPort,
		ODBCName:      resp.ODBCName,
		Ranges:        resp.Ranges,
		LayoutColumns: resp.LayoutColumns,
		Footer:        resp.Footer,
		Plugins: config.Plugins{
			Dir:    resp.PluginsDir,
			Active: active,
			Plugin: pins,
		},
		Display: config.Display{
			DefaultMode:     resp.DefaultMode,
			ControlToken:    token,
			ShotStrokeWidth: resp.ShotStrokeWidth,
		},
	}
}

// validateConfig rejects values that would leave the server in a broken state.
func validateConfig(c *config.Config) error {
	switch {
	case c.UDPPort < 1 || c.UDPPort > 65535:
		return fmt.Errorf("udpPort must be between 1 and 65535, got %d", c.UDPPort)
	case c.Ranges < 1 || c.Ranges > maxRanges:
		return fmt.Errorf("ranges must be between 1 and %d, got %d", maxRanges, c.Ranges)
	case c.LayoutColumns < 1 || c.LayoutColumns > maxRanges:
		return fmt.Errorf("layoutColumns must be between 1 and %d, got %d", maxRanges, c.LayoutColumns)
	case c.Display.ShotStrokeWidth < 0 || c.Display.ShotStrokeWidth > 5:
		return fmt.Errorf("shotStrokeWidth must be between 0 and 5, got %g", c.Display.ShotStrokeWidth)
	case strings.TrimSpace(c.Plugins.Dir) == "":
		return fmt.Errorf("pluginsDir must not be empty")
	}
	return nil
}

// maxRanges is a sanity ceiling; real ranges have far fewer lanes.
const maxRanges = 256

func restartFieldsForConfig(old, new *config.Config) []string {
	var fields []string
	if old.UDPPort != new.UDPPort {
		fields = append(fields, "udpPort")
	}
	if old.ODBCName != new.ODBCName {
		fields = append(fields, "odbcName")
	}
	if old.Plugins.Dir != new.Plugins.Dir {
		fields = append(fields, "pluginsDir")
	}
	if !reflect.DeepEqual(old.Plugins.Plugin, new.Plugins.Plugin) {
		fields = append(fields, "pluginPins")
	}
	return fields
}

func (h *Handlers) applyConfigHotReload(newCfg *config.Config) {
	h.mutateCfg(func(c *config.Config) {
		c.UDPPort = newCfg.UDPPort
		c.ODBCName = newCfg.ODBCName
		c.Ranges = newCfg.Ranges
		c.LayoutColumns = newCfg.LayoutColumns
		c.Footer = newCfg.Footer
		c.Plugins = newCfg.Plugins
		c.Display = newCfg.Display
	})
	h.syncRangeCount(newCfg.Ranges)
	if h.PluginState != nil && newCfg.Plugins.Active != "" &&
		h.PluginState.ActivePluginID() != newCfg.Plugins.Active {
		if err := h.PluginState.Activate(newCfg.Plugins.Active); err != nil {
			log.Printf("activate plugin %q after config save: %v", newCfg.Plugins.Active, err)
		}
	}
}

// syncRangeCount resizes live + plugin state to the configured lane count.
// Safe to call repeatedly; no-ops when already matching.
func (h *Handlers) syncRangeCount(n int) {
	if n < 1 {
		return
	}
	if h.State != nil {
		h.State.SetNumRanges(n)
	}
	if h.PluginState != nil && h.PluginState.NumRanges() != n {
		if err := h.PluginState.SetNumRanges(n); err != nil {
			log.Printf("resize plugin state to %d ranges: %v", n, err)
		}
	}
}

// Config returns or updates the application configuration.
func (h *Handlers) Config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.configResponse())
	case http.MethodPut:
		if !h.checkControlToken(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if h.ConfigPath == "" {
			http.Error(w, "config path not configured", http.StatusInternalServerError)
			return
		}
		var req ConfigResponse
		if !decodeJSONBody(w, r, &req) {
			return
		}
		// Serialise the read-modify-write so two concurrent saves cannot
		// interleave and leave config.xml and h.Cfg disagreeing.
		h.saveMu.Lock()
		defer h.saveMu.Unlock()

		oldSnapshot := h.cfgSnapshot()
		newCfg := h.responseToConfig(req, &oldSnapshot)
		if err := validateConfig(newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		restartFields := restartFieldsForConfig(&oldSnapshot, newCfg)

		if err := config.Save(h.ConfigPath, newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.applyConfigHotReload(newCfg)

		if len(restartFields) == 0 {
			if h.Hub != nil {
				h.Hub.BroadcastAll(map[string]any{"type": "config_changed", "config": h.configResponse()})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(configSaveResponse{
			Status:          "ok",
			HotReload:       len(restartFields) == 0,
			RestartRequired: len(restartFields) > 0,
			RestartFields:   restartFields,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type pluginConfigResponse struct {
	ID                string         `json:"id"`
	Overrides         map[string]any `json:"overrides"`
	ManifestDefaults  map[string]any `json:"manifestDefaults"`
	Merged            map[string]any `json:"merged"`
	ConfigSchema      map[string]any `json:"configSchema"`
}

type pluginConfigSaveRequest struct {
	Merged    map[string]any `json:"merged"`
	Overrides map[string]any `json:"overrides"`
}

// PluginConfig handles GET/PUT /api/plugins/:id/config.
func (h *Handlers) PluginConfig(w http.ResponseWriter, r *http.Request, pluginID string) {
	ap, err := h.Plugins.GetActive(pluginID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defaults := map[string]any{}
	if ap.Logic != nil {
		defaults = ap.Logic.DefaultConfig()
	} else if ap.Manifest.Config.Defaults != nil {
		defaults = ap.Manifest.Config.Defaults
	}
	overrides, err := config.LoadPluginConfig(h.Plugins.RootDir(), pluginID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	merged := h.Plugins.MergedConfig(pluginID)

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		schema := map[string]any{"type": "object"}
		if ap.Logic != nil {
			schema = ap.Logic.ConfigSchema()
		}
		_ = json.NewEncoder(w).Encode(pluginConfigResponse{
			ID:               pluginID,
			Overrides:        overrides,
			ManifestDefaults: defaults,
			Merged:           merged,
			ConfigSchema:     schema,
		})
	case http.MethodPut:
		if !h.checkControlToken(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var req pluginConfigSaveRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		toSave := req.Overrides
		if toSave == nil {
			if req.Merged == nil {
				http.Error(w, "merged or overrides required", http.StatusBadRequest)
				return
			}
			toSave = config.DiffOverrides(defaults, req.Merged)
		}
		if err := config.SavePluginConfig(h.Plugins.RootDir(), pluginID, toSave); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"overrides": toSave,
			"merged":    mergeConfigMaps(defaults, toSave),
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func mergeConfigMaps(base, overrides map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
