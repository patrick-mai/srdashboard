package gameutil

import "math"

const (
	DefaultHoleInHoleMinOverlap = 0.5
	DefaultHoleInHoleMinValue   = 8.5
	DefaultShotDiameterMm       = 4.5
	DefaultDSGPerMm             = 100.0
)

func ClampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func MeanFloats(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func PlayerLabel(name string, rangeNum int) string {
	if name != "" {
		return name
	}
	return "Stand " + Itoa(rangeNum)
}

// CircleOverlapRatio returns intersection area / area of one circle (equal radii).
func CircleOverlapRatio(x1, y1, x2, y2, r float64) float64 {
	if r <= 0 {
		return 0
	}
	d := math.Hypot(x2-x1, y2-y1)
	if d >= 2*r {
		return 0
	}
	if d <= 0 {
		return 1
	}
	part := 2 * r * r * math.Acos(d/(2*r))
	part -= 0.5 * d * math.Sqrt(4*r*r-d*d)
	area := math.Pi * r * r
	if area <= 0 {
		return 0
	}
	return part / area
}

// HoleInHole reports whether the current shot repeats the previous one on the same lane.
// dec is the RAW DecValue: the 8.5 gate is deliberately not handicap-adjusted.
func HoleInHole(prevX, prevY, x, y float64, dec float64, cfg map[string]any) (ok bool, overlap float64) {
	minVal := CfgFloat(cfg, "holeInHoleMinValue", DefaultHoleInHoleMinValue)
	if dec <= minVal {
		return false, 0
	}
	dsgPerMm := CfgFloat(cfg, "dsgPerMm", DefaultDSGPerMm)
	if dsgPerMm <= 0 {
		dsgPerMm = DefaultDSGPerMm
	}
	diam := CfgFloat(cfg, "shotDiameterMm", DefaultShotDiameterMm)
	if diam <= 0 {
		diam = DefaultShotDiameterMm
	}
	r := diam / 2 * dsgPerMm
	overlap = CircleOverlapRatio(prevX, prevY, x, y, r)
	minO := CfgFloat(cfg, "holeInHoleMinOverlap", DefaultHoleInHoleMinOverlap)
	if overlap > minO {
		return true, overlap
	}
	return false, overlap
}
