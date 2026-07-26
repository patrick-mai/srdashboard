package api

import (
	"encoding/json"
	"net/http"
	"reflect"

	"srdashboard/config"
)

// ConfigResponse is the JSON response for GET /api/config
type ConfigResponse struct {
	UDPPort       int                `json:"udpPort"`
	ODBCName      string             `json:"odbcName"`
	Ranges        int                `json:"ranges"`
	LayoutColumns int                `json:"layoutColumns"`
	DefaultTarget string             `json:"defaultTarget"`
	Footer        config.Footer      `json:"footer"`
	PluginsDir    string             `json:"pluginsDir"`
	ActivePlugin  string             `json:"activePlugin"`
	PluginPins    []config.PluginRef `json:"pluginPins"`
	ControlToken  string             `json:"controlToken,omitempty"`
	DefaultMode   string             `json:"defaultDisplayMode"`
	ShotStrokeWidth float64          `json:"shotStrokeWidth"`
}

type configSaveResponse struct {
	Status          string   `json:"status"`
	HotReload       bool     `json:"hotReload"`
	RestartRequired bool     `json:"restartRequired"`
	RestartFields   []string `json:"restartFields,omitempty"`
}

func (h *Handlers) configResponse() ConfigResponse {
	active := h.Cfg.Plugins.Active
	if h.PluginState != nil {
		if id := h.PluginState.ActivePluginID(); id != "" {
			active = id
		}
	}
	stroke := h.Cfg.Display.ShotStrokeWidth
	if stroke <= 0 {
		stroke = 0.1
	}
	return ConfigResponse{
		UDPPort:         h.Cfg.UDPPort,
		ODBCName:        h.Cfg.ODBCName,
		Ranges:          h.Cfg.Ranges,
		LayoutColumns:   h.Cfg.LayoutColumns,
		DefaultTarget:   h.Cfg.DefaultTarget,
		Footer:          h.Cfg.Footer,
		PluginsDir:      h.Cfg.Plugins.Dir,
		ActivePlugin:    active,
		PluginPins:      h.Cfg.Plugins.Plugin,
		ControlToken:    h.Cfg.Display.ControlToken,
		DefaultMode:     h.Cfg.Display.DefaultMode,
		ShotStrokeWidth: stroke,
	}
}

func (h *Handlers) responseToConfig(resp ConfigResponse) *config.Config {
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
	return &config.Config{
		UDPPort:       resp.UDPPort,
		ODBCName:      resp.ODBCName,
		Ranges:        resp.Ranges,
		LayoutColumns: resp.LayoutColumns,
		DefaultTarget: resp.DefaultTarget,
		Footer:        resp.Footer,
		Plugins: config.Plugins{
			Dir:    resp.PluginsDir,
			Active: active,
			Plugin: pins,
		},
		Display: config.Display{
			DefaultMode:     resp.DefaultMode,
			ControlToken:    resp.ControlToken,
			ShotStrokeWidth: resp.ShotStrokeWidth,
		},
	}
}

func restartFieldsForConfig(old, new *config.Config) []string {
	var fields []string
	if old.UDPPort != new.UDPPort {
		fields = append(fields, "udpPort")
	}
	if old.ODBCName != new.ODBCName {
		fields = append(fields, "odbcName")
	}
	if old.Ranges != new.Ranges {
		fields = append(fields, "ranges")
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
	h.Cfg.UDPPort = newCfg.UDPPort
	h.Cfg.ODBCName = newCfg.ODBCName
	h.Cfg.Ranges = newCfg.Ranges
	h.Cfg.LayoutColumns = newCfg.LayoutColumns
	h.Cfg.DefaultTarget = newCfg.DefaultTarget
	h.Cfg.Footer = newCfg.Footer
	h.Cfg.Plugins = newCfg.Plugins
	h.Cfg.Display = newCfg.Display
	if h.PluginState != nil && newCfg.Plugins.Active != "" &&
		h.PluginState.ActivePluginID() != newCfg.Plugins.Active {
		_ = h.PluginState.Activate(newCfg.Plugins.Active)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		newCfg := h.responseToConfig(req)
		oldSnapshot := *h.Cfg
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
	defaults := ap.Logic.DefaultConfig()
	overrides, err := config.LoadPluginConfig(h.Plugins.RootDir(), pluginID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	merged := h.Plugins.MergedConfig(pluginID)

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginConfigResponse{
			ID:               pluginID,
			Overrides:        overrides,
			ManifestDefaults: defaults,
			Merged:           merged,
			ConfigSchema:     ap.Logic.ConfigSchema(),
		})
	case http.MethodPut:
		if !h.checkControlToken(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var req pluginConfigSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
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
