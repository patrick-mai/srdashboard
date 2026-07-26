package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"srdashboard/api"
	"srdashboard/config"
	"srdashboard/host/loader"
	"srdashboard/host/rangestate"
	"srdashboard/state"
	"srdashboard/udp"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	configPath := "config.xml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pm := loader.NewManager(cfg.Plugins.Dir)
	if _, err := pm.ScanInbox(); err != nil {
		log.Printf("scan plugin inbox: %v", err)
	}
	if err := pm.Reload(); err != nil {
		log.Printf("load plugins: %v", err)
	}

	st := state.NewLiveState(cfg.Ranges)
	ps := rangestate.NewManager(cfg.Ranges, pm, cfg.Plugins.Active)
	ps.SetLiveSource(st)
	hub := api.NewHub()
	ps.SetBroadcaster(hub)

	if err := ps.EnsureActive(); err != nil {
		log.Printf("activate plugin %q: %v", cfg.Plugins.Active, err)
	}

	udpListener, err := udp.NewListener(cfg.UDPPort, st)
	if err != nil {
		log.Fatalf("UDP listener: %v", err)
	}
	udpListener.SetShotNotifier(func(rng int, shot state.Shot, shotIndex int) {
		ps.OnShot(rng, shot, shotIndex)
		snap := st.Snapshot()
		for _, rs := range snap {
			if rs.RangeNum == rng {
				hub.BroadcastRange(rng, map[string]any{
					"type":  "live",
					"range": rs,
				})
				break
			}
		}
	})
	udpListener.Start()
	defer udpListener.Stop()

	handlers := &api.Handlers{
		State:       st,
		Cfg:         cfg,
		ConfigPath:  configPath,
		Plugins:     pm,
		PluginState: ps,
		Hub:         hub,
	}

	http.HandleFunc("/api/live", handlers.Live)
	http.HandleFunc("/api/live/reset", handlers.LiveReset)
	http.HandleFunc("/api/config", handlers.Config)
	http.HandleFunc("/api/historic", handlers.Historic)
	http.HandleFunc("/api/plugins/active", handlers.PluginsActiveList)
	http.HandleFunc("/api/plugins/session", handlers.PluginSession)
	http.HandleFunc("/api/plugins", handlers.PluginsList)
	http.HandleFunc("/api/plugins/install", handlers.PluginInstall)
	http.HandleFunc("/api/plugins/activate", handlers.PluginActivate)
	http.HandleFunc("/api/plugins/reload", handlers.PluginReload)
	http.HandleFunc("/api/plugins/scan-inbox", handlers.PluginScanInbox)
	http.HandleFunc("/api/plugins/", handlers.PluginByID)
	http.HandleFunc("/ws", hub.ServeWS)
	http.HandleFunc("/plugins/", handlers.ServePlugin)

	staticDir := "static"
	var staticHandler http.Handler
	if _, err := os.Stat(staticDir); err == nil {
		staticHandler = http.FileServer(http.Dir(staticDir))
	} else {
		sub, _ := fs.Sub(staticFS, "static")
		staticHandler = http.FileServer(http.FS(sub))
	}
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") || r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		staticHandler.ServeHTTP(w, r)
	}))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	server := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	log.Printf("HTTP server on http://localhost%s (active plugin: %s)", addr, cfg.Plugins.Active)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("HTTP server: %v", err)
	}
}
