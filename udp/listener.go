package udp

import (
	"encoding/json"
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
	conn *net.UDPConn
	pipe Pipeline
	done chan struct{}
}

// NewListener creates a UDP listener on the given port
func NewListener(port int, st *state.LiveState) (*Listener, error) {
	addr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	return &Listener{
		conn: conn,
		pipe: Pipeline{State: st},
		done: make(chan struct{}),
	}, nil
}

// SetShotNotifier registers a callback invoked after each shot is applied.
func (l *Listener) SetShotNotifier(fn ShotNotifier) {
	l.pipe.OnShot = fn
}

// Start begins reading UDP packets
func (l *Listener) Start() {
	go l.readLoop()
	log.Printf("UDP listener started on port %d", l.conn.LocalAddr().(*net.UDPAddr).Port)
}

// Stop closes the listener
func (l *Listener) Stop() {
	close(l.done)
	_ = l.conn.Close()
}

func (l *Listener) readLoop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-l.done:
			return
		default:
			n, _, err := l.conn.ReadFromUDP(buf)
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				log.Printf("UDP read error: %v", err)
				continue
			}
			l.handlePacket(buf[:n])
		}
	}
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
