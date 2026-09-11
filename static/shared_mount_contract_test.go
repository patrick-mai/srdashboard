package staticassets_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests lock the UI contracts that stopped the fox "meadow + Scheibe
// ghosting" mess. Go logic tests never see the DOM; this file fails CI if the
// mount/paint guards are removed again.

func staticRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	root := filepath.Join(staticRoot(t), "..")
	path := filepath.Join(append([]string{root}, parts...)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func mustContain(t *testing.T, body, want, why string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("missing %q — %s", want, why)
	}
}

func mustNotContain(t *testing.T, body, bad, why string) {
	t.Helper()
	if strings.Contains(body, bad) {
		t.Fatalf("found forbidden %q — %s", bad, why)
	}
}

func TestSharedMountDoesNotTeardownOnStaleGen(t *testing.T) {
	js := readRepoFile(t, "static", "master.js")
	mustContain(t, js, "host._sharedReady = true",
		"shared mount must mark host ready for in-place live updates")
	mustContain(t, js, "sharedHostReady(host)",
		"live path must accept any shared plugin ready flag, not only Autorennen _arLastVM")
	mustContain(t, js, "Stale mount must NOT tear down",
		"document the race: stale mountGen must not unhide #ranges-grid")
	// Exact anti-pattern that caused fox meadow/Scheibe ghosting.
	mustNotContain(t, js, "if (gen !== mountGen) teardownSharedHost();",
		"stale shared mount must not call teardownSharedHost (unhides classic grid)")
}

func TestPluginShellAwaitsAsyncPluginPaint(t *testing.T) {
	js := readRepoFile(t, "static", "plugin-shell.js")
	mustContain(t, js, "await fn(container, viewModel, assetsBase);",
		"overlapping live paints must not interleave without awaiting fox/Autorennen render")
}

func TestFoxPreservesSharedHostClassAndMasterLayout(t *testing.T) {
	js := readRepoFile(t, "plugins", "fox-on-the-run", "view.js")
	mustContain(t, js, "function setFoxSurfaceClasses",
		"fox must add classes without wiping #shared-master-host identity")
	mustContain(t, js, "container.classList.add('shared-master-host')",
		"shared host class must survive fox paints so absolute fill CSS applies")
	mustContain(t, js, "container._sharedReady = true",
		"fox master must opt into in-place live updates")
	mustContain(t, js, "foxPaintStale",
		"stale async fox paints must bail after await")
	mustContain(t, js, "fox-master-layout",
		"master skeleton must exist for hall overview")
	// Scheibe column must precede chase column (same as shooter / Autorennen / classic).
	masterIdx := strings.Index(js, "fox-master-layout")
	if masterIdx < 0 {
		t.Fatal("fox-master-layout missing")
	}
	chunk := js[masterIdx:min(len(js), masterIdx+900)]
	ti := strings.Index(chunk, "fox-target-col")
	ci := strings.Index(chunk, "fox-chase-col")
	if ti < 0 || ci < 0 || ti > ci {
		t.Fatal("fox master must place Scheibe (fox-target-col) left of chase map")
	}
	mustNotContain(t, js, "container.className = 'range-plugin-view fox-view fox-race-master'",
		"assigning className on the shared host drops shared-master-host and breaks fill")
}

func TestFoxHostCSSFillsSharedSurface(t *testing.T) {
	css := readRepoFile(t, "plugins", "fox-on-the-run", "theme.css")
	mustContain(t, css, "#shared-master-host.fox-view",
		"host itself carries .fox-view; child-only selector left the meadow unfilled")
	style := readRepoFile(t, "static", "style.css")
	mustContain(t, style, "#shared-master-host.fox-view",
		"shared host must stay opaque under fox so the classic grid cannot ghost through")
	if strings.Contains(style, "#shared-master-host:has(.fox-view)") &&
		strings.Contains(style, "background: transparent") {
		// Narrow: only fail if the fox host rule itself is transparent.
		idx := strings.Index(style, "#shared-master-host:has(.fox-view)")
		chunk := style[idx:min(len(style), idx+120)]
		if strings.Contains(chunk, "background: transparent") {
			t.Fatal("transparent shared host let range-grid Scheibe bleed through fox")
		}
	}
}

