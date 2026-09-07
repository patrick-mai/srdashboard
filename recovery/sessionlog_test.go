package recovery

import (
	"os"
	"strings"
	"testing"
)

func TestSessionLogWrite(t *testing.T) {
	dir := t.TempDir()
	sl, err := OpenSessionLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer sl.Close()

	if _, err := sl.Write([]byte("2026/09/01 19:21:30 UDP: shot applied range=1 X=50 Y=-30 DecValue=9.8 at=-\n")); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(sl.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "UDP: shot applied range=1") {
		t.Fatalf("log content: %q", data)
	}
}

func TestParseLog_JSONAndConsole(t *testing.T) {
	text := `
ignored

{"MessageType":"Event","MessageVerb":"Other"}
{"MessageType":"Event","MessageVerb":"Shot","Ranges":1,"Objects":[{"X":10,"Y":-4,"Distance":10.7,"FullValue":10,"DecValue":10.9,"Range":1}]}

2026/09/01 19:21:30 UDP: shot applied range=2 X=70 Y=-144 DecValue=10.5 at=2026-09-01T19:21:30+02:00
`
	lines := ParseLog(text)
	if len(lines) != 2 {
		t.Fatalf("got %d packets, want 2", len(lines))
	}
}

func TestParseLog_BareShotObject(t *testing.T) {
	text := `{"X":10,"Y":-4,"Distance":10.7,"FullValue":10,"DecValue":10.9,"Range":3}`
	lines := ParseLog(text)
	if len(lines) != 1 {
		t.Fatalf("got %d packets, want 1", len(lines))
	}
	if !strings.Contains(string(lines[0]), `"MessageVerb":"Shot"`) {
		t.Fatalf("not wrapped: %s", lines[0])
	}
}

func TestParseLog_BOM(t *testing.T) {
	text := "\uFEFF" + `{"MessageType":"Event","MessageVerb":"Shot","Ranges":1,"Objects":[{"X":1,"Y":2,"Distance":2.2,"FullValue":10,"DecValue":10.9,"Range":1}]}`
	lines := ParseLog(text)
	if len(lines) != 1 {
		t.Fatalf("got %d packets, want 1", len(lines))
	}
}
