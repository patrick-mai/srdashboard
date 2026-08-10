package foxontherun

import "math"

func rawShotValue(dec float64, full int) float64 {
	if dec > 0 {
		return dec
	}
	if full > 0 {
		return float64(full)
	}
	return 0
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func meanFloats(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// computeEqualizers sets skill + equalizer on each active player from calibration
// (or skill overrides). fieldAvg is the mean skill across players with a skill.
func computeEqualizers(players map[string]*Player, cfg map[string]any) float64 {
	eqOn := cfgBool(cfg, "equalizerEnabled", true)
	eqMin := cfgFloat(cfg, "equalizerMin", -2)
	eqMax := cfgFloat(cfg, "equalizerMax", 2)

	overrides := map[string]any{}
	if o, ok := cfg["skillOverrides"].(map[string]any); ok {
		overrides = o
	}
	eqOverrides := map[string]any{}
	if o, ok := cfg["equalizerOverrides"].(map[string]any); ok {
		eqOverrides = o
	}

	skills := make([]float64, 0, len(players))
	for _, p := range players {
		if p == nil || !p.Active {
			continue
		}
		if v, ok := overrides[itoa(p.RangeNum)]; ok {
			p.Skill = toFloat(v, p.Skill)
			p.Calibrated = true
		} else if len(p.CalValues) > 0 {
			p.Skill = meanFloats(p.CalValues)
			p.Calibrated = true
		}
		if p.Calibrated && p.Skill > 0 {
			skills = append(skills, p.Skill)
		}
	}
	fieldAvg := meanFloats(skills)
	for _, p := range players {
		if p == nil || !p.Active {
			continue
		}
		if v, ok := eqOverrides[itoa(p.RangeNum)]; ok {
			p.Equalizer = toFloat(v, 0)
			continue
		}
		if !eqOn || !p.Calibrated || fieldAvg <= 0 {
			p.Equalizer = 0
			continue
		}
		p.Equalizer = clampFloat(fieldAvg-p.Skill, eqMin, eqMax)
	}
	return fieldAvg
}

func startBonusForFox(foxSkill, fieldAvg float64, cfg map[string]any) float64 {
	factor := cfgFloat(cfg, "startBonusFactor", 1)
	lo := cfgFloat(cfg, "startBonusMin", 0)
	hi := cfgFloat(cfg, "startBonusMax", 8)
	gap := fieldAvg - foxSkill
	return clampFloat(gap*factor, lo, hi)
}

func effectiveDelta(raw, equalizer float64) float64 {
	return math.Max(0, raw+equalizer)
}
