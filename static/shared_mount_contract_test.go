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

func TestTannebaumEinzelHallShowsEqualTrees(t *testing.T) {
	js := readRepoFile(t, "plugins", "tannebaum-einzel", "view.js")
	css := readRepoFile(t, "plugins", "tannebaum-einzel", "theme.css")
	mustContain(t, js, "tb-trees-gallery",
		"hall must paint every stand's tree at equal size instead of a clipped mini row")
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
