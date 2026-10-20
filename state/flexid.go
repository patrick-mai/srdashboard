package state

import (
	"bytes"
	"encoding/json"
	"strings"
)

// FlexID is a DISAG identifier that arrives as a JSON string or number.
// Club.ID (Vereinsnummer) and Shooter.Identification (Passnummer) are
// documented as strings; live packets sometimes send a bare number.
type FlexID string

// UnmarshalJSON accepts a string, a number, or null. Any other JSON type
// becomes empty so a stray value cannot reject the whole shot.
func (s *FlexID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = FlexID(strings.TrimSpace(v))
		return nil
	}
	if b[0] == '-' || (b[0] >= '0' && b[0] <= '9') {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			return err
		}
		*s = FlexID(n.String())
		return nil
	}
	*s = ""
	return nil
}
