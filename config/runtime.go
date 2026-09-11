package config

import (
	"encoding/xml"
	"os"
	"path/filepath"
)

// Runtime is per-process membership: which hall lanes this instance does not serve.
// Hall config.xml still holds the maximum N; missing file means all 1..N are active.
type Runtime struct {
	XMLName          xml.Name `xml:"runtime"`
	InactiveRanges   string   `xml:"inactiveRanges"`
	WettkampfVisible *bool    `xml:"wettkampfVisible"`
}

// RuntimePath is runtime.xml beside the hall config file.
func RuntimePath(configPath string) string {
	if configPath == "" {
		return "runtime.xml"
	}
	return filepath.Join(filepath.Dir(configPath), "runtime.xml")
}

// WettkampfPath is wettkampf.xml beside the hall config file.
func WettkampfPath(configPath string) string {
	if configPath == "" {
		return "wettkampf.xml"
	}
	return filepath.Join(filepath.Dir(configPath), "wettkampf.xml")
}

// WettkampfIsVisible reports whether the CRC Wettkampf tile should show.
// Missing file / omitted field defaults to visible.
func (rt *Runtime) WettkampfIsVisible() bool {
	if rt == nil || rt.WettkampfVisible == nil {
		return true
	}
	return *rt.WettkampfVisible
}

// LoadRuntime reads path. A missing file is an empty runtime (all lanes active).
func LoadRuntime(path string) (*Runtime, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Runtime{}, nil
		}
		return nil, err
	}
	var rt Runtime
	if err := xml.Unmarshal(data, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}

// SaveRuntime writes path atomically.
func SaveRuntime(path string, rt *Runtime) error {
	if rt == nil {
		rt = &Runtime{}
	}
	data, err := xml.MarshalIndent(rt, "", "  ")
	if err != nil {
		return err
	}
	doc := append([]byte(xml.Header), data...)
	doc = append(doc, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".runtime-*.xml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(doc); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// InactiveList returns inactive lane numbers, optionally clipped to 1..max.
func (rt *Runtime) InactiveList(max int) []int {
	if rt == nil {
		return nil
	}
	out := make([]int, 0)
	for _, n := range ParseRangeList(rt.InactiveRanges) {
		if max > 0 && n > max {
			continue
		}
		out = append(out, n)
	}
	return out
}

// SetInactiveList stores the inactive set, clipped to 1..max when max > 0.
func (rt *Runtime) SetInactiveList(nums []int, max int) {
	if rt == nil {
		return
	}
	keep := make([]int, 0, len(nums))
	for _, n := range nums {
		if n < 1 {
			continue
		}
		if max > 0 && n > max {
			continue
		}
		keep = append(keep, n)
	}
	rt.InactiveRanges = FormatRangeList(keep)
}

// IsInactive reports whether this instance drops shots for rng.
func (rt *Runtime) IsInactive(rng int) bool {
	if rt == nil {
		return false
	}
	for _, n := range ParseRangeList(rt.InactiveRanges) {
		if n == rng {
			return true
		}
	}
	return false
}

// Prune drops inactive entries outside 1..max.
func (rt *Runtime) Prune(max int) {
	if rt == nil {
		return
	}
	rt.SetInactiveList(ParseRangeList(rt.InactiveRanges), max)
}
