package udp

import (
	"encoding/json"
	"math"
	"net"
	"time"

	"srdashboard/state"
)

// ShotPacketOpts describes one synthetic OpticScore Shot UDP payload.
type ShotPacketOpts struct {
	Range      int
	X          int
	Y          int
	Distance   float64
	DecValue   float64
	IsWarmup   bool
	Shooter    string
	ShotAt     time.Time // zero → omit ShotDateTime (server uses receive time)
	MenuItem   string
}

// BuildShotPacket returns JSON bytes for a DISAG Event/Shot message.
func BuildShotPacket(opts ShotPacketOpts) ([]byte, error) {
	rng := opts.Range
	if rng <= 0 {
		rng = 1
	}
	full := int(math.Floor(opts.DecValue))
	if full < 0 {
		full = 0
	}
	if full > 10 {
		full = 10
	}
	dist := opts.Distance
	if dist == 0 && opts.DecValue >= 10 {
		dist = 0.1
	}
	obj := map[string]any{
		"X":         opts.X,
		"Y":         opts.Y,
		"Distance":  dist,
		"FullValue": full,
		"DecValue":  opts.DecValue,
		"Range":     rng,
		"IsWarmup":  opts.IsWarmup,
	}
	if !opts.ShotAt.IsZero() {
		obj["ShotDateTime"] = state.FormatOpticScoreTime(opts.ShotAt)
	}
	if opts.Shooter != "" {
		obj["Shooter"] = map[string]any{
			"Firstname": opts.Shooter,
			"Lastname":  "",
		}
	}
	if opts.MenuItem != "" {
		obj["MenuItem"] = map[string]any{
			"MenuItemName": opts.MenuItem,
		}
	}
	msg := map[string]any{
		"MessageType": "Event",
		"MessageVerb": "Shot",
		"Ranges":      rng,
		"Objects":     []any{obj},
	}
	return json.Marshal(msg)
}

// SendShotPacket dials UDP and sends one shot packet.
func SendShotPacket(addr string, opts ShotPacketOpts) error {
	data, err := BuildShotPacket(opts)
	if err != nil {
		return err
	}
	return SendRawPacket(addr, data)
}

// SendRawPacket sends pre-built JSON to a UDP host:port (e.g. "127.0.0.1:30169").
func SendRawPacket(addr string, data []byte) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(data)
	return err
}
