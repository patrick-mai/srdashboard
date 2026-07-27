package config

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseConfigMapXML parses a <pluginConfig> or <config> element into a map.
func ParseConfigMapXML(data []byte) (map[string]any, error) {
	for _, root := range []string{"pluginConfig", "config"} {
		m, found, err := parseConfigRootXML(data, root)
		if err != nil {
			return nil, err
		}
		if found {
			return m, nil
		}
	}
	return map[string]any{}, nil
}

// ParseConfigInnerXML parses config key/value elements from a manifest config body.
func ParseConfigInnerXML(inner string) (map[string]any, error) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return map[string]any{}, nil
	}
	return ParseConfigMapXML([]byte("<pluginConfig>" + inner + "</pluginConfig>"))
}

func parseConfigRootXML(data []byte, rootName string) (map[string]any, bool, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != rootName {
			continue
		}
		out := make(map[string]any)
		for {
			child, err := dec.Token()
			if err != nil {
				return nil, true, err
			}
			if end, ok := child.(xml.EndElement); ok && end.Name.Local == rootName {
				return out, true, nil
			}
			elem, ok := child.(xml.StartElement)
			if !ok {
				continue
			}
			val, err := readPluginConfigElement(dec, elem)
			if err != nil {
				return nil, true, fmt.Errorf("%s: %w", elem.Name.Local, err)
			}
			out[elem.Name.Local] = val
		}
	}
}

// LoadPluginConfig reads {configDir}/{pluginID}/config.xml.
// Missing files return an empty map (no overrides).
func LoadPluginConfig(configDir, pluginID string) (map[string]any, error) {
	path := filepath.Join(configDir, pluginID, "config.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	return ParseConfigMapXML(data)
}

type pluginConfigChild struct {
	start xml.StartElement
	value any
}

func readPluginConfigElement(dec *xml.Decoder, start xml.StartElement) (any, error) {
	var text strings.Builder
	var children []pluginConfigChild

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				text.WriteString(s)
			}
		case xml.StartElement:
			val, err := readPluginConfigElement(dec, t)
			if err != nil {
				return nil, err
			}
			children = append(children, pluginConfigChild{start: t, value: val})
		case xml.EndElement:
			if t.Name.Local != start.Name.Local {
				return nil, fmt.Errorf("unexpected end element %s", t.Name.Local)
			}
			return finalizePluginConfigElement(start, text.String(), children)
		}
	}
}

func finalizePluginConfigElement(start xml.StartElement, text string, children []pluginConfigChild) (any, error) {
	if len(children) == 0 {
		return coerceScalar(text), nil
	}
	if start.Name.Local == "rangeDifficulties" || start.Name.Local == "rangeHandicaps" || start.Name.Local == "rangeTargets" {
		out := make(map[string]any)
		for _, c := range children {
			if c.start.Name.Local != "range" {
				return nil, fmt.Errorf("expected <range>, got <%s>", c.start.Name.Local)
			}
			num := attrValue(c.start.Attr, "num")
			if num == "" {
				return nil, fmt.Errorf("<range> missing num attribute")
			}
			out[num] = c.value
		}
		return out, nil
	}
	out := make(map[string]any)
	for _, c := range children {
		out[c.start.Name.Local] = c.value
	}
	return out, nil
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func coerceScalar(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
