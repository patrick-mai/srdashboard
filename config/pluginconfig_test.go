package config

import "testing"

func TestParsePluginConfigXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<pluginConfig id="maedn-party">
  <shotsPerPlayer>40</shotsPerPlayer>
  <chainRadius>1500</chainRadius>
  <defaultDifficulty>normal</defaultDifficulty>
  <rangeDifficulties>
    <range num="1">easy</range>
    <range num="2">hard</range>
  </rangeDifficulties>
</pluginConfig>`)

	got, err := ParseConfigMapXML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["shotsPerPlayer"] != 40 {
		t.Fatalf("shotsPerPlayer = %v, want 40", got["shotsPerPlayer"])
	}
	if got["chainRadius"] != 1500 {
		t.Fatalf("chainRadius = %v, want 1500", got["chainRadius"])
	}
	rd, ok := got["rangeDifficulties"].(map[string]any)
	if !ok {
		t.Fatalf("rangeDifficulties type = %T", got["rangeDifficulties"])
	}
	if rd["1"] != "easy" || rd["2"] != "hard" {
		t.Fatalf("rangeDifficulties = %#v", rd)
	}
}

func TestParseRangeTargetsXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<pluginConfig id="classic-range">
  <rangeTargets>
    <range num="1">
      <targetProfile>air_pistol_10m</targetProfile>
    </range>
    <range num="2">
      <targetProfile>air_rifle_10m</targetProfile>
    </range>
  </rangeTargets>
</pluginConfig>`)

	got, err := ParseConfigMapXML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rt, ok := got["rangeTargets"].(map[string]any)
	if !ok {
		t.Fatalf("rangeTargets type = %T", got["rangeTargets"])
	}
	r1, ok := rt["1"].(map[string]any)
	if !ok || r1["targetProfile"] != "air_pistol_10m" {
		t.Fatalf("range 1 = %#v", rt["1"])
	}
}
