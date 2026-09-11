package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"srdashboard/config"
	"srdashboard/host/loader"
	"srdashboard/host/rangestate"
	"srdashboard/state"
)

func portTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	root := filepath.Join("..", "plugins")
	pm := loader.NewManager(root)
	if err := pm.Reload(); err != nil {
		t.Fatalf("reload plugins: %v", err)
	}
	cfg := &config.Config{
		Ranges: 2,
		Plugins: config.Plugins{
			Dir:    root,
			Active: "classic-range",
		},
		Display: config.Display{
			AdminPort:       8080,
			PublicPort:      8081,
			ShotStrokeWidth: 0.1,
			DefaultMode:     "master",
		},
		Footer: config.Footer{
			CurrentShotValue: true,
		},
	}
	st := state.NewLiveState(cfg.Ranges)
	ps := rangestate.NewManager(cfg.Ranges, pm, cfg.Plugins.Active)
	ps.SetLiveSource(st)
	hub := NewHub()
	ps.SetBroadcaster(hub)
	if err := ps.EnsureActive(); err != nil {
		t.Fatalf("ensure active: %v", err)
	}
	return &Handlers{
		State:       st,
		Cfg:         cfg,
		Plugins:     pm,
		PluginState: ps,
		Hub:         hub,
	}
}

func TestPublicMuxRejectsMutations(t *testing.T) {
	h := portTestHandlers(t)
	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, h, h.Hub, StaticOptions{AllowConfig: false})

	cases := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, "/api/plugins/activate", `{"id":"classic-range"}`, http.StatusNotFound},
		{http.MethodPost, "/api/plugins/control", `{"action":"start"}`, http.StatusNotFound},
		{http.MethodPost, "/api/plugins/reload", ``, http.StatusNotFound},
		{http.MethodPost, "/api/live/reset?range=1", ``, http.StatusNotFound},
		{http.MethodPut, "/api/live", `{"ranges":[]}`, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/config", `{}`, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/recovery/replay", `{"log":""}`, http.StatusNotFound},
		{http.MethodPut, "/api/runtime", `{"inactiveRanges":[]}`, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/wettkampf", `{"reset":true}`, http.StatusMethodNotAllowed},
		{http.MethodGet, "/config", ``, http.StatusNotFound},
	}
	for _, tc := range cases {
		var body *strings.Reader
		if tc.body != "" {
			body = strings.NewReader(tc.body)
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(tc.method, tc.path, body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s: got %d want %d (%s)", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestPublicMuxAllowsReads(t *testing.T) {
	h := portTestHandlers(t)
	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, h, h.Hub, StaticOptions{})

	for _, path := range []string{
		"/api/mode",
		"/api/live",
		"/api/plugins",
		"/api/plugins/active",
		"/api/plugins/session",
		"/api/config",
		"/api/wettkampf",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d want 200 (%s)", path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mode", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var mode map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&mode); err != nil {
		t.Fatal(err)
	}
	if mode["role"] != RolePublic {
		t.Fatalf("role = %q want %q", mode["role"], RolePublic)
	}
}

func TestAdminMuxModeAndConfig(t *testing.T) {
	h := portTestHandlers(t)
	mux := http.NewServeMux()
	RegisterAdminRoutes(mux, h, h.Hub, StaticOptions{AllowConfig: true})

	req := httptest.NewRequest(http.MethodGet, "/api/mode", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mode: %d", rec.Code)
	}
	var mode map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&mode)
	if mode["role"] != RoleAdmin {
		t.Fatalf("role = %q want %q", mode["role"], RoleAdmin)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/plugins/activate", strings.NewReader(`{"id":"classic-range"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activate on admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCompactPathServesIndexOnAdminAndPublic(t *testing.T) {
	h := portTestHandlers(t)
	index := []byte("<!doctype html><title>compact-index</title>")
	opt := StaticOptions{
		AllowConfig: true,
		ReadIndex: func() ([]byte, error) {
			return index, nil
		},
	}

	admin := http.NewServeMux()
	RegisterAdminRoutes(admin, h, h.Hub, opt)
	for _, path := range []string{"/compact", "/compact/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		admin.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin GET %s: %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "compact-index") {
			t.Fatalf("admin GET %s did not serve index", path)
		}
	}

	pub := http.NewServeMux()
	RegisterPublicRoutes(pub, h, h.Hub, StaticOptions{ReadIndex: opt.ReadIndex})
	req := httptest.NewRequest(http.MethodGet, "/compact", nil)
	rec := httptest.NewRecorder()
	pub.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public GET /compact: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "compact-index") {
		t.Fatal("public GET /compact did not serve index")
	}
}
