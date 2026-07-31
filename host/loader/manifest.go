package loader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"srdashboard/config"
)

const (
	ModePerRange = "per-range"
	ModeShared   = "shared" // reserved; v1 implements per-range only

	KindDisplay = "display"
	KindGame    = "game"
)

// Manifest describes a plugin package (manifest.xml inside the plugin folder / zip).
type Manifest struct {
	ID          string
	Label       string
	Version     string
	Mode        string // per-range | shared
	Kind        string // display | game
	Description string
	Entrypoints Entrypoints
	AssetsDir   string
	Config      ManifestConfig
}

type Entrypoints struct {
	Logic string // path to logic.wasm (optional for display)
	View  string // default view.js
	Theme string // optional theme.css
}

// ManifestConfig holds default config values shipped with the plugin.
type ManifestConfig struct {
	Defaults map[string]any
}

type manifestXML struct {
	XMLName     xml.Name `xml:"plugin"`
	ID          string   `xml:"id,attr"`
	Label       string   `xml:"label,attr"`
	Version     string   `xml:"version,attr"`
	Mode        string   `xml:"mode,attr"`
	Kind        string   `xml:"kind,attr"`
	Description string   `xml:"description"`
	Entrypoints struct {
		Logic      string `xml:"logic,attr"`
		View       string `xml:"view,attr"`
		Theme      string `xml:"theme,attr"`
		// Legacy attribute names (pre-v2 manifests)
		Wasm       string `xml:"wasm,attr"`
		ViewScript string `xml:"viewScript,attr"`
		ThemeCss   string `xml:"themeCss,attr"`
	} `xml:"entrypoints"`
	AssetsDir string `xml:"assetsDir"`
	Config    struct {
		Inner string `xml:",innerxml"`
	} `xml:"config"`
}

func ParseManifestXML(data []byte) (*Manifest, error) {
	var raw manifestXML
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	defaults, err := config.ParseConfigInnerXML(raw.Config.Inner)
	if err != nil {
		return nil, fmt.Errorf("manifest config: %w", err)
	}

	logic := firstNonEmpty(raw.Entrypoints.Logic, raw.Entrypoints.Wasm)
	view := firstNonEmpty(raw.Entrypoints.View, raw.Entrypoints.ViewScript)
	theme := firstNonEmpty(raw.Entrypoints.Theme, raw.Entrypoints.ThemeCss)

	m := &Manifest{
		ID:          raw.ID,
		Label:       raw.Label,
		Version:     raw.Version,
		Mode:        strings.ToLower(strings.TrimSpace(raw.Mode)),
		Kind:        strings.ToLower(strings.TrimSpace(raw.Kind)),
		Description: raw.Description,
		Entrypoints: Entrypoints{
			Logic: logic,
			View:  view,
			Theme: theme,
		},
		AssetsDir: raw.AssetsDir,
		Config:    ManifestConfig{Defaults: defaults},
	}
	if m.ID == "" || m.Version == "" {
		return nil, fmt.Errorf("manifest missing id or version")
	}
	if m.Mode == "" {
		m.Mode = ModePerRange
	}
	if m.Kind == "" {
		// Infer: display if no logic entrypoint, else game
		if m.Entrypoints.Logic == "" {
			m.Kind = KindDisplay
		} else {
			m.Kind = KindGame
		}
	}
	if m.Mode != ModePerRange && m.Mode != ModeShared {
		return nil, fmt.Errorf("manifest mode must be %q or %q", ModePerRange, ModeShared)
	}
	if m.Kind != KindDisplay && m.Kind != KindGame {
		return nil, fmt.Errorf("manifest kind must be %q or %q", KindDisplay, KindGame)
	}
	if m.Entrypoints.View == "" {
		m.Entrypoints.View = "view.js"
	}
	if m.Kind == KindGame && m.Entrypoints.Logic == "" {
		m.Entrypoints.Logic = "logic.wasm"
	}
	if m.AssetsDir == "" {
		m.AssetsDir = "assets"
	}
	return m, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.xml"))
	if err != nil {
		return nil, err
	}
	return ParseManifestXML(data)
}

func (m *Manifest) IsDisplay() bool { return m.Kind == KindDisplay }
func (m *Manifest) HasLogic() bool  { return m.Entrypoints.Logic != "" }
func (m *Manifest) IsBuiltin() bool {
	return strings.EqualFold(strings.TrimSpace(m.Entrypoints.Logic), "builtin")
}

func (m *Manifest) ValidateFiles(dir string) error {
	required := []string{
		filepath.Join(dir, m.Entrypoints.View),
		filepath.Join(dir, "manifest.xml"),
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing required file %s: %w", filepath.Base(p), err)
		}
	}
	if m.IsBuiltin() {
		return nil
	}
	if m.HasLogic() {
		wasmPath := filepath.Join(dir, m.Entrypoints.Logic)
		if _, err := os.Stat(wasmPath); err != nil {
			return fmt.Errorf("missing required file %s: %w", filepath.Base(wasmPath), err)
		}
	} else if m.Kind == KindGame {
		return fmt.Errorf("game plugin %q requires logic.wasm or logic=\"builtin\"", m.ID)
	}
	return nil
}
