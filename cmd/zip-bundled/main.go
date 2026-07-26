package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	root := "plugins"
	outDir := filepath.Join("dist", "plugins")
	_ = os.MkdirAll(outDir, 0755)

	for _, id := range pluginIDs(root) {
		src := filepath.Join(root, id)
		manifest, err := os.ReadFile(filepath.Join(src, "manifest.xml"))
		if err != nil {
			panic(err)
		}
		version := readVersion(manifest)
		out := filepath.Join(outDir, id+"-"+version+".srplugin.zip")
		if err := zipDir(src, out); err != nil {
			panic(err)
		}
	}
}

func pluginIDs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		panic(err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "manifest.xml")); err != nil {
			continue
		}
		ids = append(ids, e.Name())
	}
	sort.Strings(ids)
	return ids
}

func readVersion(data []byte) string {
	// minimal parse: version="1.0.0" in manifest root
	s := string(data)
	const key = `version="`
	i := stringsIndex(s, key)
	if i < 0 {
		return "1.0.0"
	}
	s = s[i+len(key):]
	j := stringsIndex(s, `"`)
	if j < 0 {
		return "1.0.0"
	}
	return s[:j]
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func zipDir(src, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "config.xml" {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(rel), "logic/") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}
