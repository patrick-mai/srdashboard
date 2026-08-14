package gameutil

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	PhaseWarmup   = "warmup"
	PhaseArming   = "arming"
	PhasePlaying  = "playing"
	PhaseFinished = "finished"
)

var LaneColors = []string{
	"#2f6b3a", "#c45c26", "#1f4e79", "#8b4513",
	"#4a7c59", "#b8860b", "#5c4033", "#2e5a4c",
}

func LaneColor(rangeNum int) string {
	if rangeNum < 1 {
		return LaneColors[0]
	}
	return LaneColors[(rangeNum-1)%len(LaneColors)]
}

func RawShotValue(dec float64, full int) float64 {
	if dec > 0 {
		return dec
	}
	if full > 0 {
		return float64(full)
	}
	return 0
}

func Effective(raw, handicap float64) float64 {
	return raw + handicap
}

func Itoa(i int) string { return strconv.Itoa(i) }

func MergeConfig(defaults, cfg map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

func CfgInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := strconv.Atoi(t)
		if err == nil {
			return i
		}
	}
	return def
}

func CfgFloat(m map[string]any, key string, def float64) float64 {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	return ToFloat(v, def)
}

func CfgString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func CfgBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	}
	return def
}

func ToFloat(v any, def float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err == nil {
			return f
		}
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func CfgFloatMap(m map[string]any, key string) map[string]float64 {
	out := map[string]float64{}
	if m == nil {
		return out
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return out
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for k, v := range obj {
		out[k] = ToFloat(v, 0)
	}
	return out
}

func HandicapFor(cfg map[string]any, rangeNum int) float64 {
	m := CfgFloatMap(cfg, "handicaps")
	if v, ok := m[Itoa(rangeNum)]; ok {
		return v
	}
	return 0
}

func ParseIntList(v any) []int {
	if v == nil {
		return nil
	}
	var out []int
	switch x := v.(type) {
	case string:
		for _, part := range strings.Split(x, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			n, err := strconv.Atoi(part)
			if err == nil {
				out = append(out, n)
			}
		}
	case []any:
		for _, item := range x {
			n := int(ToFloat(item, 0))
			if n > 0 {
				out = append(out, n)
			}
		}
	case []int:
		out = append(out, x...)
	}
	return out
}

func CfgIntList(m map[string]any, key string, def []int) []int {
	if m == nil {
		return append([]int(nil), def...)
	}
	list := ParseIntList(m[key])
	if len(list) == 0 {
		return append([]int(nil), def...)
	}
	return list
}

func DefaultDisciplineTargets() map[string]any {
	return map[string]any{
		"Luftgewehr":   "air_rifle_10m",
		"LG":           "air_rifle_10m",
		"Luftpistole":  "air_pistol_10m",
		"LP":           "air_pistol_10m",
		"Kleinkaliber": "smallbore_50m_prone",
		"KK-Gewehr":    "smallbore_50m_prone",
		"KK":           "smallbore_50m_prone",
	}
}
