package loader

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestDisplay(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plugin id="classic-range" label="Classic Range" version="1.0.0" mode="per-range" kind="display">
  <description>Standard target view</description>
  <entrypoints view="view.js"/>
  <assetsDir>assets</assetsDir>
</plugin>`)
	m, err := ParseManifestXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "classic-range" || m.Kind != KindDisplay || m.Mode != ModePerRange {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if m.Entrypoints.View != "view.js" || m.HasLogic() {
		t.Fatalf("entrypoints: %+v", m.Entrypoints)
	}
}

func TestParseManifestGame(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plugin id="demo-game" label="Demo" version="1.0.0" mode="per-range" kind="game">
  <entrypoints logic="logic.wasm" view="view.js" theme="theme.css"/>
  <config><frames>10</frames></config>
</plugin>`)
	m, err := ParseManifestXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != KindGame || m.Entrypoints.Logic != "logic.wasm" || m.Entrypoints.Theme != "theme.css" {
		t.Fatalf("unexpected: %+v", m)
	}
	if m.Config.Defaults["frames"] != 10 {
		t.Fatalf("frames = %v", m.Config.Defaults["frames"])
	}
}

func TestInstallDisplayPluginZip(t *testing.T) {
	dir := t.TempDir()
	pluginsRoot := filepath.Join(dir, "plugins")
	_ = os.MkdirAll(pluginsRoot, 0755)

	src := filepath.Join("..", "..", "plugins", "classic-range")
	if _, err := os.Stat(src); err != nil {
		t.Skip("classic-range plugin not present")
	}
	zipPath := filepath.Join(dir, "classic-range.zip")
	if err := packDirZip(src, zipPath); err != nil {
		t.Fatal(err)
	}

	m := NewManager(pluginsRoot)
	info, err := m.Install(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "classic-range" || info.Kind != KindDisplay {
		t.Fatalf("unexpected install result: %+v", info)
	}
	ap, err := m.Get("classic-range")
	if err != nil {
		t.Fatal(err)
	}
	if ap.Logic == nil {
		t.Fatal("display plugin should have no WASM logic")
	}
	if _, ok := ap.Logic.(*DisplayLogic); !ok {
		t.Fatalf("expected DisplayLogic, got %T", ap.Logic)
	}
}

func TestReloadLoadsClassicRange(t *testing.T) {
	dir := t.TempDir()
	pluginsRoot := filepath.Join(dir, "plugins")
	src := filepath.Join("..", "..", "plugins", "classic-range")
	if _, err := os.Stat(src); err != nil {
		t.Skip("classic-range not present")
	}
	if err := copyDir(src, filepath.Join(pluginsRoot, "classic-range")); err != nil {
		t.Fatal(err)
	}

	m := NewManager(pluginsRoot)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	active, err := m.ListLoaded()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Manifest.ID != "classic-range" {
		t.Fatalf("expected classic-range, got %+v", active)
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

func packDirZip(src, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := w.Create(rel + "/")
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	})
}
