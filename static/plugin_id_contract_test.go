package staticassets_test

import (
	"encoding/xml"
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
		if man.Mode == "shared" {
			mustContain(t, master, "id === '"+id+"'",
				id+" must appear in master.js shared-plugin / start-button wiring or the hall stays on the classic grid")
		}
	}
	want := []string{
		"autorennen", "barrikade", "classic-range", "fox-on-the-run",
		"ludo", "tannebaum-einzel", "tannebaum-team", "zehner-bingo",
	}
	wantSet := map[string]bool{}
	for _, id := range want {
		wantSet[id] = true
	}
	foundSet := map[string]bool{}
	for _, id := range found {
		foundSet[id] = true
		if !wantSet[id] {
			t.Errorf("unexpected bundled plugin folder %s", id)
		}
	}
	for _, id := range want {
		if !foundSet[id] {
			t.Errorf("missing bundled plugin folder %s", id)
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
}
