package loader

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds an archive whose entry names are written verbatim, so tests
// can construct the malformed names a hostile package would use.
func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pkg.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnzipRejectsEscapingEntries(t *testing.T) {
	cases := map[string]string{
		"parent traversal":   "../escaped.txt",
		"deep traversal":     "a/../../escaped.txt",
		"absolute unix path": "/tmp/escaped.txt",
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			src := writeZip(t, map[string]string{
				"manifest.xml": "<plugin/>",
				entry:          "pwned",
			})
			dest := t.TempDir()
			if err := unzip(src, dest); err == nil {
				t.Fatal("unzip accepted an escaping entry")
			}
			// Nothing may have landed next to the destination directory.
			outside := filepath.Join(filepath.Dir(dest), "escaped.txt")
			if _, err := os.Stat(outside); err == nil {
				t.Fatalf("wrote outside the destination: %s", outside)
			}
		})
	}
}

func TestUnzipRejectsTooManyEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i <= MaxPluginZipEntries; i++ {
		w, err := zw.Create(filepath.ToSlash(filepath.Join("many", "f"+strings.Repeat("0", 3)+string(rune('a'+i%26))+itoa(i))))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "many.zip")
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := unzip(src, t.TempDir()); err == nil {
		t.Fatal("unzip accepted an archive over the entry limit")
	}
}

func TestUnzipExtractsNormalPackage(t *testing.T) {
	src := writeZip(t, map[string]string{
		"manifest.xml":     "<plugin/>",
		"view.js":          "// view",
		"assets/logo.svg":  "<svg/>",
		"nested/dir/a.txt": "a",
	})
	dest := t.TempDir()
	if err := unzip(src, dest); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"manifest.xml", "view.js", filepath.Join("assets", "logo.svg"), filepath.Join("nested", "dir", "a.txt")} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestSanitizeUploadName(t *testing.T) {
	cases := map[string]string{
		"demo.srplugin.zip":            "demo.srplugin.zip",
		"../../evil.zip":               "evil.zip",
		`..\..\evil.zip`:               "evil.zip",
		"/etc/passwd":                  "passwd",
		`C:\Windows\System32\evil.zip`: "evil.zip",
		"":                             "upload.srplugin.zip",
		"..":                           "upload.srplugin.zip",
	}
	for in, want := range cases {
		if got := sanitizeUploadName(in); got != want {
			t.Errorf("sanitizeUploadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckZipEntryName(t *testing.T) {
	valid := []string{"manifest.xml", "assets/logo.svg", "a/b/c.txt", "."}
	for _, name := range valid {
		if err := checkZipEntryName(name); err != nil {
			t.Errorf("checkZipEntryName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"..", "../x", "/abs/path"}
	for _, name := range invalid {
		if err := checkZipEntryName(name); err == nil {
			t.Errorf("checkZipEntryName(%q) = nil, want error", name)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
