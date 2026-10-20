package rwkbericht

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Vorlagen live as PDFs in the Classic Range Condensed plugin folder.
// The Schützenkreis Soest-Lippstadt letterhead is not included.
const (
	pluginID       = "classic-range-condensed"
	vorlagenRelDir = "plugins/" + pluginID
)

// Vorlage is one PDF form from the Classic Range Condensed plugin folder.
type Vorlage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ErrNoVorlage is returned when the folder is missing or has no PDF.
var ErrNoVorlage = errors.New("keine RWK-Vorlage")

type vorlageFile struct {
	Vorlage
	path string
}

// Vorlagen lists the PDFs in the Classic Range Condensed plugin folder, sorted by name.
// The first entry is the default when a report does not name a form.
func Vorlagen() ([]Vorlage, error) {
	files, err := vorlageFiles()
	if err != nil {
		return nil, err
	}
	out := make([]Vorlage, len(files))
	for i, f := range files {
		out[i] = f.Vorlage
	}
	return out, nil
}

func vorlageFiles() ([]vorlageFile, error) {
	dir, err := findVorlagenDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []vorlageFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, ok := vorlageID(e.Name())
		if !ok {
			continue
		}
		out = append(out, vorlageFile{
			Vorlage: Vorlage{ID: id, Name: id},
			path:    filepath.Join(dir, e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func vorlageID(name string) (string, bool) {
	if name != filepath.Base(name) {
		return "", false
	}
	ext := filepath.Ext(name)
	if !strings.EqualFold(ext, ".pdf") {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimSuffix(name, ext))
	if id == "" || strings.Contains(id, "..") {
		return "", false
	}
	return id, true
}

// pluginRoot is set from config so a custom plugins directory is used.
var pluginRoot string

// SetPluginDir points the report at pluginsDir/classic-range-condensed.
// An empty dir uses the default plugins folder next to the program.
func SetPluginDir(pluginsDir string) {
	pluginsDir = strings.TrimSpace(pluginsDir)
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}
	pluginRoot = filepath.Join(pluginsDir, pluginID)
}

func findVorlagenDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("SR_RWK_VORLAGEN")); d != "" {
		if dirExists(d) {
			return d, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNoVorlage, d)
	}
	var candidates []string
	if pluginRoot != "" {
		candidates = append(candidates, pluginRoot)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), vorlagenRelDir))
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			candidates = append(candidates, filepath.Join(dir, vorlagenRelDir))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if dirExists(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: Plugin %q fehlt", ErrNoVorlage, pluginID)
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func templateByID(id string) ([]byte, error) {
	files, err := vorlageFiles()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoVorlage
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return os.ReadFile(files[0].path)
	}
	for _, f := range files {
		if f.ID == id {
			return os.ReadFile(f.path)
		}
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownVorlage, id)
}