func TestSharedViewModelUsesMasterRangeNum(t *testing.T) {
	js := readRepoFile(t, "static", "master.js")
	mustContain(t, js, "rangeNum: 0",
		"shared VM must not pass a lane id or fox paints shooter layout on the hall screen")
}

func TestNewSharedGamesPreserveHostClassAndLayout(t *testing.T) {
	style := readRepoFile(t, "static", "style.css")
	master := readRepoFile(t, "static", "master.js")
	games := []struct {
		id, dir, prefix, setFn, staleFn, startLabel string
	}{
		{"zehner-bingo", "zehner-bingo", "zb", "setZbSurfaceClasses", "zbPaintStale", "Bingo starten"},
		{"barrikade", "barrikade", "br", "setBrSurfaceClasses", "brPaintStale", "Barrikade starten"},
		{"ludo", "ludo", "ld", "setLdSurfaceClasses", "ldPaintStale", "Ludo starten"},
		{"tauziehen", "tauziehen", "tz", "setTzSurfaceClasses", "tzPaintStale", "Tauziehen starten"},
		{"kettenreaktion", "kettenreaktion", "kr", "setKrSurfaceClasses", "krPaintStale", "Kette starten"},
		{"biathlon", "biathlon", "bt", "setBtSurfaceClasses", "btPaintStale", "Biathlon starten"},
		{"schrumpfender-kreis", "schrumpfender-kreis", "sk", "setSkSurfaceClasses", "skPaintStale", "Kreis starten"},
		{"kronen-duell", "kronen-duell", "kd", "setKdSurfaceClasses", "kdPaintStale", "Duell starten"},
		{"bank-oder-risiko", "bank-oder-risiko", "bk", "setBkSurfaceClasses", "bkPaintStale", "Risiko starten"},
		{"ko-pokal", "ko-pokal", "kp", "setKpSurfaceClasses", "kpPaintStale", "Pokal starten"},
		{"schiessgolf", "schiessgolf", "sg", "setSgSurfaceClasses", "sgPaintStale", "Golf starten"},
		{"turmbau", "turmbau", "tw", "setTwSurfaceClasses", "twPaintStale", "Turm starten"},
		{"ansage-duell", "ansage-duell", "ad", "setAdSurfaceClasses", "adPaintStale", "Ansage starten"},
	}
	for _, g := range games {
		js := readRepoFile(t, "plugins", g.dir, "view.js")
		css := readRepoFile(t, "plugins", g.dir, "theme.css")
		mustContain(t, js, "function "+g.setFn,
			g.id+" must add classes without wiping #shared-master-host identity")
		mustContain(t, js, "container.classList.add('shared-master-host')",
			g.id+" shared host class must survive paints so absolute fill CSS applies")
		mustContain(t, js, "container._sharedReady = true",
			g.id+" master must opt into in-place live updates")
		mustContain(t, js, g.staleFn,
			g.id+" stale async paints must bail after await")
		mustContain(t, js, g.prefix+"-master-layout",
			g.id+" master skeleton must exist for hall overview")
		masterIdx := strings.Index(js, g.prefix+"-master-layout")
		if masterIdx < 0 {
			t.Fatalf("%s master layout missing", g.id)
		}
		chunk := js[masterIdx:min(len(js), masterIdx+900)]
		ti := strings.Index(chunk, g.prefix+"-target-col")
		bi := strings.Index(chunk, g.prefix+"-board-col")
		if ti < 0 || bi < 0 || ti > bi {
			t.Fatalf("%s master must place Scheibe (%s-target-col) left of board", g.id, g.prefix)
		}
		mustNotContain(t, js, "container.className = 'range-plugin-view "+g.prefix+"-view",
			"assigning className on the shared host drops shared-master-host and breaks fill")
		mustContain(t, css, "#shared-master-host."+g.prefix+"-view",
			"host itself carries ."+g.prefix+"-view; child-only selector left the surface unfilled")
		mustContain(t, style, "#shared-master-host."+g.prefix+"-view",
			"shared host must stay opaque under "+g.id+" so the classic grid cannot ghost through")
		mustContain(t, master, g.startLabel,
			g.id+" start button label")
		mustContain(t, js, g.prefix+"-compact-layout",
			g.id+" compact hall skeleton must exist")
		mustContain(t, js, "isCompact ? '' : '<div class=\""+g.prefix+"-scheibe-wrap\"",
			g.id+" compact skeleton must omit the target disc")
		mustContain(t, css, g.prefix+"-compact-layout ."+g.prefix+"-main",
			g.id+" compact CSS must give the board the leftover width")
	}
}

