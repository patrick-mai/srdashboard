package darts

import "math"

// Measured radii from the photographed sheet, millimetres from the bull.
// 100 DSG = 1 mm. A boundary belongs to the inner zone (r <= outer edge).
const (
	dsgPerMM = 100.0

	innerBullMM = 4.0
	outerBullMM = 8.0
	trebleInMM  = 36.8
	trebleOutMM = 42.4
	doubleInMM  = 63.4
	doubleOutMM = 69.3
	numberInMM  = 71.4
	numberOutMM = 85.3

	// AssumedOuterDoubleDSG is the outer double wire used to set board scale.
	AssumedOuterDoubleDSG = doubleOutMM * dsgPerMM
)

// Segment numbers, clockwise from 20 at +Y, X to the right.
var segmentNumbers = []int{20, 1, 18, 4, 13, 6, 10, 15, 2, 17, 3, 19, 7, 16, 8, 11, 14, 9, 12, 5}

// Radii are one board size. Scale 1 is the photograph.
type Radii struct {
	InnerBull float64
	OuterBull float64
	TrebleIn  float64
	TrebleOut float64
	DoubleIn  float64
	DoubleOut float64
	NumberIn  float64
	NumberOut float64
}

// MeasuredRadii is the photographed board (scale 1).
func MeasuredRadii() Radii {
	return ScaledRadii(1)
}

// ScaledRadii multiplies every measured radius by scale. Ratios stay put.
func ScaledRadii(scale float64) Radii {
	if scale <= 0 {
		scale = 1
	}
	return Radii{
		InnerBull: innerBullMM * scale,
		OuterBull: outerBullMM * scale,
		TrebleIn:  trebleInMM * scale,
		TrebleOut: trebleOutMM * scale,
		DoubleIn:  doubleInMM * scale,
		DoubleOut: doubleOutMM * scale,
		NumberIn:  numberInMM * scale,
		NumberOut: numberOutMM * scale,
	}
}

// Hit is the dart value of one accepted coordinate. DecValue is not an input.
type Hit struct {
	Number     int
	Multiplier int
	Points     int
	Label      string
	IsDouble   bool
}

// Classify turns OpticScore X/Y into a dart. scale 1 uses the measured radii.
// The origin is the bull. The clock is not rotated.
func Classify(x, y int, scale float64) Hit {
	r := math.Hypot(float64(x), float64(y)) / dsgPerMM
	z := ScaledRadii(scale)
	switch {
	case r <= z.InnerBull:
		return Hit{Multiplier: 2, Points: 50, Label: "50", IsDouble: true}
	case r <= z.OuterBull:
		return Hit{Multiplier: 1, Points: 25, Label: "25", IsDouble: false}
	case r <= z.DoubleOut:
		n := segmentNumber(float64(x), float64(y))
		mult := 1
		switch {
		case r <= z.TrebleIn:
			mult = 1
		case r <= z.TrebleOut:
			mult = 3
		case r <= z.DoubleIn:
			mult = 1
		default:
			mult = 2
		}
		return numberedHit(n, mult)
	default:
		return Hit{Label: "0"}
	}
}

func numberedHit(n, mult int) Hit {
	prefix := "S"
	switch mult {
	case 2:
		prefix = "D"
	case 3:
		prefix = "T"
	}
	return Hit{
		Number:     n,
		Multiplier: mult,
		Points:     n * mult,
		Label:      prefix + itoa(n),
		IsDouble:   mult == 2,
	}
}

// segmentNumber picks the nearer wedge centre. An exact wire goes clockwise.
func segmentNumber(x, y float64) int {
	ang := math.Atan2(x, y)
	if ang < 0 {
		ang += 2 * math.Pi
	}
	seg := math.Pi / 10
	idx := int(math.Floor((ang+seg/2)/seg)) % 20
	if idx < 0 {
		idx = 0
	}
	return segmentNumbers[idx]
}

// CalibrationScale is hypot / assumed outer double. The centre cannot set the
// size. Coordinates outside ±9000 never arrive: ValidateShot drops them.
func CalibrationScale(x, y int) (float64, bool) {
	r := math.Hypot(float64(x), float64(y))
	if r <= innerBullMM*dsgPerMM {
		return 0, false
	}
	return r / AssumedOuterDoubleDSG, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
