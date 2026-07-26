package udp

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// normalizeOpticScoreJSON returns UTF-8 JSON bytes for OpticScore payloads.
// Live UDP and Windows log files often use Windows-1252 for German umlauts (äöüß).
// encoding/json requires UTF-8 and replaces invalid bytes with U+FFFD ("�") otherwise.
func normalizeOpticScoreJSON(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil || len(decoded) == 0 {
		return data
	}
	return decoded
}
