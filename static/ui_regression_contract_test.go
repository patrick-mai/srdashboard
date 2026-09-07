package staticassets_test

import "testing"

func TestUIRegressionCoversReadabilityGlitches(t *testing.T) {
	py := readRepoFile(t, "scripts", "ui_regression.py")
	mustContain(t, py, "READABILITY_JS",
		"hall/tablet captures must run a DOM readability pass, not only leftover-space scrollbars")
	mustContain(t, py, "clipHits",
		"the Regeln overflow was clipped button text; leftover-space scrollbars never saw it")
	mustContain(t, py, "slimLabelLeak",
		"collapsed 2.85rem rail must not show the Regeln or Menü words")
	mustContain(t, py, "chromeOverlaps",
		"slim-rail hamburger and Regeln must not sit on top of each other")
	mustContain(t, py, "pluginErrors",
		"a visible .plugin-error means the view failed to paint")
	mustContain(t, py, "dupViews",
		"a second view.js insert throws Identifier has already been declared")
	mustContain(t, py, "cellOverlaps",
		"fox 0.0 sitting on gestellt is overlapping standings cells")
	mustContain(t, py, "wrapHits",
		"standings hint wrapping (Ludo 'zwei' on its own line) is not a cell overlap")
	mustContain(t, py, "getClientRects",
		"wrapHits must count CSS boxes, not only scrollWidth vs clientWidth")
	mustContain(t, py, "standings text wrapped",
		"a wrapHits result must fail the UI run, not only sit in the JSON")
	mustContain(t, py, `("Alex", "Alpha")`,
		"Ludo hall capture must use long names, not Test B1")
	mustContain(t, py, "shooter=names[r - 1]",
		"Ludo play must put the long names on every lane, not only stand 1")
	mustContain(t, py, "for r in range(1, 7):",
		"Ludo play must fill all 6 lanes so every standings row has a name plus the move hint")
	mustContain(t, py, "contrastHits",
		"plugin footer legend on glass over a dark scene is unreadably low-contrast")
	mustContain(t, py, "footer contrast",
		"a contrastHits result must fail the UI run, not only sit in the JSON")
	mustContain(t, py, "pageerror",
		"uncaught JS must fail the UI run, not only a screenshot")
	mustContain(t, py, `("compact", self.compact)`,
		"each game capture must include the /compact hall without the disc")
	mustContain(t, py, `BASE + "/compact"`,
		"compact page loads /compact like shooter loads /1")
}

func TestReadmeCaptureMixesClassicDisciplines(t *testing.T) {
	py := readRepoFile(t, "scripts", "capture_readme_screenshots.py")
	mustContain(t, py, "CLASSIC_RANGE_PROGRAMS",
		"the next Classic Range recapture must name the mixed-discipline table")
	mustContain(t, py, `"LP 40 Schuss"`,
		"at least one stand must be Luftpistole, not another LG 40")
	mustContain(t, py, `"KK 40 Schuss"`,
		"at least one stand must be Kleinkaliber so the hall shows the 50 m face")
	mustContain(t, py, `"LG 40 Schuss"`,
		"stand 1 stays LG 40 for the shooter screenshot")
	mustContain(t, py, `"LG 30 Schuss Auflage"`,
		"Auflage must be in the mix so condensed shows LGA and decimal scoring")
	mustContain(t, py, `disc=prog["disc"]`,
		"fill_classic must send DiscType from the per-stand program, not hardcoded LG")
	mustContain(t, py, `band=prog["band"]`,
		"LP/KK shots must use the 80 DSG teiler band or ValidateShot drops them")
}
