package udp

import (
	"encoding/json"
	"log"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// NormalizeOpticScoreJSON returns UTF-8 JSON for OpticScore UDP/log payloads.
// OpticScore on Windows often emits Windows-1252 for German umlauts (äöüß).
// encoding/json needs UTF-8; invalid sequences would otherwise become U+FFFD ("�").
// Call this on the receive path before Unmarshal — senders must not be required
// to re-encode log files.
func NormalizeOpticScoreJSON(data []byte) []byte {
	return normalizeOpticScoreJSON(data)
}

func normalizeOpticScoreJSON(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	if utf8.Valid(data) {
		return data
	}
	decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) {
		return data
	}
	return decoded
}

// decodeOpticScoreJSON unmarshals OpticScore JSON, converting Windows-1252 → UTF-8
// when needed so German umlauts survive on the receive path.
func decodeOpticScoreJSON(data []byte, dst any) error {
	normalized := normalizeOpticScoreJSON(data)
	err := json.Unmarshal(normalized, dst)
	if err == nil {
		return nil
	}
	// Rare: bytes looked like UTF-8 but JSON still failed — try forced CP1252.
	if utf8.Valid(data) {
		if decoded, decErr := charmap.Windows1252.NewDecoder().Bytes(data); decErr == nil && len(decoded) > 0 {
			if err2 := json.Unmarshal(decoded, dst); err2 == nil {
				log.Printf("UDP: accepted payload after forced Windows-1252 decode")
				return nil
			}
		}
	}
	return err
}