func TestAutorennenPreservesSharedHostClassAndLayout(t *testing.T) {
	js := readRepoFile(t, "plugins", "autorennen", "view.js")
	css := readRepoFile(t, "plugins", "autorennen", "theme.css")
	style := readRepoFile(t, "static", "style.css")
	master := readRepoFile(t, "static", "master.js")
	mustContain(t, js, "window.SRPluginViews['autorennen']",
		"hall/shooter paint looks up SRPluginViews[pluginId]; a mismatched key leaves the surface blank")
	mustContain(t, js, "container.classList.add('shared-master-host')",
		"shared host class must survive Autorennen paints so absolute fill CSS applies")
	mustContain(t, js, "container._sharedReady = true",
		"Autorennen master must opt into in-place live updates")
	mustContain(t, js, "ar-master-layout",
		"master skeleton must exist for hall overview")
	mustNotContain(t, js, "container.className = 'range-plugin-view autorennen-view",
		"assigning className on the shared host drops shared-master-host and blanks the hall")
	mustContain(t, css, "#shared-master-host.autorennen-view",
		"host itself carries .autorennen-view; child-only selector left the track unfilled")
	mustContain(t, style, "#shared-master-host.autorennen-view",
		"shared host must stay opaque under Autorennen so the classic grid cannot ghost through")
	mustContain(t, master, "id === 'autorennen'",
		"isSharedPlugin fallback / start-button wiring must use the new id or the hall stays on the range grid")
	mustContain(t, master, "Rennen starten",
		"Autorennen start button label")
	mustNotContain(t, css, "max-height: 22%",
		"capping shooter standings at 22% forces a scrollbar while unused space sits below")
	mustContain(t, style, "#master-chrome[hidden]",
		"display:flex on #master-chrome must not override [hidden] or shooter pages are 200vh")
}

func TestFoxScheibeKeepsRoundRings(t *testing.T) {
	css := readRepoFile(t, "plugins", "fox-on-the-run", "theme.css")
	idx := strings.Index(css, ".fox-scheibe-frame svg")
	if idx < 0 {
		t.Fatal("missing .fox-scheibe-wrap svg rule")
	}
	chunk := css[idx:min(len(css), idx+500)]
	if strings.Contains(chunk, "drop-shadow") {
		t.Fatal("drop-shadow on the target SVG ghosts a second set of ring strokes")
	}
	if strings.Contains(chunk, "100cqi") {
		t.Fatal("container-query width/height on the SVG stretched the disc off-square")
	}
}

func TestZehnerBingoHallUsesFillGrid(t *testing.T) {
	js := readRepoFile(t, "plugins", "zehner-bingo", "view.js")
	css := readRepoFile(t, "plugins", "zehner-bingo", "theme.css")
	mustContain(t, js, "zb-cards-' + cardsCols(ordered.length)",
		"card grid column count must follow seated players (6 → 3×2)")
	mustContain(t, css, ".zb-cards-3",
		"6-stand halls use a 3-column card grid")
	mustNotContain(t, css, "minmax(150px",
		"150px auto-fit left bingo cards small in the leftover board column")
}

