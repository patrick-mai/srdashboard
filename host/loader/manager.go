package loader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
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
	name := sanitizeUploadName(filename)
	dest := filepath.Join(m.inboxDir, name)
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	// Independent of any limit the HTTP layer applies, never let one upload
	// write more than the package limit into the inbox.
	if _, err := io.Copy(f, io.LimitReader(r, MaxPluginUploadBytes)); err != nil {
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

// sanitizeUploadName reduces a client-supplied filename to a bare, safe name.
// Both separators are stripped so a Windows-style path cannot survive on Linux.
func sanitizeUploadName(filename string) string {
	name := strings.ReplaceAll(filename, "\\", "/")
	name = path.Base(name)
	name = strings.TrimLeft(name, ".")
	if name == "" || strings.ContainsAny(name, `/\:*?"<>|`) {
		return "upload.srplugin.zip"
	}
	return name
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
	if manifest.IsBuiltin() {
		b, err := newBuiltinLogic(manifest)
		if err != nil {
			return err
		}
		logic = b
	} else if manifest.HasLogic() {
		w, err := NewWasmLogic(context.Background(), manifest, dir)
		if err != nil {
			return err
		}
		logic = w
	} else if manifest.Kind == KindDisplay {
		logic = NewDisplayLogic(manifest, dir)
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

// Limits applied to uploaded plugin packages.
const (
	// MaxPluginUploadBytes caps the compressed upload accepted by the install endpoint.
	MaxPluginUploadBytes = 64 << 20 // 64 MiB
	// MaxPluginUnpackedBytes caps the total extracted size, guarding against zip bombs.
	MaxPluginUnpackedBytes = 256 << 20 // 256 MiB
	// MaxPluginZipEntries caps the archive entry count.
	MaxPluginZipEntries = 10000
)

// checkZipEntryName rejects archive entries that try to escape the destination
// directory. Names are already slash-normalised and cleaned by the caller.
func checkZipEntryName(name string) error {
	switch {
	case name == "." || name == "":
		return nil
	case strings.HasPrefix(name, "/"):
		return fmt.Errorf("plugin package entry %q is an absolute path", name)
	case name == ".." || strings.HasPrefix(name, "../"):
		return fmt.Errorf("plugin package entry %q escapes the install directory", name)
	case filepath.IsAbs(filepath.FromSlash(name)) || filepath.VolumeName(filepath.FromSlash(name)) != "":
		return fmt.Errorf("plugin package entry %q is an absolute path", name)
	}
	return nil
}

// isWithin reports whether abs path child lives under abs path root.
func isWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

	if len(r.File) > MaxPluginZipEntries {
		return fmt.Errorf("plugin package has %d entries, limit is %d", len(r.File), MaxPluginZipEntries)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}

	var written int64
	for _, f := range r.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		// A tampered archive is rejected outright rather than silently
		// skipped, so a partial install can never look like a clean one.
		if err := checkZipEntryName(name); err != nil {
			return err
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
		path, err := filepath.Abs(filepath.Join(absDest, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if !isWithin(absDest, path) {
			return fmt.Errorf("plugin package entry %q escapes the install directory", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}
		if !f.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("plugin package entry %q is not a regular file", f.Name)
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
		// Copy through a shrinking budget so a zip bomb cannot fill the disk
		// however large the declared uncompressed sizes claim to be.
		n, err := io.Copy(out, io.LimitReader(rc, MaxPluginUnpackedBytes-written+1))
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		written += n
		if written > MaxPluginUnpackedBytes {
			return fmt.Errorf("plugin package expands beyond the %d byte limit", int64(MaxPluginUnpackedBytes))
		}
	}
	return nil
}
