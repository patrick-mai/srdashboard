package rwkbericht

import "time"

// Shooter is one row in the RWK report.
type Shooter struct {
	Name     string
	Passnr   string
	Jahrgang string
	Ergebnis string
}

// Side is the home or guest half of the report.
type Side struct {
	Disziplin  string
	Verein     string
	Durchgang  string
	Gruppe     string
	Mannschaft string
	Members    []Shooter
	Summe      string
	Einzel     []Shooter
}

// Bericht is the filled RWK report. Empty strings are left blank in the form.
type Bericht struct {
	Datum string
	Heim  Side
	Gast  Side
}

// DateLayout is the date written into the datum field.
const DateLayout = "02.01.2006"

// Today returns the local date in the report layout.
func Today(now time.Time) string {
	return now.In(time.Local).Format(DateLayout)
}
