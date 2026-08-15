package staticassets_test

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rulebookDoc struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Sections []struct {
		Heading    string   `json:"heading"`
		Paragraphs []string `json:"paragraphs"`
		Items      []string `json:"items"`
	} `json:"sections"`
}

func TestGamePluginsShipRulebookJSON(t *testing.T) {
	pluginsDir := filepath.Join(staticRoot(t), "..", "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	var games []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		raw, err := os.ReadFile(filepath.Join(pluginsDir, id, "manifest.xml"))
		if err != nil {
			continue
		}
		var man struct {
			Kind string `xml:"kind,attr"`
		}
		if err := xml.Unmarshal(raw, &man); err != nil {
			t.Fatalf("%s manifest: %v", id, err)
		}
		if man.Kind != "game" {
			if _, err := os.Stat(filepath.Join(pluginsDir, id, "rulebook.json")); err == nil {
				t.Errorf("%s is kind=%q but has rulebook.json — display plugins must not show Regeln", id, man.Kind)
			}
			continue
		}
		games = append(games, id)
		body := readRepoFile(t, "plugins", id, "rulebook.json")
		var book rulebookDoc
		if err := json.Unmarshal([]byte(body), &book); err != nil {
			t.Errorf("%s rulebook.json: %v", id, err)
			continue
		}
		if strings.TrimSpace(book.Title) == "" {
			t.Errorf("%s rulebook.json missing title", id)
		}
		if len(book.Sections) == 0 {
			t.Errorf("%s rulebook.json has no sections — overlay would be empty", id)
		}
		for i, sec := range book.Sections {
			if strings.TrimSpace(sec.Heading) == "" {
				t.Errorf("%s rulebook section %d missing heading", id, i)
			}
			if len(sec.Paragraphs) == 0 && len(sec.Items) == 0 {
				t.Errorf("%s rulebook section %q has no paragraphs or items", id, sec.Heading)
			}
		}
	}
	if len(games) == 0 {
		t.Fatal("no game plugins found")
	}
}

func TestRulebookOverlayMatchesQRModal(t *testing.T) {
	index := readRepoFile(t, "static", "index.html")
	mustContain(t, index, "id=\"rulebook-btn\"",
		"master slim rail needs a Regeln button like the per-range QR control")
	mustContain(t, index, "/rulebook.js",
		"index must load rulebook.js or the overlay never opens")
	js := readRepoFile(t, "static", "rulebook.js")
	mustContain(t, js, "qr-result-modal",
		"rulebook overlay must reuse the QR modal chrome so hall lighting/theme stay consistent")
	mustContain(t, js, "rulebook-dialog",
		"rulebook dialog class drives the wider text overlay")
	mustContain(t, js, "window.SRRulebook",
		"master.js talks to SRRulebook")
	master := readRepoFile(t, "static", "master.js")
	mustContain(t, master, "syncRulebookButtons",
		"plugin switch must show/hide Regeln for the active game")
	style := readRepoFile(t, "static", "style.css")
	mustContain(t, style, ".rulebook-dialog",
		"rulebook overlay needs CSS next to the QR modal")
}
