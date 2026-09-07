package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"srdashboard/state"
)

// Message is the DISAG OpticScore JSON message envelope
type Message struct {
	MessageType string            `json:"MessageType"`
	MessageVerb string            `json:"MessageVerb"`
	Ranges      int               `json:"Ranges"`
	Objects     []json.RawMessage `json:"Objects"`
	// Envelope-level timestamps (some OpticScore builds).
	Timestamp string `json:"Timestamp"`
	DateTime  string `json:"DateTime"`
	Time      string `json:"Time"`
	DATETIME  string `json:"DATETIME"`
}

// EventTime returns a timestamp from the message envelope, if present.
func (m Message) EventTime() (time.Time, bool) {
	return state.EventTimeFromFields(m.Timestamp, m.DateTime, m.Time, m.DATETIME)
}

// ShotNotifier is called after a shot is applied to live state.
type ShotNotifier func(rng int, shot state.Shot, shotIndex int)

// Listener reads DISAG OpticScore UDP datagrams and runs them through Pipeline.
type Listener struct {
	conn     *net.UDPConn
	send     *net.UDPConn // nil unless forwarding
	fwd      *net.UDPAddr
	pipe     Pipeline
	done     chan struct{}
	fwdLogAt time.Time
	seen     [forwardSeenSlots]uint64
	seenI    int
}

// NewListener creates a UDP listener on the given port.
// forward is udpForward (host:port or bare port); empty disables resend.
func NewListener(port int, st *state.LiveState, forward string) (*Listener, error) {
	addr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	listenPort := conn.LocalAddr().(*net.UDPAddr).Port
	fwd, err := ParseForwardAddr(forward)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if TargetsOwnListener(fwd, listenPort) {
		_ = conn.Close()
		return nil, fmt.Errorf("udpForward %s targets this listener's port %d", forward, listenPort)
	}
	var send *net.UDPConn
	if fwd != nil {
		send, err = net.DialUDP("udp", nil, fwd)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("udpForward dial %s: %w", fwd, err)
		}
	}
	return &Listener{
		conn: conn,
		send: send,
		fwd:  fwd,
		pipe: Pipeline{State: st},
		done: make(chan struct{}),
	}, nil
}

// LocalAddr is the bound UDP address (tests and diagnostics).
func (l *Listener) LocalAddr() net.Addr {
	if l == nil || l.conn == nil {
		return nil
	}
	return l.conn.LocalAddr()
}

// SetShotNotifier registers a callback invoked after each shot is applied.
func (l *Listener) SetShotNotifier(fn ShotNotifier) {
	l.pipe.OnShot = fn
}

// SetShotFilter registers the membership gate (configured 1..N and not inactive).
func (l *Listener) SetShotFilter(f ShotFilter) {
	l.pipe.Filter = f
}

// Ingest applies one OpticScore datagram without UDP (replay / recovery).
func (l *Listener) Ingest(data []byte) int {
	return l.pipe.Ingest(data)
}

// ReplayLog bulk-applies recovered packets through the validated ingest path,
// including OnShot so game plugins reconstruct from the same shots.
func (l *Listener) ReplayLog(packets [][]byte) int {
	applied := 0
	for _, pkt := range packets {
		applied += l.pipe.IngestReplay(pkt)
	}
	return applied
}

// Start begins reading UDP packets
func (l *Listener) Start() {
	go l.readLoop()
	if l.fwd != nil {
		log.Printf("UDP listener started on port %d, forwarding to %s", l.conn.LocalAddr().(*net.UDPAddr).Port, l.fwd)
		return
	}
	log.Printf("UDP listener started on port %d", l.conn.LocalAddr().(*net.UDPAddr).Port)
}

// Stop closes the listener
func (l *Listener) Stop() {
	close(l.done)
	if l.send != nil {
		_ = l.send.Close()
	}
	_ = l.conn.Close()
}

func (l *Listener) readLoop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-l.done:
			return
		default:
			n, src, err := l.conn.ReadFromUDP(buf)
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				log.Printf("UDP read error: %v", err)
				continue
			}
			l.dispatch(buf[:n], src)
		}
	}
}

// dispatch copies a datagram to udpForward (if set) then ingests it.
// Inactive-lane filtering happens in Ingest only — the next dashboard still
// needs every raw packet. Exact-payload echoes (a forward loop) are dropped.
func (l *Listener) dispatch(data []byte, src *net.UDPAddr) {
	if l.fwd != nil {
		if sameUDPAddr(src, l.fwd) || l.alreadySeen(data) {
			return
		}
		if _, err := l.send.Write(data); err != nil {
			l.logForwardErr(err)
		}
	}
	l.handlePacket(data)
}

func (l *Listener) alreadySeen(data []byte) bool {
	h := payloadHash(data)
	if h == 0 {
		h = 1
	}
	for _, x := range l.seen {
		if x == h {
			return true
		}
	}
	l.seen[l.seenI] = h
	l.seenI = (l.seenI + 1) % forwardSeenSlots
	return false
}

func (l *Listener) logForwardErr(err error) {
	now := time.Now()
	if !l.fwdLogAt.IsZero() && now.Sub(l.fwdLogAt) < 5*time.Second {
		return
	}
	l.fwdLogAt = now
	log.Printf("UDP forward to %s: %v", l.fwd, err)
}

func (l *Listener) handlePacket(data []byte) {
	l.pipe.Ingest(data)
}

func warnIfReplacementInName(first, last string) {
	for _, s := range []string{first, last} {
		for _, r := range s {
			if r == '\uFFFD' {
				log.Printf("UDP: shooter name contains U+FFFD — upstream likely JSON-decoded CP1252 as UTF-8; send raw OpticScore bytes")
				return
			}
		}
	}
}

func shotAtOrDash(t time.Time, ok bool) string {
	if !ok || t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339Nano)
}
