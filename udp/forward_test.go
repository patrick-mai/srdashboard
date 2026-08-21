package udp

import (
	"net"
	"testing"
)

func TestParseForwardAddr(t *testing.T) {
	t.Parallel()
	if addr, err := ParseForwardAddr(""); err != nil || addr != nil {
		t.Fatalf("empty: addr=%v err=%v", addr, err)
	}
	addr, err := ParseForwardAddr("30170")
	if err != nil {
		t.Fatal(err)
	}
	if !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || addr.Port != 30170 {
		t.Fatalf("bare port: %v", addr)
	}
	addr, err = ParseForwardAddr("192.168.10.5:30169")
	if err != nil {
		t.Fatal(err)
	}
	if !addr.IP.Equal(net.IPv4(192, 168, 10, 5)) || addr.Port != 30169 {
		t.Fatalf("host:port: %v", addr)
	}
	if _, err := ParseForwardAddr("not-a-port"); err == nil {
		t.Fatal("expected error for garbage")
	}
	if _, err := ParseForwardAddr("0.0.0.0:30170"); err == nil {
		t.Fatal("expected error for unspecified host")
	}
	if _, err := ParseForwardAddr("127.0.0.1:0"); err == nil {
		t.Fatal("expected error for port 0")
	}
}

func TestTargetsOwnListener(t *testing.T) {
	t.Parallel()
	loop, err := ParseForwardAddr("127.0.0.1:30169")
	if err != nil {
		t.Fatal(err)
	}
	if !TargetsOwnListener(loop, 30169) {
		t.Fatal("loopback same port is this listener")
	}
	if TargetsOwnListener(loop, 30170) {
		t.Fatal("loopback other port is a second local dashboard")
	}
	other, err := ParseForwardAddr("8.8.8.8:30169")
	if err != nil {
		t.Fatal(err)
	}
	if TargetsOwnListener(other, 30169) {
		t.Fatal("remote host same port is a network hop, not a self-loop")
	}
	if TargetsOwnListener(nil, 30169) {
		t.Fatal("nil forward is off")
	}
}
