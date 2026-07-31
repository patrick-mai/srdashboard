package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"srdashboard/config"
	"srdashboard/host/loader"
	"srdashboard/state"
)

func testHandlers(t *testing.T, token string) (*Handlers, string) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "classic-range")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "view.js"), []byte("// view"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("top secret"), 0644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.xml")
	cfg := &config.Config{
		UDPPort:       30169,
		Ranges:        2,
		LayoutColumns: 2,
		Plugins:       config.Plugins{Dir: filepath.Join(dir, "plugins"), Active: "classic-range"},
		Display:       config.Display{ControlToken: token, ShotStrokeWidth: 0.1},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	return &Handlers{
		State:      state.NewLiveState(2),
		Cfg:        cfg,
		ConfigPath: cfgPath,
		Plugins:    loader.NewManager(filepath.Join(dir, "plugins")),
	}, dir
}

func TestConfigGetDoesNotLeakControlToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")

	rec := httptest.NewRecorder()
	h.Config(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatalf("GET /api/config leaked the control token: %s", rec.Body.String())
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.ControlTokenSet {
		t.Fatal("controlTokenSet should be true when a token is configured")
	}
	if resp.ControlToken != nil {
		t.Fatalf("controlToken should be omitted, got %q", *resp.ControlToken)
	}
}

func TestConfigPutKeepsTokenWhenOmitted(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")

	body := `{"udpPort":30169,"ranges":2,"layoutColumns":2,"pluginsDir":"plugins","activePlugin":"classic-range","shotStrokeWidth":0.1}`
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	req.Header.Set("X-SR-Control-Token", "s3cret")
	rec := httptest.NewRecorder()
	h.Config(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := h.cfgSnapshot().Display.ControlToken; got != "s3cret" {
		t.Fatalf("token = %q, want it preserved", got)
	}
}

func TestConfigPutSetsAndClearsToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	base := `"udpPort":30169,"ranges":2,"layoutColumns":2,"pluginsDir":"plugins","activePlugin":"classic-range","shotStrokeWidth":0.1`

	put := func(extra string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader("{"+base+","+extra+"}"))
		req.Header.Set("X-SR-Control-Token", h.cfgSnapshot().Display.ControlToken)
		rec := httptest.NewRecorder()
		h.Config(rec, req)
		return rec
	}

	if rec := put(`"controlToken":"newtoken"`); rec.Code != http.StatusOK {
		t.Fatalf("set: status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := h.cfgSnapshot().Display.ControlToken; got != "newtoken" {
		t.Fatalf("token = %q, want newtoken", got)
	}
	if rec := put(`"controlToken":""`); rec.Code != http.StatusOK {
		t.Fatalf("clear: status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := h.cfgSnapshot().Display.ControlToken; got != "" {
		t.Fatalf("token = %q, want cleared", got)
	}
}

func TestConfigPutRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		"zero ranges":       `{"udpPort":30169,"ranges":0,"layoutColumns":2,"pluginsDir":"plugins","activePlugin":"classic-range"}`,
		"negative ranges":   `{"udpPort":30169,"ranges":-4,"layoutColumns":2,"pluginsDir":"plugins","activePlugin":"classic-range"}`,
		"bad udp port":      `{"udpPort":70000,"ranges":2,"layoutColumns":2,"pluginsDir":"plugins","activePlugin":"classic-range"}`,
		"zero columns":      `{"udpPort":30169,"ranges":2,"layoutColumns":0,"pluginsDir":"plugins","activePlugin":"classic-range"}`,
		"empty plugins dir": `{"udpPort":30169,"ranges":2,"layoutColumns":2,"pluginsDir":"","activePlugin":"classic-range"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := testHandlers(t, "")
			rec := httptest.NewRecorder()
			h.Config(rec, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLiveMutationsRequireControlToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")

	rec := httptest.NewRecorder()
	h.LiveReset(rec, httptest.NewRequest(http.MethodPost, "/api/live/reset?range=1", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated reset: status = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodPut, "/api/live", strings.NewReader(`{"ranges":[]}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated replace: status = %d, want 403", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/live/reset?range=1", nil)
	req.Header.Set("X-SR-Control-Token", "s3cret")
	rec = httptest.NewRecorder()
	h.LiveReset(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated reset: status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestCheckControlTokenRejectsWrongToken(t *testing.T) {
	h, _ := testHandlers(t, "s3cret")
	req := httptest.NewRequest(http.MethodPost, "/api/live/reset?range=1", nil)
	req.Header.Set("X-SR-Control-Token", "wrong")
	if h.checkControlToken(req) {
		t.Fatal("wrong token accepted")
	}
}

func TestServePluginRejectsTraversal(t *testing.T) {
	h, dir := testHandlers(t, "")

	// Requests go through the mux in production, which also cleans paths; this
	// exercises the handler's own containment check.
	for _, rest := range []string{"../secret.txt", "../../secret.txt", "sub/../../secret.txt"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/plugins/classic-range/x", nil)
		req.URL.Path = "/plugins/classic-range/" + rest
		h.ServePlugin(rec, req)
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "top secret") {
			t.Fatalf("traversal %q served %s", rest, filepath.Join(dir, "secret.txt"))
		}
	}

	rec := httptest.NewRecorder()
	h.ServePlugin(rec, httptest.NewRequest(http.MethodGet, "/plugins/classic-range/view.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("legitimate asset: status = %d", rec.Code)
	}
}

func TestIsWithin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "srv", "plugins")
	cases := []struct {
		child string
		want  bool
	}{
		{filepath.Join(root, "classic-range", "view.js"), true},
		{root, true},
		{filepath.Join(root, "..", "config.xml"), false},
		{filepath.Join(root, "..", "plugins-evil", "view.js"), false},
		{filepath.Join(string(filepath.Separator), "etc", "passwd"), false},
	}
	for _, tc := range cases {
		if got := isWithin(root, tc.child); got != tc.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", root, tc.child, got, tc.want)
		}
	}
}

func TestServePluginBlocksConfigXML(t *testing.T) {
	h, _ := testHandlers(t, "")
	rec := httptest.NewRecorder()
	h.ServePlugin(rec, httptest.NewRequest(http.MethodGet, "/plugins/classic-range/config.xml", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSameOriginRejectsForeignOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true},
		{"http://range.local:8080", true},
		{"https://range.local:8080", true},
		{"http://evil.example", false},
		{"http://range.local:9999", false},
		{"::not a url::", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.Host = "range.local:8080"
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if got := sameOrigin(req); got != tc.want {
			t.Errorf("sameOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}

func TestDecodeJSONBodyRejectsOversizedPayload(t *testing.T) {
	huge := strings.Repeat("a", maxJSONBody+1024)
	req := httptest.NewRequest(http.MethodPut, "/api/live", strings.NewReader(`{"junk":"`+huge+`"}`))
	rec := httptest.NewRecorder()
	var dst map[string]any
	if decodeJSONBody(rec, req, &dst) {
		t.Fatal("oversized body accepted")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
