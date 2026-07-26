package loader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"srdashboard/config"
	"srdashboard/host/logicapi"
)

const defaultInboxDir = "host/inbox"

// PluginInfo is a lightweight installed-plugin listing.
type PluginInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Mode    string `json:"mode"`
}

// ActivePlugin is a loaded plugin (manifest + optional WASM logic).
type ActivePlugin struct {
	Manifest *Manifest       `json:"manifest"`
	Dir      string          `json:"-"`
	Logic    logicapi.Logic  `json:"-"` // nil for display plugins
}

// Manager scans plugins/, loads optional logic.wasm, and installs zips.
type Manager struct {
	mu       sync.RWMutex
	rootDir  string
	inboxDir string
	loaded   map[string]*ActivePlugin
}

func NewManager(rootDir string) *Manager {
	if rootDir == "" {
		rootDir = "plugins"
	}
	inbox := defaultInboxDir
	_ = os.MkdirAll(inbox, 0755)
	_ = os.MkdirAll(rootDir, 0755)
	return &Manager{
		rootDir:  rootDir,
		inboxDir: inbox,
		loaded:   map[string]*ActivePlugin{},
	}
}

func (m *Manager) RootDir() string { return m.rootDir }

func (m *Manager) pluginDir(id string) string {
	return filepath.Join(m.rootDir, id)
}

func (m *Manager) ListInstalled() ([]PluginInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil, err
	}
	var out []PluginInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifest, err := LoadManifest(m.pluginDir(e.Name()))
		if err != nil {
			continue
		}
		out = append(out, PluginInfo{
			ID:      manifest.ID,
			Label:   manifest.Label,
			Version: manifest.Version,
			Kind:    manifest.Kind,
			Mode:    manifest.Mode,
		})
	}
	return out, nil
}

func (m *Manager) Get(id string) (*ActivePlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ap, ok := m.loaded[id]
	if !ok {
		return nil, fmt.Errorf("plugin %q not loaded", id)
	}
	return ap, nil
}

// GetActive is an alias kept for API compatibility.
func (m *Manager) GetActive(id string) (*ActivePlugin, error) {
	return m.Get(id)
}

func (m *Manager) ListLoaded() ([]*ActivePlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ActivePlugin, 0, len(m.loaded))
	for _, ap := range m.loaded {
		out = append(out, ap)
	}
	return out, nil
}

// ListActive returns all loaded plugins (installed folders with valid manifests).
func (m *Manager) ListActive() ([]*ActivePlugin, error) {
	return m.ListLoaded()
}

func (m *Manager) Install(zipPath string) (PluginInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := validateZip(zipPath)
	if err != nil {
		return PluginInfo{}, err
	}

	dest := m.pluginDir(manifest.ID)
	configBackup, _ := os.ReadFile(filepath.Join(dest, "config.xml"))
	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return PluginInfo{}, err
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return PluginInfo{}, err
	}
	if err := unzip(zipPath, dest); err != nil {
		_ = os.RemoveAll(dest)
		return PluginInfo{}, err
	}
	if len(configBackup) > 0 {
		_ = os.WriteFile(filepath.Join(dest, "config.xml"), configBackup, 0644)
	}
	// Prefer extracted folder's own manifest (may differ if zip had nested layout)
	loaded, err := LoadManifest(dest)
	if err != nil {
		_ = os.RemoveAll(dest)
		return PluginInfo{}, err
	}
	if err := loaded.ValidateFiles(dest); err != nil {
		_ = os.RemoveAll(dest)
		return PluginInfo{}, err
	}
	if err := m.loadPluginLocked(loaded.ID); err != nil {
		return PluginInfo{}, err
	}
	return PluginInfo{
		ID:      loaded.ID,
		Label:   loaded.Label,
		Version: loaded.Version,
		Kind:    loaded.Kind,
		Mode:    loaded.Mode,
	}, nil
}

func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ap, ok := m.loaded[id]; ok {
		if w, ok := ap.Logic.(*WasmLogic); ok {
			_ = w.Close(context.Background())
		}
		delete(m.loaded, id)
	}
	return os.RemoveAll(m.pluginDir(id))
}

