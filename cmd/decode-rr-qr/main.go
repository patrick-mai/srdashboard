// decode-rr-qr prints the JSON inside a Ring Reader import QR URL.
//
//	go run ./cmd/decode-rr-qr "https://ringreader.app/import/qr#…"
//	go run ./cmd/decode-rr-qr -file url.txt
//
// Payload = base64url (no padding) of raw DEFLATE (no zlib/gzip wrapper).
package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	file := flag.String("file", "", "Read URL or fragment from a text file")
	flag.Parse()

	raw := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read file: %v\n", err)
			os.Exit(1)
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/decode-rr-qr <url-or-fragment>")
		os.Exit(2)
	}

	frag := raw
	if i := strings.Index(raw, "#"); i >= 0 {
		frag = raw[i+1:]
	}
	frag = strings.TrimSpace(frag)

	compressed, err := base64.RawURLEncoding.DecodeString(frag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "base64url: %v\n", err)
		os.Exit(1)
	}
	r := flate.NewReader(bytes.NewReader(compressed))
	plain, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deflate: %v (need raw DEFLATE, not gzip/zlib)\n", err)
		os.Exit(1)
	}

	var v any
	if err := json.Unmarshal(plain, &v); err != nil {
		fmt.Fprintf(os.Stderr, "json: %v\n%s\n", err, plain)
		os.Exit(1)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "indent: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
