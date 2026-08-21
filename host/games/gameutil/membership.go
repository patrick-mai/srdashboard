package gameutil

// InactiveSetFromParams returns inactive lane numbers when params includes
// inactiveRanges (an empty list means every lane is active). present is false
// when the key is missing so callers can leave existing membership alone.
func InactiveSetFromParams(params map[string]any) (set map[int]bool, present bool) {
	if params == nil {
		return nil, false
	}
	v, ok := params["inactiveRanges"]
	if !ok {
		return nil, false
	}
	set = map[int]bool{}
	for _, n := range ParseIntList(v) {
		if n > 0 {
			set[n] = true
		}
	}
	return set, true
}

func IntSet(nums []int) map[int]bool {
	out := map[int]bool{}
	for _, n := range nums {
		if n > 0 {
			out[n] = true
		}
	}
	return out
}

// LiveActive reads the per-lane active flag from a sync_live entry.
// Missing key keeps def (usually the value from inactiveRanges).
func LiveActive(m map[string]any, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m["active"]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	}
	return def
}

// WarmupDiscard reports whether a shot is still Probe, and whether this
// non-warmup event should open competition for every active lane.
// Once the field is open, warmup-flagged shots on other lanes count as play.
func WarmupDiscard(fieldOpen, isWarmup bool) (discard, openNow bool) {
	if fieldOpen {
		return false, false
	}
	if isWarmup {
		return true, false
	}
	return false, true
}

func IsWarmupShot(liveWarmup, shotWarmup bool) bool {
	return liveWarmup || shotWarmup
}
