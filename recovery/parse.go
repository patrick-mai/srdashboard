package recovery

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"srdashboard/state"
	"srdashboard/udp"
)

// Console line written by Pipeline.Ingest, optionally prefixed with the Go log timestamp.
var consoleShotLine = regexp.MustCompile(`UDP:\s*shot applied range=(\d+)\s+X=([+-]?\d+)\s+Y=([+-]?\d+)\s+DecValue=([+-]?[\d.]+)(?:\s+at=(\S+))?`)

var goLogPrefix = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`)

// ParseLog turns pasted log text into OpticScore Event/Shot JSON packets ready for ingest.
func ParseLog(text string) [][]byte {
	text = strings.TrimPrefix(text, "\uFEFF")
	var out [][]byte
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if pkt := parseLogLine(line); len(pkt) > 0 {
			out = append(out, pkt)
		}
	}
	return out
}

func parseLogLine(line []byte) []byte {
	normalized := udp.NormalizeOpticScoreJSON(line)
	if pkt, ok := parseJSONLine(normalized); ok {
		return pkt
	}
	if pkt, ok := parseConsoleLine(string(normalized)); ok {
		return pkt
	}
	// Timestamp or other prefix before JSON, e.g. "2026-01-01 12:00:00 {...}"
	if idx := bytes.IndexByte(normalized, '{'); idx > 0 {
		if pkt, ok := parseJSONLine(normalized[idx:]); ok {
			return pkt
		}
	}
	return nil
}

func parseJSONLine(line []byte) ([]byte, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return nil, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, false
	}
	msgType := strings.TrimSpace(jsonStringCI(raw, "MessageType"))
	msgVerb := strings.TrimSpace(jsonStringCI(raw, "MessageVerb"))
	if strings.EqualFold(msgType, "Event") && strings.EqualFold(msgVerb, "Shot") {
		return bytes.Clone(line), true
	}
	// Bare shot object without envelope (some exports).
	if _, hasX := raw["X"]; hasX {
		if _, hasDec := raw["DecValue"]; hasDec {
			return wrapShotObject(line)
		}
	}
	return nil, false
}

func wrapShotObject(line []byte) ([]byte, bool) {
	var shot map[string]any
	if err := json.Unmarshal(line, &shot); err != nil {
		return nil, false
	}
	rng := 1
	switch v := shot["Range"].(type) {
	case float64:
		rng = int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			rng = int(n)
		}
	}
	msg := map[string]any{
		"MessageType": "Event",
		"MessageVerb": "Shot",
		"Ranges":      rng,
		"Objects":     []any{shot},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, false
	}
	return out, true
}

func parseConsoleLine(line string) ([]byte, bool) {
	m := consoleShotLine.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	rng, _ := strconv.Atoi(m[1])
	x, _ := strconv.Atoi(m[2])
	y, _ := strconv.Atoi(m[3])
	dec, _ := strconv.ParseFloat(m[4], 64)
	atField := ""
	if len(m) > 5 {
		atField = m[5]
	}
	return consoleShot(rng, x, y, dec, consoleEventTime(line, atField))
}

func consoleEventTime(line, atField string) time.Time {
	if t, ok := state.ParseOpticScoreTime(atField); ok {
		return t
	}
	if pm := goLogPrefix.FindStringSubmatch(line); len(pm) == 2 {
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", pm[1], time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func jsonStringCI(raw map[string]json.RawMessage, key string) string {
	if v, ok := raw[key]; ok {
		return trimJSONString(v)
	}
	lower := strings.ToLower(key)
	for k, v := range raw {
		if strings.ToLower(k) == lower {
			return trimJSONString(v)
		}
	}
	return ""
}

func trimJSONString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err == nil {
			return decoded
		}
	}
	return s
}
