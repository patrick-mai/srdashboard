# SRDashboard

Live shooting-range **display** for DISAG OpticScore. Shows configurable ranges with target visualization, footer stats, and optional **game plugins**.

**Stack:** Go 1.24 HTTP/WebSocket server + vanilla JS frontend.

**License:** [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0).

---

## Quick start

### Prerequisites

- Go 1.21+ ([go.dev/dl](https://go.dev/dl/))
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

Open **http://localhost:8080** (master display). For a single-range tablet: `http://localhost:8080/?display=shooter&range=2`.

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
  plugins/{id}             ← display or game plugins
        ↓
  api/hub (WebSocket) + /api/live (HTTP)
        ↓
  static/ (master.js, shooter.js, plugin views)
```

| Layer | Path | Role |
|-------|------|------|
| Site config | `config.xml` | Ranges, UDP port, footer toggles |
| UDP | `udp/` | OpticScore JSON Live listener |
| Live state | `state/` | Shots, series sums, shooter names |
| Plugin host | `host/loader`, `host/rangestate`, `host/logicapi` | Load plugins, run matches, standings |
| Plugins | `plugins/{id}/` | `manifest.xml`, optional `logic/`, `view.js`, assets |
| Frontend | `static/` | Master grid, shooter tablet, target SVG in `static/assets/` |

---

## Site config (`config.xml`)

| Key | Default | Description |
|-----|---------|-------------|
| `udpPort` | 30169 | OpticScore JSON Live UDP port |
| `ranges` | 6 | Number of shooting ranges |
| `layoutColumns` | 3 | Panels per row on master display |
| `defaultTarget` | `10_m_Air_Rifle_target.svg` | Target SVG under `static/assets/` |
| `odbcName` | — | ODBC DSN for historic DB (**not wired in UI yet**) |
| `footer/*` | mostly `true` | Footer stat visibility toggles |
| `display/controlToken` | — | If set, master POSTs need header `X-SR-Control-Token` |
| `plugins/dir` | `plugins` | Plugin root directory |

Master UI includes a **Settings** panel (`config-editor.js`) for site + per-plugin overrides.

---

## Display modes & API

| URL | Role |
|-----|------|
| `/?display=master` | All ranges, plugin control, live status, settings |
| `/?display=shooter&range=N` | Single-range tablet UI |

| Endpoint | Purpose |
|----------|---------|
| `GET /api/live` | Live range state (JSON) |
| `GET /api/config` | Site config |
| `GET /api/historic` | Historic ODBC status (stub) |
| `GET /api/plugins`, `/api/plugins/active` | Installed / active plugins |
| `GET /api/plugins/session?range=N` | Plugin session + viewModel for range |
| `POST /api/plugins/control` | Start/stop plugin on all ranges |
| `GET /ws?range=N` | WebSocket: `live`, `plugin_session`, `match` |
| `/plugins/{id}/view.js` | Plugin frontend |
| `/assets/` | Target SVG and static assets |

---

## Plugins

### Bundled

| ID | Label | Mode | Status |
|----|-------|------|--------|
| `classic-range` | Classic Range View | Solo display | **Stable** — standard target, footer, last-10 chart |

### Layout

```
plugins/{id}/
  manifest.xml      ← id, label, default config
  config.xml        ← site overrides (not in distribution zips)
  logic/*.go        ← optional server scoring (compiled into host)
  view.js           ← browser UI (SRPluginViews.{id})
  theme.css         ← optional
  assets/           ← optional images/SVG
```

Distribution zips (view/assets only):

```bash
go run ./cmd/zip-bundled
```

---

## Frontend building blocks

| File | Role |
|------|------|
| `static/app.js` | Core live target rendering |
| `static/master.js` | Range grid, plugin control |
| `static/shooter.js` | Tablet view per range |
| `static/plugin-shell.js` | Loads plugin `view.js` + theme |
| `static/config-editor.js` | Site + plugin settings UI |

---

## Project layout

```
srdashboard/
  main.go                 Entry: HTTP, UDP, plugin wiring
  config.xml              Site config
  config/                 XML load/save, plugin config merge
  api/                    REST + WebSocket hub
  udp/                    OpticScore listener
  state/                  Live range state, shot parsing
  host/
    loader/               Plugin manifests, Go/WASM logic
    rangestate/           Sessions, matches, standings
    logicapi/             Plugin interfaces
  plugins/                One folder per plugin
  static/                 Web UI + assets/ target SVG
  cmd/
    replay-log/           Replay DISAG JSON log over UDP
    send-shot/            Send synthetic Shot UDP packets for testing
    zip-bundled/          Build .srplugin.zip distributions
  LICENSE                 AGPL-3.0
```

---

## Known limitations

| Area | State |
|------|--------|
| Historic view / ODBC | DSN in config; `GET /api/historic` stub; UI/queries not built |
| WASM plugins | Loader supports `game.wasm`; bundled plugins may use Go builtins |

---

## Tests

```bash
go test ./...
```

---

## License

Copyright (C) 2026 the SRDashboard authors.

By contributing to this project, you agree that your contributions are licensed under AGPL-3.0 and that all rights in those contributions are granted to the SRDashboard project. See the **Contributions** section at the top of [LICENSE](LICENSE).

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the [GNU Affero General Public License](LICENSE) for more details.
