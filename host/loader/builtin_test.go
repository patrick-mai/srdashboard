package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinManifestValidate(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "view.js"), []byte("//"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte(`<?xml version="1.0"?>
<plugin id="demo-builtin" label="Demo" version="1.0.0" mode="shared" kind="game">
  <entrypoints view="view.js" logic="builtin"/>
</plugin>`), 0644)
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsBuiltin() {
		t.Fatal("expected builtin")
	}
	if err := m.ValidateFiles(dir); err != nil {
		t.Fatal(err)
	}
}
