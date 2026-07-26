package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePluginConfigPreservesComments(t *testing.T) {
	dir := t.TempDir()
	pluginID := "maedn-party"
	pluginDir := filepath.Join(dir, pluginID)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, "config.xml")
	original := `<?xml version="1.0" encoding="UTF-8"?>
<pluginConfig id="maedn-party">
  <shotsPerPlayer>40</shotsPerPlayer>
  <!-- Per-range difficulty (range num = DISAG range number): easy, normal, or hard -->
  <!--
  <rangeDifficulties>
    <range num="1">easy</range>
    <range num="2">normal</range>
  </rangeDifficulties>
  -->
</pluginConfig>`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SavePluginConfig(dir, pluginID, map[string]any{
		"shotsPerPlayer": 50,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "Per-range difficulty") {
		t.Fatalf("comment lost:\n%s", text)
	}
	if !strings.Contains(text, "<shotsPerPlayer>50</shotsPerPlayer>") {
		t.Fatalf("value not updated:\n%s", text)
	}
}

func TestSaveGlobalConfigPreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.xml")
	original := `<?xml version="1.0" encoding="UTF-8"?>
<config>
  <!-- UDP port for OpticScore -->
  <udpPort>30169</udpPort>
  <ranges>6</ranges>
</config>`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Plugins.Active != "classic-range" {
		t.Fatalf("default active = %q", cfg.Plugins.Active)
	}
	cfg.LayoutColumns = 4
	cfg.Plugins.Active = "classic-range"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "UDP port for OpticScore") {
		t.Fatalf("comment lost:\n%s", text)
	}
	if !strings.Contains(text, "<layoutColumns>4</layoutColumns>") {
		t.Fatalf("layoutColumns not written:\n%s", text)
	}
	if !strings.Contains(text, `active="classic-range"`) {
		t.Fatalf("active attr missing:\n%s", text)
	}
}

func TestDiffOverrides(t *testing.T) {
	defaults := map[string]any{"shotsPerPlayer": 40, "chainRadius": 1500}
	merged := map[string]any{"shotsPerPlayer": 50, "chainRadius": 1500}
	got := DiffOverrides(defaults, merged)
	if len(got) != 1 || got["shotsPerPlayer"] != 50 {
		t.Fatalf("diff = %#v", got)
	}
}