func TestSharedGamesFillLeftoverBoardColumn(t *testing.T) {
	tiles := []struct{ dir, pfx string }{
		{"biathlon", "bt"},
		{"kettenreaktion", "kr"},
		{"schiessgolf", "sg"},
		{"ansage-duell", "ad"},
		{"kronen-duell", "kd"},
		{"bank-oder-risiko", "bk"},
		{"turmbau", "tw"},
	}
	for _, g := range tiles {
		js := readRepoFile(t, "plugins", g.dir, "view.js")
		css := readRepoFile(t, "plugins", g.dir, "theme.css")
		mustContain(t, js, "function cardsCols",
			g.dir+" must pick grid columns from seated stands (6 → 3×2)")
		mustContain(t, css, "."+g.pfx+"-cards-3",
			g.dir+" 6-stand hall must use a 3-column tile grid")
		mustContain(t, css, "minmax(0, 1.45fr)",
			g.dir+" board column must get the leftover width")
		mustContain(t, css, "minmax(12.5rem, 13.75rem)",
			g.dir+" compact stats rail must stay slim")
	}
	boards := []string{"barrikade", "ludo", "schrumpfender-kreis", "tauziehen"}
	for _, dir := range boards {
		css := readRepoFile(t, "plugins", dir, "theme.css")
		mustContain(t, css, "minmax(0, 1.45fr)",
			dir+" shared board must get leftover width, not a 3×2 of copies")
		mustContain(t, css, "minmax(12.5rem, 13.75rem)",
			dir+" compact stats rail must stay slim")
	}
	br := readRepoFile(t, "plugins", "barrikade", "theme.css")
	mustNotContain(t, br, ".br-board-svg { width: 100%; height: auto;",
		"height:auto on the citadel SVG left the path small in the leftover column")
}

func TestTannebaumEinzelHallShowsEqualTrees(t *testing.T) {
	js := readRepoFile(t, "plugins", "tannebaum-einzel", "view.js")
	css := readRepoFile(t, "plugins", "tannebaum-einzel", "theme.css")
	mustContain(t, js, "tb-trees-gallery",
		"hall must paint every stand's tree at equal size instead of a clipped mini row")
	mustContain(t, css, "minmax(0, 1.45fr)",
		"team and shooter tree columns must get more than half of .tb-main")
	mustNotContain(t, css, "max-height: 36%",
		"capping the mini tree row at 36% plus overflow-y:hidden cut off the trunks")
	mustNotContain(t, css, "max-height: 220px",
		"fixed 220px mini frames cropped the bottom-left of each small tree")
}

func TestLudoViewHonorsBoardArms(t *testing.T) {
	js := readRepoFile(t, "plugins", "ludo", "view.js")
	mustContain(t, js, "game.boardArms",
		"2-player matches send boardArms=4; ignoring it draws a 2-point star with almost no fields")
}

func TestCompactHallIsADedicatedView(t *testing.T) {
	app := readRepoFile(t, "static", "app.js")
	master := readRepoFile(t, "static", "master.js")
	mustContain(t, app, "/^\\/compact\\/?$/",
		"pathname /compact must resolve as display compact like /1 is shooter")
	mustContain(t, app, "qDisplay === 'compact'",
		"?display=compact must resolve the compact hall")
	mustContain(t, master, "btn-compact-toggle",
		"Compact lives next to Vollbild in the master menu")
	mustContain(t, master, "location.assign(compactOn ? '/' : '/compact')",
		"Compact loads /compact; it must not restyle the current master DOM")
}

func TestFoxAndTannebaumCompactOmitScheibe(t *testing.T) {
	fox := readRepoFile(t, "plugins", "fox-on-the-run", "view.js")
	mustContain(t, fox, "fox-compact-layout",
		"fox compact hall skeleton must exist")
	mustContain(t, fox, "compact ? '' : '<div class=\"fox-scheibe-wrap\"",
		"fox compact must omit the target disc")
	css := readRepoFile(t, "plugins", "fox-on-the-run", "theme.css")
	mustContain(t, css, ".fox-compact-layout .fox-main",
		"fox compact CSS must give the chase map the leftover width")

	for _, dir := range []string{"tannebaum-einzel", "tannebaum-team"} {
		js := readRepoFile(t, "plugins", dir, "view.js")
		tbCSS := readRepoFile(t, "plugins", dir, "theme.css")
		mustContain(t, js, "tb-compact-layout",
			dir+" compact hall skeleton must exist")
		mustContain(t, js, "isCompact ? '' : '<div class=\"tb-scheibe-wrap\"",
			dir+" compact must omit the target disc")
		mustContain(t, tbCSS, ".tb-compact-layout .tb-main",
			dir+" compact CSS must give the trees the leftover width")
	}
}

