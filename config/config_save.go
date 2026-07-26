package config

import (
	"fmt"
	"path/filepath"

	"github.com/beevik/etree"
)

// Save writes global config to path, preserving existing XML comments where possible.
func Save(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	applyDefaults(cfg)

	doc, existed, err := loadXMLDoc(path)
	if err != nil {
		return err
	}
	root := ensureRoot(doc, "config")
	if !existed {
		root.Tag = "config"
	}

	setChildScalar(root, "udpPort", cfg.UDPPort)
	setChildScalar(root, "odbcName", cfg.ODBCName)
	setChildScalar(root, "ranges", cfg.Ranges)
	setChildScalar(root, "layoutColumns", cfg.LayoutColumns)
	setChildScalar(root, "defaultTarget", cfg.DefaultTarget)

	if oldSkin := root.SelectElement("skin"); oldSkin != nil {
		root.RemoveChild(oldSkin)
	}

	footer := selectOrCreate(root, "footer")
	setChildBool(footer, "currentShotValue", cfg.Footer.CurrentShotValue)
	setChildBool(footer, "teiler", cfg.Footer.Teiler)
	setChildBool(footer, "shotNumber", cfg.Footer.ShotNumber)
	setChildBool(footer, "overallSumInt", cfg.Footer.OverallSumInt)
	setChildBool(footer, "overallSumDecimal", cfg.Footer.OverallSumDecimal)
	setChildBool(footer, "predictionInt", cfg.Footer.PredictionInt)
	setChildBool(footer, "predictionDecimal", cfg.Footer.PredictionDecimal)
	setChildBool(footer, "seriesSumsInt", cfg.Footer.SeriesSumsInt)
	setChildBool(footer, "seriesSumsDecimal", cfg.Footer.SeriesSumsDecimal)
	setChildBool(footer, "last10Int", cfg.Footer.Last10Int)
	setChildBool(footer, "last10Decimal", cfg.Footer.Last10Decimal)

	plugins := selectOrCreate(root, "plugins")
	if cfg.Plugins.Dir != "" {
		if attr := plugins.SelectAttr("dir"); attr != nil {
			attr.Value = cfg.Plugins.Dir
		} else {
			plugins.CreateAttr("dir", cfg.Plugins.Dir)
		}
	}
	active := cfg.Plugins.Active
	if active == "" {
		active = "classic-range"
	}
	if attr := plugins.SelectAttr("active"); attr != nil {
		attr.Value = active
	} else {
		plugins.CreateAttr("active", active)
	}
	syncPluginRefs(plugins, cfg.Plugins.Plugin)

	display := selectOrCreate(root, "display")
	setChildScalar(display, "defaultMode", cfg.Display.DefaultMode)
	setChildScalar(display, "controlToken", cfg.Display.ControlToken)
	setChildScalar(display, "shotStrokeWidth", cfg.Display.ShotStrokeWidth)

	return writeXMLDocAtomic(path, doc)
}

func setChildScalar(parent *etree.Element, tag string, v any) {
	el := selectOrCreate(parent, tag)
	setScalarText(el, v)
}

func setChildBool(parent *etree.Element, tag string, v bool) {
	setChildScalar(parent, tag, v)
}

func syncPluginRefs(plugins *etree.Element, refs []PluginRef) {
	existing := plugins.SelectElements("plugin")
	keep := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		keep[ref.ID] = true
		var el *etree.Element
		for _, e := range existing {
			if e.SelectAttrValue("id", "") == ref.ID {
				el = e
				break
			}
		}
		if el == nil {
			el = plugins.CreateElement("plugin")
			el.CreateAttr("id", ref.ID)
		}
		if ref.Version != "" {
			if attr := el.SelectAttr("version"); attr != nil {
				attr.Value = ref.Version
			} else {
				el.CreateAttr("version", ref.Version)
			}
		}
	}
	for _, e := range existing {
		id := e.SelectAttrValue("id", "")
		if id != "" && !keep[id] {
			plugins.RemoveChild(e)
		}
	}
}

// PluginConfigPath returns {configDir}/{pluginID}/config.xml.
func PluginConfigPath(configDir, pluginID string) string {
	return filepath.Join(configDir, pluginID, "config.xml")
}
