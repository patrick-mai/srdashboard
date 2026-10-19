package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const ocrCapability = "Language.OCR~~~de-DE~0.0.1.0"

var dismStateRE = regexp.MustCompile(`(?im)^State\s*:\s*(.+?)\s*$`)

func parseDismState(out string) string {
	m := dismStateRE.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func dismCapabilityState() (string, error) {
	exe := "dism.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		exe = filepath.Join(root, "System32", "dism.exe")
	}
	cmd := exec.Command(exe, "/Online", "/Get-CapabilityInfo", "/CapabilityName:"+ocrCapability)
	out, err := cmd.CombinedOutput()
	state := parseDismState(string(out))
	if state == "" && err != nil {
		return "", err
	}
	return state, nil
}

func dismFailure(err error) string {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if strings.Contains(msg, "740") {
		return "DISM braucht eine Eingabeaufforderung mit erhöhten Rechten"
	}
	return "nicht abgefragt"
}

func runCheck() int {
	state, err := dismCapabilityState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", ocrCapability, dismFailure(err))
		return 1
	}
	fmt.Printf("%s: %s\n", ocrCapability, state)
	if !strings.EqualFold(state, "Installed") {
		fmt.Fprintf(os.Stderr, "Capability %s ist nicht installiert\n", ocrCapability)
		return 1
	}
	return 0
}
