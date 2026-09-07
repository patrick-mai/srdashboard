package recovery

import (
	"encoding/json"
	"math"
	"time"

	"srdashboard/state"
	"srdashboard/udp"
)

// consoleShot builds an ingest packet from a parsed console line.
// DiscType is omitted so ValidateShot may accept either the LG or LP/KK teiler band;
// Distance is the hypot of the logged coordinates.
func consoleShot(rng, x, y int, dec float64, at time.Time) ([]byte, bool) {
	dist := math.Round(math.Hypot(float64(x), float64(y))*10) / 10
	obj := map[string]any{
		"X":         x,
		"Y":         y,
		"Distance":  dist,
		"FullValue": udp.FullValueOf(dec),
		"DecValue":  dec,
		"Range":     rng,
	}
	if !at.IsZero() {
		obj["ShotDateTime"] = state.FormatOpticScoreTime(at)
	}
	msg := map[string]any{
		"MessageType": "Event",
		"MessageVerb": "Shot",
		"Ranges":      rng,
		"Objects":     []any{obj},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, false
	}
	return out, true
}
