package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: zip-plugin <plugin-dir> [out.srplugin.zip]\n\n")
		fmt.Fprintf(os.Stderr, "Package one extra plugin. Shipped plugins go out as folders under dist/<platform>/plugins/, not as zips.\n")
		os.Exit(2)
	}
	src := os.Args[1]
	manifestPath := filepath.Join(src, "manifest.xml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zip-plugin: %s: %v\n", manifestPath, err)
		os.Exit(1)
	}
	id := filepath.Base(filepath.Clean(src))
	version := readAttr(manifest, "version")
	if version == "" {
		version = "1.0.0"
	}
	out := id + "-" + version + ".srplugin.zip"
	if len(os.Args) == 3 {
		out = os.Args[2]
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil && filepath.Dir(out) != "." {
		fmt.Fprintf(os.Stderr, "zip-plugin: %v\n", err)
		os.Exit(1)
	}
	if err := zipDir(src, out); err != nil {
		fmt.Fprintf(os.Stderr, "zip-plugin: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

func readAttr(data []byte, key string) string {
	s := string(data)
	needle := key + `="`
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	s = s[i+len(needle):]
	j := strings.Index(s, `"`)
	if j < 0 {
		return ""
	}
	return s[:j]
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
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "logic/") {
			return nil
		}
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
