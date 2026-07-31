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

// Listener reads DISAG OpticScore UDP messages and forwards shots to the state
type Listener struct {
	conn     *net.UDPConn
	state    *state.LiveState
	onShot   ShotNotifier
	done     chan struct{}
}

// NewListener creates a UDP listener on the given port
func NewListener(port int, st *state.LiveState) (*Listener, error) {
	addr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	return &Listener{
		conn:  conn,
		state: st,
		done:  make(chan struct{}),
	}, nil
}

// SetShotNotifier registers a callback invoked after each shot is applied.
func (l *Listener) SetShotNotifier(fn ShotNotifier) {
	l.onShot = fn
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
	data = normalizeOpticScoreJSON(data)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("UDP: invalid JSON (len=%d): %v", len(data), err)
		return
	}
	if msg.MessageType != "Event" || msg.MessageVerb != "Shot" {
		log.Printf("UDP: ignored message MessageType=%q MessageVerb=%q (expected Event/Shot)", msg.MessageType, msg.MessageVerb)
		return
	}
	if len(msg.Objects) == 0 {
		log.Printf("UDP: Shot message has no Objects")
		return
	}
	var shot state.ShotPayload
	if err := json.Unmarshal(msg.Objects[0], &shot); err != nil {
		log.Printf("UDP: failed to parse shot object: %v", err)
		return
	}
	rng := msg.Ranges
	if rng == 0 && shot.Range > 0 {
		rng = shot.Range
	}
	if rng == 0 {
		rng = 1
	}
	receivedAt := time.Now()
	shotAt, hasShotAt := shot.EventTime()
	if !hasShotAt {
		shotAt, hasShotAt = msg.EventTime()
	}
	if !l.state.ApplyShotAt(rng, &shot, shotAt, receivedAt) {
		// Silently dropping these makes a mis-set range count look like a dead
		// lane, so say so explicitly.
		log.Printf("UDP: dropped shot for unknown range=%d (check <ranges> in config.xml)", rng)
		return
	}
	log.Printf("UDP: shot applied range=%d X=%d Y=%d DecValue=%.1f at=%v", rng, shot.X, shot.Y, shot.DecValue, shotAtOrDash(shotAt, hasShotAt))
	if l.onShot != nil {
		s := state.Shot{
			X:          shot.X,
			Y:          shot.Y,
			Distance:   shot.Distance,
			FullValue:  shot.FullValue,
			DecValue:   shot.DecValue,
			IsWarmup:   shot.IsWarmup,
			ReceivedAt: receivedAt,
		}
		if hasShotAt {
			s.At = shotAt
		}
		snap := l.state.Snapshot()
		shotIndex := 0
		for _, rs := range snap {
			if rs.RangeNum == rng {
				shotIndex = rs.ShotNumber
				break
			}
		}
		l.onShot(rng, s, shotIndex)
	}
}

func shotAtOrDash(t time.Time, ok bool) string {
	if !ok || t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339Nano)
}
