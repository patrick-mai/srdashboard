package loader

import (
	"encoding/json"
	"os"
	"path/filepath"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

// DisplayLogic provides config defaults and schema for display plugins (no WASM scoring).
type DisplayLogic struct {
	id      string
	label   string
	version string
	defaults map[string]any
	schema   map[string]any
}

func NewDisplayLogic(manifest *Manifest, dir string) *DisplayLogic {
	schema := loadConfigSchema(dir)
	defaults := map[string]any{}
	if manifest.Config.Defaults != nil {
		defaults = copyMap(manifest.Config.Defaults)
	}
	return &DisplayLogic{
		id:       manifest.ID,
		label:    manifest.Label,
		version:  manifest.Version,
		defaults: defaults,
		schema:   schema,
	}
}

func loadConfigSchema(dir string) map[string]any {
	path := filepath.Join(dir, "config-schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return map[string]any{"type": "object"}
	}
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	return schema
}

func (d *DisplayLogic) ID() string      { return d.id }
func (d *DisplayLogic) Label() string   { return d.label }
func (d *DisplayLogic) Version() string { return d.version }

func (d *DisplayLogic) DefaultConfig() map[string]any {
	return copyMap(d.defaults)
}

func (d *DisplayLogic) ConfigSchema() map[string]any {
	return d.schema
}

func (d *DisplayLogic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	return logicapi.SessionState("{}"), nil
}

func (d *DisplayLogic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	return sess, nil, nil
}

func (d *DisplayLogic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	return map[string]any{}, nil
}

var _ logicapi.Logic = (*DisplayLogic)(nil)
