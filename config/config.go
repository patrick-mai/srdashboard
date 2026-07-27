package config

import (
	"encoding/xml"
	"os"
	"strconv"
)

// Config holds application configuration
type Config struct {
	XMLName       xml.Name `xml:"config"`
	UDPPort       int      `xml:"udpPort"`
	ODBCName      string   `xml:"odbcName"`
	Ranges        int      `xml:"ranges"`
	LayoutColumns int    `xml:"layoutColumns"` // number of panels per row (e.g. 4 → 4 in first row, 2 in second for 6 ranges)
	Footer        Footer `xml:"footer"`
	Plugins       Plugins  `xml:"plugins"`
	Display       Display  `xml:"display"`
}

// Plugins holds the plugin directory, site-active plugin, and optional version pins.
type Plugins struct {
	Dir    string      `xml:"dir,attr"`    // plugins/{id}/ — manifest, assets, config.xml
	Active string      `xml:"active,attr"` // always-on active plugin id (default classic-range)
	Plugin []PluginRef `xml:"plugin"`
}

// PluginRef pins an installed plugin version from the global config.
type PluginRef struct {
	ID      string `xml:"id,attr" json:"id"`
	Version string `xml:"version,attr" json:"version"`
}

// Display holds UI defaults and control token for master displays.
type Display struct {
	DefaultMode      string  `xml:"defaultMode" json:"defaultMode"`
	ControlToken     string  `xml:"controlToken" json:"controlToken"`
	ShotStrokeWidth  float64 `xml:"shotStrokeWidth" json:"shotStrokeWidth"` // pellet outline width in mm (SVG user units)
}

// Footer holds visibility toggles for footer elements
type Footer struct {
	CurrentShotValue  bool `xml:"currentShotValue" json:"currentShotValue"`
	Teiler            bool `xml:"teiler" json:"teiler"`
	ShotNumber        bool `xml:"shotNumber" json:"shotNumber"`
	OverallSumInt     bool `xml:"overallSumInt" json:"overallSumInt"`
	OverallSumDecimal bool `xml:"overallSumDecimal" json:"overallSumDecimal"`
	PredictionInt     bool `xml:"predictionInt" json:"predictionInt"`
	PredictionDecimal bool `xml:"predictionDecimal" json:"predictionDecimal"`
	SeriesSumsInt     bool `xml:"seriesSumsInt" json:"seriesSumsInt"`
	SeriesSumsDecimal bool `xml:"seriesSumsDecimal" json:"seriesSumsDecimal"`
	Last10Int         bool `xml:"last10Int" json:"last10Int"`
	Last10Decimal     bool `xml:"last10Decimal" json:"last10Decimal"`
}

// Load reads config from an XML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.UDPPort == 0 {
		cfg.UDPPort = 30169
	}
	if cfg.Ranges == 0 {
		cfg.Ranges = 6
	}
	if cfg.LayoutColumns <= 0 {
		cfg.LayoutColumns = 4
	}
	if cfg.Plugins.Dir == "" {
		cfg.Plugins.Dir = "plugins"
	}
	if cfg.Plugins.Active == "" {
		cfg.Plugins.Active = "classic-range"
	}
	if cfg.Display.DefaultMode == "" {
		cfg.Display.DefaultMode = "master"
	}
	if cfg.Display.ShotStrokeWidth <= 0 {
		cfg.Display.ShotStrokeWidth = 0.1
	}
}

// ParseBool parses XML boolean (true/false strings)
func ParseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