func (m *Manager) MergedConfig(id string) map[string]any {
	ap, err := m.Get(id)
	if err != nil {
		return nil
	}
	cfg := map[string]any{}
	if ap.Manifest.Config.Defaults != nil {
		cfg = copyMap(ap.Manifest.Config.Defaults)
	}
	if ap.Logic != nil {
		for k, v := range ap.Logic.DefaultConfig() {
			if _, exists := cfg[k]; !exists {
				cfg[k] = v
			}
		}
	}
	overrides, err := config.LoadPluginConfig(m.rootDir, id)
	if err != nil {
		return cfg
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	return cfg
}

func (m *Manager) SaveToInbox(filename string, r io.Reader) (string, error) {
	name := filepath.Base(filename)
	if name == "" || name == "." {
		name = "upload.srplugin.zip"
	}
	dest := filepath.Join(m.inboxDir, name)
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = os.Remove(dest)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func (m *Manager) ScanInbox() ([]PluginInfo, error) {
	entries, err := os.ReadDir(m.inboxDir)
	if err != nil {
		return nil, err
	}
	var installed []PluginInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".srplugin.zip") && !strings.HasSuffix(lower, ".zip") {
			continue
		}
		path := filepath.Join(m.inboxDir, name)
		info, err := m.Install(path)
		if err != nil {
			continue
		}
		_ = os.Remove(path)
		installed = append(installed, info)
	}
	return installed, nil
}

// Reload rescans plugins/ and (re)loads manifests + optional WASM.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ap := range m.loaded {
		if w, ok := ap.Logic.(*WasmLogic); ok {
			_ = w.Close(context.Background())
		}
	}
	m.loaded = map[string]*ActivePlugin{}

	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if _, err := os.Stat(filepath.Join(m.pluginDir(id), "manifest.xml")); err != nil {
			continue
		}
		if err := m.loadPluginLocked(id); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d plugin(s) failed to load: %v", len(errs), errs)
	}
	return nil
}

func (m *Manager) loadPluginLocked(id string) error {
	dir := m.pluginDir(id)
	manifest, err := LoadManifest(dir)
	if err != nil {
		return err
	}
	if err := manifest.ValidateFiles(dir); err != nil {
		return err
	}
	var logic logicapi.Logic
	if manifest.HasLogic() {
		w, err := NewWasmLogic(context.Background(), manifest, dir)
		if err != nil {
			return err
		}
		logic = w
	}
	if old, ok := m.loaded[id]; ok {
		if w, ok := old.Logic.(*WasmLogic); ok {
			_ = w.Close(context.Background())
		}
	}
	m.loaded[id] = &ActivePlugin{Manifest: manifest, Dir: dir, Logic: logic}
	return nil
}

func (m *Manager) ActiveBaseURL(id string) (string, error) {
	if _, err := LoadManifest(m.pluginDir(id)); err != nil {
		return "", err
	}
	return "/plugins/" + id, nil
}

func validateZip(zipPath string) (*Manifest, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var manifestData []byte
	for _, f := range r.File {
		if filepath.Base(f.Name) == "manifest.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			manifestData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if manifestData == nil {
		return nil, fmt.Errorf("manifest.xml not found in zip")
	}
	manifest, err := ParseManifestXML(manifestData)
	if err != nil {
		return nil, err
	}
	if manifest.ID == "" || manifest.Version == "" {
		return nil, fmt.Errorf("invalid manifest")
	}
	return manifest, nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Detect single top-level directory wrapper
	prefix := ""
	var tops []string
	for _, f := range r.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if name == "." || name == "" {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			found := false
			for _, t := range tops {
				if t == parts[0] {
					found = true
					break
				}
			}
			if !found {
				tops = append(tops, parts[0])
			}
		}
	}
	if len(tops) == 1 {
		// If everything lives under one folder that isn't a file at root, strip it
		onlyDir := true
		for _, f := range r.File {
			name := filepath.ToSlash(filepath.Clean(f.Name))
			if name == tops[0] || strings.HasPrefix(name, tops[0]+"/") {
				continue
			}
			onlyDir = false
			break
		}
		if onlyDir {
			prefix = tops[0] + "/"
		}
	}

	for _, f := range r.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if strings.HasPrefix(name, "..") {
			continue
		}
		if prefix != "" {
			if name == strings.TrimSuffix(prefix, "/") {
				continue
			}
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" {
			continue
		}
		path := filepath.Join(dest, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(path, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
