package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"srdashboard/host/logicapi"
	"srdashboard/state"
)

// WasmLogic runs plugin scoring from a WASM module (portable “DLL”).
// Expected exports (JSON in/out via alloc + ptr/len):
//
//	init(configJSON) → stateJSON
//	on_shot(payloadJSON) → {state, events}
//	view_model(payloadJSON) → object
type WasmLogic struct {
	Manifest *Manifest
	Dir      string
	runtime  wazero.Runtime
	module   api.Module
}

func NewWasmLogic(ctx context.Context, manifest *Manifest, dir string) (*WasmLogic, error) {
	if manifest == nil || !manifest.HasLogic() {
		return nil, fmt.Errorf("no logic.wasm for plugin %q", manifest.ID)
	}
	wasmPath := filepath.Join(dir, manifest.Entrypoints.Logic)
	data, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, err
	}

	r := wazero.NewRuntime(ctx)
	mod, err := r.Instantiate(ctx, data)
	if err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("wasm instantiate: %w", err)
	}

	w := &WasmLogic{Manifest: manifest, Dir: dir, runtime: r, module: mod}
	for _, name := range []string{"init", "on_shot", "view_model"} {
		if mod.ExportedFunction(name) == nil {
			_ = r.Close(ctx)
			return nil, fmt.Errorf("wasm module missing %s export", name)
		}
	}
	if mod.ExportedFunction("alloc") == nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("wasm module missing alloc export")
	}
	return w, nil
}

func (w *WasmLogic) ID() string      { return w.Manifest.ID }
func (w *WasmLogic) Label() string   { return w.Manifest.Label }
func (w *WasmLogic) Version() string { return w.Manifest.Version }

func (w *WasmLogic) DefaultConfig() map[string]any {
	if w.Manifest.Config.Defaults != nil {
		return copyMap(w.Manifest.Config.Defaults)
	}
	return map[string]any{}
}

func (w *WasmLogic) ConfigSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func (w *WasmLogic) Init(cfg map[string]any) (logicapi.SessionState, error) {
	cfgJSON, _ := json.Marshal(cfg)
	result, err := w.callJSON("init", string(cfgJSON))
	if err != nil {
		return nil, err
	}
	return logicapi.SessionState(result), nil
}

func (w *WasmLogic) OnShot(sess logicapi.SessionState, rangeNum int, shot state.Shot, shotIndex int) (logicapi.SessionState, []logicapi.PluginEvent, error) {
	payload := map[string]any{
		"state":     json.RawMessage(sess),
		"rangeNum":  rangeNum,
		"shot":      shot,
		"shotIndex": shotIndex,
	}
	b, _ := json.Marshal(payload)
	result, err := w.callJSON("on_shot", string(b))
	if err != nil {
		return sess, nil, err
	}
	var resp struct {
		State  json.RawMessage        `json:"state"`
		Events []logicapi.PluginEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return logicapi.SessionState(result), nil, nil
	}
	return logicapi.SessionState(resp.State), resp.Events, nil
}

func (w *WasmLogic) ViewModel(sess logicapi.SessionState, rangeNum int) (map[string]any, error) {
	payload := map[string]any{
		"state":    json.RawMessage(sess),
		"rangeNum": rangeNum,
	}
	b, _ := json.Marshal(payload)
	result, err := w.callJSON("view_model", string(b))
	if err != nil {
		return nil, err
	}
	var vm map[string]any
	if err := json.Unmarshal([]byte(result), &vm); err != nil {
		return nil, err
	}
	return vm, nil
}

func (w *WasmLogic) callJSON(export, input string) (string, error) {
	fn := w.module.ExportedFunction(export)
	if fn == nil {
		return "", fmt.Errorf("wasm export %s not found", export)
	}
	inPtr := w.writeString(input)
	results, err := fn.Call(context.Background(), uint64(inPtr), uint64(len(input)))
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	outPtr := uint32(results[0])
	return w.readString(outPtr), nil
}

func (w *WasmLogic) writeString(s string) uint32 {
	mem := w.module.Memory()
	if mem == nil {
		return 0
	}
	alloc := w.module.ExportedFunction("alloc")
	if alloc == nil {
		return 0
	}
	res, err := alloc.Call(context.Background(), uint64(len(s)))
	if err != nil || len(res) == 0 {
		return 0
	}
	ptr := uint32(res[0])
	_ = mem.Write(ptr, []byte(s))
	return ptr
}

func (w *WasmLogic) readString(ptr uint32) string {
	mem := w.module.Memory()
	if mem == nil || ptr == 0 {
		return ""
	}
	buf, ok := mem.Read(ptr, 65536)
	if !ok {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func (w *WasmLogic) Close(ctx context.Context) error {
	if w.runtime != nil {
		return w.runtime.Close(ctx)
	}
	return nil
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
