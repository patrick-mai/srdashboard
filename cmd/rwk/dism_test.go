package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseDismState(t *testing.T) {
	out := `
Capability Identity : Language.OCR~~~de-DE~0.0.1.0
Name : Language.OCR~~~de-DE~0.0.1.0
State : Installed
Display Name : OCR
`
	if got := parseDismState(out); got != "Installed" {
		t.Fatalf("got %q", got)
	}
	if got := parseDismState("State : Not Present\n"); got != "Not Present" {
		t.Fatalf("got %q", got)
	}
	if got := parseDismState("no state here"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestDismFailure(t *testing.T) {
	if got := dismFailure(fmt.Errorf("exit status 740")); !strings.Contains(got, "erhöhten Rechten") {
		t.Fatalf("got %q", got)
	}
	if got := dismFailure(fmt.Errorf("exit status 1")); got != "nicht abgefragt" {
		t.Fatalf("got %q", got)
	}
}
