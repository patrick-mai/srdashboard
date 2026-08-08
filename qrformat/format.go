package qrformat

import (
	"fmt"
	"sort"
)

// Encoder turns a range result into a scannable QR URL for one consumer format.
type Encoder interface {
	ID() string
	Label() string
	EncodeURL(snap ResultInput) (string, error)
}

var registry = map[string]Encoder{}

// Register adds an encoder (called from init in format packages).
func Register(e Encoder) {
	if e == nil || e.ID() == "" {
		return
	}
	registry[e.ID()] = e
}

// Get returns an encoder by id.
func Get(id string) (Encoder, bool) {
	e, ok := registry[id]
	return e, ok
}

// List returns registered encoders sorted by id.
func List() []Encoder {
	out := make([]Encoder, 0, len(registry))
	for _, e := range registry {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// MustGet is Get that panics — for tests.
func MustGet(id string) Encoder {
	e, ok := Get(id)
	if !ok {
		panic(fmt.Sprintf("qrformat: unknown format %q", id))
	}
	return e
}