func TestClassicCondensedHeaderSwitchDoesNotMixChrome(t *testing.T) {
	core := readRepoFile(t, "static", "target-core.js")
	master := readRepoFile(t, "static", "master.js")
	crc := readRepoFile(t, "plugins", "classic-range-condensed", "view.js")
	classic := readRepoFile(t, "plugins", "classic-range", "view.js")

	mustContain(t, core, "function setHallPluginId",
		"header paint must know which hall plugin is active, not only the crc-panel class")
	mustContain(t, core, `hallPluginId === 'classic-range-condensed'`,
		"Classic QR/chip/title chrome must not paint while Condensed is the hall plugin")
	mustContain(t, core, "delete header._crcHtml",
		"Classic header rebuild must drop the Condensed HTML cache or the next fill skips")
	mustContain(t, core, `header.querySelector('.crc-header-line')`,
		"switching back to Classic must wipe the Condensed header line, not reuse it")
	mustContain(t, master, "core.setHallPluginId(id || '')",
		"activate must set the hall plugin id before live frames paint mixed headers")
	mustContain(t, crc, "header.querySelector('.range-header-top')",
		"Condensed must treat a Classic header (QR/chip) as stale even when _crcHtml matches")
	mustContain(t, crc, "return ensureTargetRegistry(assetsBase).then",
		"plugin-shell must await Condensed paint or some lanes keep Classic headers")
	mustContain(t, crc, "if (hall && hall !== PLUGIN_ID) return",
		"a late Condensed paint after switching back to Classic must not rewrite the header")
	mustContain(t, classic, "return ensureClassicTargetRegistry(assetsBase).then(paint)",
		"plugin-shell must await Classic paint the same way as Condensed")
}

func TestClassicCondensedCurrentShotColorByValue(t *testing.T) {
	crc := readRepoFile(t, "plugins", "classic-range-condensed", "view.js")
	css := readRepoFile(t, "plugins", "classic-range-condensed", "theme.css")
	core := readRepoFile(t, "static", "target-core.js")

	mustContain(t, core, "data-dec-value",
		"shot circles must expose DecValue so Condensed can color the current pellet")
	mustContain(t, crc, "function shotBandClass(value)",
		"Condensed must classify the current shot as 10 / 9 / other")
	mustContain(t, crc, "if (value >= 10) return 'crc-shot-10'",
		"10.0–10.9 stay red")
	mustContain(t, crc, "if (value >= 9) return 'crc-shot-9'",
		"all 9s become yellow")
	mustContain(t, crc, "return 'crc-shot-low'",
		"every other current shot is green")
	mustContain(t, css, "circle:not(.is-last)",
		"older pellets stay grey")
	mustContain(t, css, "fill: #9a9a9a !important",
		"older pellets keep the existing grey fill")
	mustContain(t, css, "fill: #d62828 !important",
		"current 10.0–10.9 stay red")
	mustContain(t, css, ".crc-shot-9",
		"current 9s are yellow")
	mustContain(t, css, "fill: #e8b923 !important",
		"current 9s are yellow")
	mustContain(t, css, ".crc-shot-low",
		"current 8-and-below are green")
	mustContain(t, css, "fill: #2d9e4f !important",
		"current 8-and-below are green")
}

