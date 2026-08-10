# SRDashboard

Live shooting-range **display** for DISAG OpticScore. Shows configurable ranges with target visualization, footer stats, result QR codes, and optional **game plugins**.

**Stack:** Go 1.24 HTTP/WebSocket server + vanilla JS frontend.

**License:** [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0).

---

## Quick start

### Prerequisites

- Go 1.24+ ([go.dev/dl](https://go.dev/dl/))
- DISAG OpticScore JSON Live on UDP port **30169** (default)

### Build & run

```bash
go build -o srdashboard.exe .
./srdashboard.exe
```

Custom site config:

```bash
./srdashboard.exe C:\path\to\config.xml
```

Open **http://localhost:8080** (master display). Shortcuts:

| URL | Role |
|-----|------|
| `/?display=master` | All ranges (default) |
| `/?display=shooter&range=2` or `/2` | Single-range tablet |
| `/config` | Settings UI |

Override the listen port with `PORT` (e.g. `PORT=9090 ./srdashboard.exe`).

### Cross-compile (e.g. Raspberry Pi)

```powershell
.\scripts\build-arm64.ps1
```

Pre-built binaries for each platform live in the `dist/` folder (e.g. `dist/windows-amd64/`, `dist/linux-amd64/`, `dist/linux-arm64/`). Non-developers can run those directly without installing Go.

---

## Architecture

```
DISAG OpticScore (UDP JSON)
        ↓
  udp/listener.go          ← parses Shot events, ShotDateTime
        ↓
  state/LiveState          ← per-range target + footer
        ↓
  host/rangestate          ← active plugin sessions / match
        ↓
  host/games/* + plugins/{id}  ← built-in game logic + views
        ↓
  api/hub (WebSocket) + /api/live (HTTP)
        ↓
  static/ (master.js, shooter.js, plugin views)
```

| Layer | Path | Role |
|-------|------|------|
| Site config | `config.xml` | Ranges, UDP port, footer toggles, active plugin |
| UDP | `udp/` | OpticScore JSON Live listener |
| Live state | `state/` | Shots, series sums, shooter names |
| Plugin host | `host/loader`, `host/rangestate`, `host/logicapi` | Load plugins, run matches, standings |
| Game logic | `host/games/` | Built-in Go scoring (`f1race`, `foxontherun`, `tannebaum`) |
| Plugins | `plugins/{id}/` | `manifest.xml`, `view.js`, theme, assets |
| Result QR | `qrformat/` | RingReader and related QR encodings |
| Frontend | `static/` | Master grid, shooter tablet, shared target core |

---

## Site config (`config.xml`)

| Key | Default | Description |
|-----|---------|-------------|
| `udpPort` | 30169 | OpticScore JSON Live UDP port (1–65535) |
| `ranges` | 6 | Number of shooting ranges (1–256) |
| `layoutColumns` | 4 | Panels per row on master display |
| `odbcName` | — | ODBC DSN for historic DB (**not wired in UI yet**) |
| `footer/*` | mostly `true` | Footer stat visibility toggles |
| `plugins/@dir` | `plugins` | Plugin root directory |
| `plugins/@active` | `classic-range` | Always-on active plugin id |
| `display/defaultMode` | `master` | Default display mode |
| `display/shotStrokeWidth` | — | Pellet outline width in mm (SVG units) |
| `display/controlToken` | — | See [Access control](#access-control) |

Master UI includes a **Settings** panel (`/config`) for site + per-plugin overrides. Target faces and discipline mapping live in each plugin’s config (e.g. `plugins/classic-range/config.xml`), not in global `config.xml`.

---

## Access control

Every state-changing endpoint (config saves, plugin install/activate/reload, race control, live reset and replace) is gated on the `display/controlToken` value.

**When `controlToken` is empty the server is open** — anyone who can reach port 8080 can change plugins, edit the config and reset live scores. That is the default and is fine on an isolated range network; the server logs a warning at startup so it is never a silent condition. Set a token for anything reachable beyond the range LAN.

The token is **write-only over HTTP**. `GET /api/config` reports only `controlTokenSet: true|false`, never the value, so a display cannot hand the credential to whoever asks it. Each device stores its own copy in `localStorage`:

- Enter it on the master display via the **Control-Token** button in the menu, or
- let any control action prompt for it — a `403` triggers a prompt and retries once.

To change the token, save the config with a new `controlToken` value. Omitting the field on `PUT /api/config` keeps the stored token; sending an explicit empty string clears it.

WebSocket upgrades are restricted to same-origin requests, so other sites cannot subscribe to live range data from a browser that has the dashboard open.

---

## Display modes & API

| URL | Role |
|-----|------|
| `/?display=master` | All ranges, plugin control, live status, settings |
| `/?display=shooter&range=N` or `/N` | Single-range tablet UI |
| `/config` | Site + plugin settings |

Endpoints marked 🔒 require the `X-SR-Control-Token` header when a token is configured.

| Endpoint | Purpose |
|----------|---------|
| `GET /api/live` | Live range state (JSON) |
| 🔒 `PUT /api/live` | Replace range state (restore after restart) |
| 🔒 `POST /api/live/reset?range=N` | Clear one range to defaults |
| `GET /api/config` | Site config (never includes the control token) |
| 🔒 `PUT /api/config` | Save site config |
| `GET /api/historic` | Historic ODBC status (stub) |
| `GET /api/qr/formats` | Available result QR formats |
| `GET /api/qr?range=N&fmt=rr` | Result QR metadata / payload |
| `GET /api/qr.png?range=N&fmt=rr` | Result QR as PNG |
| `GET /api/plugins`, `/api/plugins/active` | Installed / active plugins |
| `GET /api/plugins/session?range=N` | Plugin session + viewModel for range |
| `GET`/🔒 `PUT /api/plugins/{id}/config` | Per-plugin overrides |
| 🔒 `DELETE /api/plugins/{id}` | Uninstall a plugin |
| 🔒 `POST /api/plugins/control` | Start/stop plugin on all ranges |
| 🔒 `POST /api/plugins/install` | Upload a `.srplugin.zip` (max 64 MiB) |
| 🔒 `POST /api/plugins/activate`, `/reload`, `/scan-inbox` | Plugin lifecycle |
| `GET /ws?range=N` | WebSocket: `live`, `plugin_session`, `match` (same-origin only) |
| `/plugins/{id}/view.js` | Plugin frontend |
| `/assets/` | Target SVG and static assets |

---

## Plugins

### Bundled

| ID | Label | Mode | Status |
|----|-------|------|--------|
| `classic-range` | Classic Range View | Solo display | **Stable** — target, footer, last-10 chart, result QR |
| `f1-race` | F1 Race | Shared game | **In development** — circuits, pits, DRS; logic in `host/games/f1race/` |
| `fox-on-the-run` | Fox on the Run | Shared game | **In development** — Fuchsjagd with calibration, equalizer, terrain; `host/games/foxontherun/` |
| `tannebaum-einzel` | Tannebaum Einzel | Shared game | **In development** — per-stand Kegel tree (stages A/B/C); `host/games/tannebaum/` |
| `tannebaum-team` | Tannebaum Team | Shared game | **In development** — two-team tree race; same logic package |

Game plugins register via `loader.RegisterBuiltin` (blank-imported from `main.go`). Target-registry / face assets are shipped per plugin so distribution zips stay self-contained.

### Layout

```
plugins/{id}/
  manifest.xml       ← id, label, mode, kind, default config
  config.xml         ← site overrides (not in distribution zips)
  config-schema.json ← optional settings schema for the UI
  view.js            ← browser UI (SRPluginViews.{id})
  theme.css          ← optional
  assets/            ← optional images/SVG
```

Server scoring for bundled games lives under `host/games/`, not under `plugins/*/logic/`. Third-party plugins may still ship `logic.wasm` (see `host/loader`).

---

## Frontend building blocks

| File | Role |
|------|------|
| `static/app.js` | Display mode routing (`master` / `shooter` / `config`) |
| `static/target-core.js` | Shared target SVG, shot plotting, result QR UI |
| `static/master.js` | Range grid, plugin control |
| `static/shooter.js` | Tablet view per range |
| `static/plugin-shell.js` | Loads plugin `view.js` + theme (same-origin `/plugins/{id}/` only) |
| `static/config-editor.js` | Site + plugin settings forms |
| `static/config-page.js` | Standalone `/config` page |
| `static/auth.js` | Stores the control token per device, prompts on `403` |

---

## Project layout

What end users need is the binary, `config.xml`, and `plugins/` (as in `dist/{platform}/`). Source layout of the runtime:

```
srdashboard/
  main.go                 Entry: HTTP, UDP, plugin wiring
  config.xml              Site config
  config/                 XML load/save, plugin config merge
  api/                    REST + WebSocket hub + QR
  udp/                    OpticScore listener
  state/                  Live range state, shot parsing
  qrformat/               Result QR encoders (e.g. RingReader)
  host/
    loader/               Plugin manifests, builtins, WASM
    rangestate/           Sessions, matches, standings
    logicapi/             Plugin interfaces
    games/                Built-in Go game logic
      f1race/
      foxontherun/
      tannebaum/
  plugins/                One folder per plugin (views + assets)
  static/                 Web UI + assets/ target SVG
  LICENSE                 AGPL-3.0
```

---

## Known limitations

| Area | State |
|------|--------|
| Historic view / ODBC | DSN in config; `GET /api/historic` stub; UI/queries not built |
| WASM plugins | Loader supports `game.wasm`; bundled games use Go builtins; no execution timeout yet |
| Changing `ranges` | Requires a restart; the API reports it in `restartFields` |
| Transport security | Plain HTTP on all interfaces; put it behind a reverse proxy for TLS |
| Shared games | F1 / Fox / Tannebaum are playable but still evolving (balance, UX, edge cases) |

---

## Tests

```bash
go test ./...
```

The race detector needs a C toolchain on Windows; on a machine that has one, run the concurrency-sensitive packages with it:

```bash
CGO_ENABLED=1 go test -race ./state/... ./host/... ./api/...
```

---

## License

Copyright (C) 2026 the SRDashboard authors.

By contributing to this project, you agree that your contributions are licensed under AGPL-3.0 and that all rights in those contributions are granted to the SRDashboard project. See the **Contributions** section at the top of [LICENSE](LICENSE).

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the [GNU Affero General Public License](LICENSE) for more details.
