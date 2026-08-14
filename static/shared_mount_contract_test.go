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
		"live path must accept any shared plugin ready flag, not only F1 _f1LastVM")
	mustContain(t, js, "Stale mount must NOT tear down",
		"document the race: stale mountGen must not unhide #ranges-grid")
	// Exact anti-pattern that caused fox meadow/Scheibe ghosting.
	mustNotContain(t, js, "if (gen !== mountGen) teardownSharedHost();",
		"stale shared mount must not call teardownSharedHost (unhides classic grid)")
}

func TestPluginShellAwaitsAsyncPluginPaint(t *testing.T) {
	js := readRepoFile(t, "static", "plugin-shell.js")
	mustContain(t, js, "await fn(container, viewModel, assetsBase);",
		"overlapping live paints must not interleave without awaiting fox/F1 render")
}

func TestFoxPreservesSharedHostClassAndMasterLayout(t *testing.T) {
	js := readRepoFile(t, "plugins", "fox-on-the-run", "view.js")
	mustContain(t, js, "function setFoxSurfaceClasses",
		"fox must add classes without wiping #f1-race-master-host identity")
	mustContain(t, js, "container.classList.add('f1-race-master-host')",
		"shared host class must survive fox paints so absolute fill CSS applies")
	mustContain(t, js, "container._sharedReady = true",
		"fox master must opt into in-place live updates")
	mustContain(t, js, "foxPaintStale",
		"stale async fox paints must bail after await")
	mustContain(t, js, "fox-master-layout",
		"master skeleton must exist for hall overview")
	// Scheibe column must precede chase column (same as shooter / F1 / classic).
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
		"assigning className on the shared host drops f1-race-master-host and breaks fill")
}

func TestFoxHostCSSFillsSharedSurface(t *testing.T) {
	css := readRepoFile(t, "plugins", "fox-on-the-run", "theme.css")
	mustContain(t, css, "#f1-race-master-host.fox-view",
		"host itself carries .fox-view; child-only selector left the meadow unfilled")
	style := readRepoFile(t, "static", "style.css")
	mustContain(t, style, "#f1-race-master-host.fox-view",
		"shared host must stay opaque under fox so the classic grid cannot ghost through")
	if strings.Contains(style, "#f1-race-master-host:has(.fox-view)") &&
		strings.Contains(style, "background: transparent") {
		// Narrow: only fail if the fox host rule itself is transparent.
		idx := strings.Index(style, "#f1-race-master-host:has(.fox-view)")
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
		{"malefiz", "malefiz", "mz", "setMzSurfaceClasses", "mzPaintStale", "Malefiz starten"},
		{"maedn", "maedn", "md", "setMdSurfaceClasses", "mdPaintStale", "Spiel starten"},
	}
	for _, g := range games {
		js := readRepoFile(t, "plugins", g.dir, "view.js")
		css := readRepoFile(t, "plugins", g.dir, "theme.css")
		mustContain(t, js, "function "+g.setFn,
			g.id+" must add classes without wiping #f1-race-master-host identity")
		mustContain(t, js, "container.classList.add('f1-race-master-host')",
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
			"assigning className on the shared host drops f1-race-master-host and breaks fill")
		mustContain(t, css, "#f1-race-master-host."+g.prefix+"-view",
			"host itself carries ."+g.prefix+"-view; child-only selector left the surface unfilled")
		mustContain(t, style, "#f1-race-master-host."+g.prefix+"-view",
			"shared host must stay opaque under "+g.id+" so the classic grid cannot ghost through")
		mustContain(t, master, g.startLabel,
			g.id+" start button label")
	}
}
