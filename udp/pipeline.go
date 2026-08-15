package udp

import (
	"log"
	"time"

	"srdashboard/state"
)

// Pipeline is the shot path after the UDP socket: decode, validate, apply, notify.
// Live OpticScore and every synthetic injector must go through Ingest.
type Pipeline struct {
	State  *state.LiveState
	OnShot ShotNotifier
}

// Ingest handles one OpticScore Event/Shot datagram (same as the listener read loop).
func (p *Pipeline) Ingest(data []byte) {
	if p == nil || p.State == nil {
		return
	}
	var msg Message
	if err := decodeOpticScoreJSON(data, &msg); err != nil {
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
	receivedAt := time.Now()
	for oi, raw := range msg.Objects {
		var shot state.ShotPayload
		if err := decodeOpticScoreJSON(raw, &shot); err != nil {
			log.Printf("UDP: failed to parse shot object[%d]: %v", oi, err)
			continue
		}
		if shot.Shooter != nil {
			warnIfReplacementInName(shot.Shooter.Firstname, shot.Shooter.Lastname)
		}
		rng := shot.Range
		if rng == 0 {
			rng = msg.Ranges
		}
		if rng == 0 {
			rng = 1
		}
		if err := ValidateShot(&shot); err != nil {
			log.Printf("UDP: dropped inconsistent shot range=%d X=%d Y=%d Distance=%.1f FullValue=%d DecValue=%.1f: %v",
				rng, shot.X, shot.Y, shot.Distance, shot.FullValue, shot.DecValue, err)
			continue
		}
		shotAt, hasShotAt := shot.EventTime()
		if !hasShotAt {
			shotAt, hasShotAt = msg.EventTime()
		}
		if !p.State.ApplyShotAt(rng, &shot, shotAt, receivedAt) {
			log.Printf("UDP: dropped shot for unknown range=%d (check <ranges> in config.xml)", rng)
			continue
		}
		log.Printf("UDP: shot applied range=%d X=%d Y=%d DecValue=%.1f at=%v", rng, shot.X, shot.Y, shot.DecValue, shotAtOrDash(shotAt, hasShotAt))
		if p.OnShot != nil {
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
			p.OnShot(rng, s, p.State.ShotNumber(rng))
		}
	}
}

// IngestPacket is the UDP pipeline without a socket — tests and simulators use this
// instead of ApplyShot + OnShot so they cannot bypass validation.
func IngestPacket(st *state.LiveState, onShot ShotNotifier, data []byte) {
	p := Pipeline{State: st, OnShot: onShot}
	p.Ingest(data)
}
