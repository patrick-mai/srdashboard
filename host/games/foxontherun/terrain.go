package foxontherun

// Terrain flavor buckets (presentation). Math uses dampen/aid only.
// Labels use ASCII ae/ue/oe so the source file stays encoding-safe on Windows.

var foxMalus = []terrainFlavor{
	{ID: "broken_bridge", LabelDE: "Kaputte Bruecke"},
	{ID: "gutter", LabelDE: "Graben"},
	{ID: "puddle", LabelDE: "Pfuetze"},
	{ID: "brambles", LabelDE: "Dornen"},
}

var hunterMalus = []terrainFlavor{
	{ID: "wet_roots", LabelDE: "Nasse Wurzeln"},
	{ID: "fog_bank", LabelDE: "Nebelschwade"},
	{ID: "fence_line", LabelDE: "Zaunlinie"},
	{ID: "split_pack", LabelDE: "Geteilte Meute"},
}

var hunterBoost = []terrainFlavor{
	{ID: "fresh_scent", LabelDE: "Frischfaehrte"},
	{ID: "open_field", LabelDE: "Freies Feld"},
	{ID: "horn_call", LabelDE: "Horneruf"},
	{ID: "rising_moon", LabelDE: "Aufgehender Mond"},
}

var foxBoost = []terrainFlavor{
	{ID: "thick_cover", LabelDE: "Dickicht"},
	{ID: "false_trail", LabelDE: "Falsche Faehrte"},
	{ID: "burrow_mouth", LabelDE: "Baueingang"},
	{ID: "tailwind", LabelDE: "Rueckenwind"},
}

type terrainFlavor struct {
	ID      string
	LabelDE string
}

type terrainEffect struct {
	Side        string // "fox" | "hunter" — who is dampened or aided
	Kind        string // "dampen" | "aid"
	Amount      float64
	FlavorID    string
	FlavorLabel string
}

// computeTerrain returns dampen/aid for the upcoming shot based on current lead.
// Dampen applies to the leading side; trail aid (default 0) to the trailer.
func computeTerrain(lead float64, escapeTarget float64, shotIndex int, forFoxShot bool, cfg map[string]any) terrainEffect {
	if !cfgBool(cfg, "terrainEnabled", true) {
		return terrainEffect{}
	}
	band := cfgFloat(cfg, "terrainBand", 5)
	dampMax := cfgFloat(cfg, "terrainDampMax", 1.0)
	aidMax := cfgFloat(cfg, "trailAidMax", 0)
	if escapeTarget <= 0 {
		escapeTarget = 30
	}
	mid := escapeTarget / 2

	if lead > mid+band {
		// Fox runaway — dampen fox; optional aid hunters
		urg := (lead - (mid + band)) / mathMax(1e-6, escapeTarget-(mid+band))
		damp := mathMin(dampMax, urg*dampMax)
		if forFoxShot {
			f := pickFlavor(foxMalus, shotIndex)
			return terrainEffect{Side: "fox", Kind: "dampen", Amount: damp, FlavorID: f.ID, FlavorLabel: f.LabelDE}
		}
		if aidMax > 0 {
			aid := mathMin(aidMax, urg*aidMax)
			f := pickFlavor(hunterBoost, shotIndex)
			return terrainEffect{Side: "hunter", Kind: "aid", Amount: aid, FlavorID: f.ID, FlavorLabel: f.LabelDE}
		}
		return terrainEffect{}
	}
	if lead < mid-band {
		// Fox nearly caught — dampen hunters; optional aid fox
		urg := ((mid - band) - lead) / mathMax(1e-6, mid-band)
		damp := mathMin(dampMax, urg*dampMax)
		if !forFoxShot {
			f := pickFlavor(hunterMalus, shotIndex)
			return terrainEffect{Side: "hunter", Kind: "dampen", Amount: damp, FlavorID: f.ID, FlavorLabel: f.LabelDE}
		}
		if aidMax > 0 {
			aid := mathMin(aidMax, urg*aidMax)
			f := pickFlavor(foxBoost, shotIndex)
			return terrainEffect{Side: "fox", Kind: "aid", Amount: aid, FlavorID: f.ID, FlavorLabel: f.LabelDE}
		}
		return terrainEffect{}
	}
	return terrainEffect{}
}

func pickFlavor(list []terrainFlavor, shotIndex int) terrainFlavor {
	if len(list) == 0 {
		return terrainFlavor{}
	}
	if shotIndex < 0 {
		shotIndex = 0
	}
	return list[shotIndex%len(list)]
}

func applyTerrainToDelta(rawPlusEq float64, eff terrainEffect, shooterIsFox bool) float64 {
	delta := rawPlusEq
	if eff.Amount <= 0 {
		return mathMax(0, delta)
	}
	applies := false
	if eff.Side == "fox" && shooterIsFox {
		applies = true
	}
	if eff.Side == "hunter" && !shooterIsFox {
		applies = true
	}
	if !applies {
		return mathMax(0, delta)
	}
	if eff.Kind == "dampen" {
		return mathMax(0, delta-eff.Amount)
	}
	if eff.Kind == "aid" {
		return mathMax(0, delta+eff.Amount)
	}
	return mathMax(0, delta)
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
