package udp

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
	"unicode"
)

const forwardSeenSlots = 128

// ParseForwardAddr parses udpForward. Empty is off (nil, nil).
// A bare port number means 127.0.0.1:port. Otherwise host:port
// (IPv6 as [addr]:port). The destination IP must not be unspecified.
func ParseForwardAddr(s string) (*net.UDPAddr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if isBarePort(s) {
		s = net.JoinHostPort("127.0.0.1", s)
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return nil, fmt.Errorf("udpForward must be host:port or a port number, got %q", s)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("udpForward port must be between 1 and 65535, got %q", portStr)
	}
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, portStr))
	if err != nil {
		return nil, fmt.Errorf("udpForward %s: %w", s, err)
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		return nil, fmt.Errorf("udpForward must name a concrete host, got %s", s)
	}
	return addr, nil
}

// TargetsOwnListener reports whether forwarding to fwd would send back into
// this process's listen port (loopback, unspecified, or a local interface).
func TargetsOwnListener(fwd *net.UDPAddr, listenPort int) bool {
	if fwd == nil || listenPort < 1 || fwd.Port != listenPort {
		return false
	}
	return isLocalIP(fwd.IP)
}

func isBarePort(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isLocalIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP == nil {
			continue
		}
		if ipnet.IP.Equal(ip) {
			return true
		}
	}
	return false
}

func payloadHash(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

func sameUDPAddr(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Port != b.Port {
		return false
	}
	if a.IP == nil || b.IP == nil {
		return a.IP == nil && b.IP == nil
	}
	return a.IP.Equal(b.IP)
}
