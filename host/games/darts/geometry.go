package darts

// DartboardGeometry is the single source of truth for the physical board
// dimensions. The radii match the rings in plugins/darts/assets/dartboard.svg.
type DartboardGeometry struct {
	DistanceUnitsPerMm  float64 `json:"distanceUnitsPerMm"`
	AssetViewBoxUnits   float64 `json:"assetViewBoxUnits"`
	BoardRadiusMm       float64 `json:"boardRadiusMm"`
	TripleInnerRadiusMm float64 `json:"tripleInnerRadiusMm"`
	TripleOuterRadiusMm float64 `json:"tripleOuterRadiusMm"`
	DoubleInnerRadiusMm float64 `json:"doubleInnerRadiusMm"`
	DoubleOuterRadiusMm float64 `json:"doubleOuterRadiusMm"`
	OuterBullRadiusMm   float64 `json:"outerBullRadiusMm"`
	BullRadiusMm        float64 `json:"bullRadiusMm"`
}

var standardDartboardGeometry = DartboardGeometry{
	DistanceUnitsPerMm:  100,
	AssetViewBoxUnits:   451,
	BoardRadiusMm:       69.5,
	TripleInnerRadiusMm: 36.0,
	TripleOuterRadiusMm: 43.58,
	DoubleInnerRadiusMm: 62.7,
	DoubleOuterRadiusMm: 69.5,
	OuterBullRadiusMm:   9.57,
	BullRadiusMm:        4.4,
}

func dartboardGeometry() DartboardGeometry {
	return standardDartboardGeometry
}
