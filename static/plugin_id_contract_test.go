package staticassets_test

import (
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Plugin ids must match folder names, manifest ids, view.js registration, and
// the hall shell. A mismatch after a rename leaves SRPluginViews[id] undefined
// and the shared host blank.

type pluginManifestAttrs struct {
	XMLName xml.Name `xml:"plugin"`
	ID      string   `xml:"id,attr"`
	Label   string   `xml:"label,attr"`
	Mode    string   `xml:"mode,attr"`
	Kind    string   `xml:"kind,attr"`
}

func TestEveryPluginIdMatchesFolderManifestAndView(t *testing.T) {
	pluginsDir := filepath.Join(staticRoot(t), "..", "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	master := readRepoFile(t, "static", "master.js")
	shell := readRepoFile(t, "static", "plugin-shell.js")
	mustContain(t, shell, "window.SRPluginViews && window.SRPluginViews[pluginId]",
		"plugin-shell must paint via SRPluginViews[pluginId]")

	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		manPath := filepath.Join(pluginsDir, id, "manifest.xml")
		if _, err := os.Stat(manPath); err != nil {
			continue
		}
		found = append(found, id)
		raw := readRepoFile(t, "plugins", id, "manifest.xml")
		var man pluginManifestAttrs
		if err := xml.Unmarshal([]byte(raw), &man); err != nil {
			t.Fatalf("%s manifest: %v", id, err)
		}
		if man.ID != id {
			t.Errorf("%s: folder name %q != manifest id %q — /plugins/{id}/ URLs and Get(id) would miss the files", id, id, man.ID)
		}
		view := readRepoFile(t, "plugins", id, "view.js")
		registers := strings.Contains(view, "SRPluginViews['"+id+"']") ||
			strings.Contains(view, `SRPluginViews["`+id+`"]`) ||
			strings.Contains(view, "PLUGIN_ID = '"+id+"'") ||
			strings.Contains(view, `PLUGIN_ID = "`+id+`"`)
		if !registers {
			t.Errorf("%s: view.js does not register SRPluginViews for %q — hall/shooter would show JSON fallback", id, id)
		}
		for _, old := range []string{"f1-race", "maedn", "malefiz", "f1-race-master-host"} {
			if strings.Contains(view, old) {
				t.Errorf("%s: view.js still contains %q", id, old)
			}
			if strings.Contains(raw, old) {
				t.Errorf("%s: manifest still contains %q", id, old)
			}
		}
		if man.Mode == "shared" {
			mustContain(t, master, "id === '"+id+"'",
				id+" must appear in master.js shared-plugin / start-button wiring or the hall stays on the classic grid")
		}
	}
	want := []string{
		"autorennen", "barrikade", "classic-range", "fox-on-the-run",
		"ludo", "tannebaum-einzel", "tannebaum-team", "zehner-bingo",
	}
	got := strings.Join(found, ",")
	for _, id := range want {
		if !strings.Contains(","+got+",", ","+id+",") {
			t.Errorf("missing bundled plugin folder %s", id)
		}
	}
	for _, old := range []string{"f1-race", "maedn", "malefiz"} {
		if strings.Contains(","+got+",", ","+old+",") {
			t.Errorf("old plugin folder %s is still present", old)
		}
	}
}

func TestAutorennenCircuitAssetsMatchViewMap(t *testing.T) {
	js := readRepoFile(t, "plugins", "autorennen", "view.js")
	circuits := []string{"bergsee", "steinring", "hafenpark", "langring"}
	root := filepath.Join(staticRoot(t), "..", "plugins", "autorennen", "assets", "circuits")
	for _, id := range circuits {
		mustContain(t, js, id+": 'circuits/"+id+".svg'",
			"view.js must fetch the renamed circuit file or the track SVG 404s")
		for _, suffix := range []string{".svg", "-bg.png"} {
			path := filepath.Join(root, id+suffix)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("missing circuit asset %s: %v", path, err)
			}
		}
	}
	for _, old := range []string{"spa", "nuerburgring", "melbourne", "nordschleife"} {
		mustNotContain(t, js, old+":",
			"old circuit id in CIRCUIT_FILES would 404 after the file rename")
		if _, err := os.Stat(filepath.Join(root, old+".svg")); err == nil {
			t.Errorf("old circuit file %s.svg still present", old)
		}
	}
}

func TestOldPluginNamesGoneFromSource(t *testing.T) {
	root := filepath.Join(staticRoot(t), "..")
	forbidden := []string{"f1-race", "f1-race-master-host", "f1race", "maedn", "malefiz"}
	scanRoots := []string{
		filepath.Join(root, "plugins"),
		filepath.Join(root, "host", "games"),
		filepath.Join(root, "static"),
		filepath.Join(root, "cmd"),
		filepath.Join(root, "testdata"),
		filepath.Join(root, "config.xml"),
		filepath.Join(root, "main.go"),
	}
	extOK := map[string]bool{
		".go": true, ".js": true, ".css": true, ".xml": true, ".json": true,
		".html": true, ".md": true, ".svg": true, ".txt": true,
	}
	var hits []string
	scanFile := func(path string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if !extOK[strings.ToLower(filepath.Ext(path))] && filepath.Base(path) != "config.xml" {
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		body := string(b)
		rel, _ := filepath.Rel(root, path)
		for _, bad := range forbidden {
			if strings.Contains(body, bad) {
				hits = append(hits, rel+": "+bad)
			}
		}
	}
	for _, start := range scanRoots {
		info, err := os.Stat(start)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			scanFile(start)
			continue
		}
		_ = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			scanFile(path)
			return nil
		})
	}
	if len(hits) > 0 {
		t.Fatalf("old plugin names still in source (GUI would look up the wrong id):\n  %s", strings.Join(hits, "\n  "))
	}
}