func TestClassicCondensedWettkampfTileSlot(t *testing.T) {
	core := readRepoFile(t, "static", "target-core.js")
	master := readRepoFile(t, "static", "master.js")
	crc := readRepoFile(t, "plugins", "classic-range-condensed", "view.js")
	css := readRepoFile(t, "plugins", "classic-range-condensed", "theme.css")

	mustContain(t, master, ">Wettkampf</label>",
		"Bahnwahl must offer a Wettkampf checkbox next to Bahnen")
	mustContain(t, master, `data-slot="wettkampf"`,
		"Wettkampf must not use data-range or Bahn deactivate would ResetRange")
	mustContain(t, core, "function ensureWettkampfPanel",
		"the extra grid cell must be created beside Bahn 1…N")
	mustContain(t, core, `data-slot="wettkampf"`,
		"the Wettkampf panel must be a named slot, not a fake Bahn number")
	mustContain(t, core, "if (isWettkampfPanel(panel)) return;",
		"ensurePluginPanels 1…N cleanup must keep the Wettkampf slot")
	mustContain(t, core, "if (isWettkampfPanel(panel)) {",
		"hideIdleRanges must not treat the Wettkampf tile as an idle Bahn")
	mustContain(t, core, "grid.classList.toggle('has-wettkampf', !!wk)",
		"Wettkampf must not count as a 7th Bahn cell in applyLayout")
	mustContain(t, core, `wk.style.gridRow = '1 / -1'`,
		"Wettkampf sits in an extra right-hand column spanning the hall rows")
	mustContain(t, core, "function packRangeColumns",
		"hidden idle Bahnen must not leave empty 3+1 holes beside Wettkampf")
	mustContain(t, crc, "paintWettkampf: paintWettkampf",
		"live/WS updates must paint the Wettkampf tile in place")
	mustContain(t, crc, "function openWettkampfModal",
		"team assignment lives on the tile, not /config")
	mustContain(t, crc, "function extrasByTeamId",
		"excluded club shooters stay listed under their Mannschaft, separated")
	mustContain(t, crc, "function extraTeamId",
		"ohne Mannschaft grouping must work as soon as the shooter is marked, not after the program ends")
	mustContain(t, crc, "function teamUsesDecimal",
		"Auflage / LGA30 Wettkampf totals must use DecValue, not integer rings")
	mustContain(t, crc, "function clubsWithMultipleTeams",
		"one club with several Mannschaften must color Bahn and Wettkampf headers by team")
	mustContain(t, css, ".crc-wk-extras",
		"club shooters outside the Mannschaft sit below the team list, not in the Summe")
	mustContain(t, css, ".crc-wettkampf-panel",
		"the extra cell needs CRC layout chrome")
	mustContain(t, crc, "const visible = teams.filter",
		"empty leftover Mannschaften (Adler I / Mitte I) must not occupy the Wettkampf tile")
	mustContain(t, css, ".crc-wk-teams",
		"Mannschaften stack in the side column instead of sitting side by side")
	mustContain(t, css, "flex: 0 0 auto",
		"teams size to their shooters so a refresh cannot clip A/B/C/Z off the frame")
}

func TestAnalysePluginSelectsSessionResults(t *testing.T) {
	js := readRepoFile(t, "plugins", "analyse", "view.js")
	css := readRepoFile(t, "plugins", "analyse", "theme.css")
	master := readRepoFile(t, "static", "master.js")
	core := readRepoFile(t, "static", "target-core.js")
	mustContain(t, js, "analyse-select",
		"Analyse needs a session-result selector, not a live Bahn grid")
	mustContain(t, js, "renderClassicRangeView",
		"Analyse must reuse Classic Range paint for the selected result")
	mustContain(t, js, "/api/analyse/results",
		"Analyse reads frozen session results, not only live lanes")
	mustContain(t, css, ".analyse-layout",
		"one result fills the shared host")
	mustContain(t, master, "id === 'analyse'",
		"Analyse is shared or the hall stays on the range grid")
	mustContain(t, master, "function isDisplayPlugin",
		"Analyse must not show game Start on the side rail")
	mustContain(t, core, "sessionResultId",
		"QR on an archived result must not fetch the live Bahn")
	mustContain(t, core, "result=",
		"QR fetch must accept a frozen result id")
}

