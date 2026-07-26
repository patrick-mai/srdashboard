package loader

import "testing"

func TestParseManifestXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plugin id="demo" label="Demo" version="1.0.0" mode="per-range" kind="game">
  <description>Demo game</description>
  <entrypoints logic="logic.wasm" view="view.js"/>
  <assetsDir>assets</assetsDir>
  <config>
    <frames>10</frames>
    <pinsPerFrame>10</pinsPerFrame>
  </config>
</plugin>`)

	m, err := ParseManifestXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "demo" || m.Version != "1.0.0" {
		t.Fatalf("id/version = %q / %q", m.ID, m.Version)
	}
	if m.Config.Defaults["frames"] != 10 {
		t.Fatalf("frames = %v", m.Config.Defaults["frames"])
	}
	if m.Entrypoints.Logic != "logic.wasm" {
		t.Fatalf("logic = %q", m.Entrypoints.Logic)
	}
}

func TestParseManifestLegacyAttrs(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plugin id="legacy" label="Legacy" version="1.0.0">
  <entrypoints wasm="logic.wasm" viewScript="view.js" themeCss="theme.css"/>
</plugin>`)
	m, err := ParseManifestXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Entrypoints.Logic != "logic.wasm" || m.Entrypoints.View != "view.js" || m.Entrypoints.Theme != "theme.css" {
		t.Fatalf("legacy attrs: %+v", m.Entrypoints)
	}
	if m.Kind != KindGame {
		t.Fatalf("kind = %q", m.Kind)
	}
}
