# Plugin authoring

## Terminology

| Layer | Role |
|-------|------|
| **Display** | SRDashboard itself — live ranges, targets, footer stats |
| **Plugin** | Loaded from `plugins/{id}/` (selector on master) |
| **Logic** | Server scoring in `plugins/{id}/logic/` (bundled with the host) |
| **View** | Browser UI in `view.js` → `SRPluginViews` |

## Directory layout

Every plugin is **one folder** under `plugins/`:

```
plugins/
  classic-range/
    manifest.xml       ← identity + shipped default config
    config.xml         ← optional site overrides (not in zips)
    logic/             ← optional server scoring (Go, compiled into host)
    view.js
    theme.css          (optional)
    assets/            (optional)
```

Host code lives under `host/` (`loader/`, `rangestate/`, `logicapi/`). Upload zips go in `host/inbox/`.

Upload zips land in `host/inbox/`, then unpack into `plugins/{id}/`.  
Distribution zips are built to `dist/plugins/` via `go run ./cmd/zip-bundled`.

## manifest.xml

Required attributes on `<plugin>`: `id`, `label`, `version`. Optional: `minHostVersion`, `mode`, `kind`.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<plugin id="classic-range" label="Classic Range View" version="1.0.0" mode="per-range" kind="display">
  <description>Standard target, footer stats, and last-10 chart — the default range display.</description>
  <entrypoints view="view.js"/>
  <assetsDir>assets</assetsDir>
</plugin>
```

Third-party plugins without bundled Go logic may ship `logic.wasm` instead (see `host/loader` WASM support).

## config.xml (same folder)

Site-specific overrides, merged on top of manifest defaults:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<pluginConfig id="classic-range">
  <!-- plugin-specific keys -->
</pluginConfig>
```

## view.js contract

```javascript
window.SRPluginViews = window.SRPluginViews || {};
window.SRPluginViews.myplugin = function render(container, viewModel, assetsBaseUrl) {
  // Draw UI from server viewModel only
};
```

## Build a distribution zip

```bash
go run ./cmd/zip-bundled
```

Creates `dist/plugins/{id}-{version}.srplugin.zip` from each `plugins/{id}/` with a `manifest.xml` (excludes `config.xml` and `logic/`).

## Install

- Copy or edit plugin folders directly under `plugins/{id}/`
- Or drop a zip in `host/inbox/` and click **Scan inbox** / `POST /api/plugins/scan-inbox`
- API upload: `POST /api/plugins/install`

## Runtime URLs

- Assets: `/plugins/{id}/view.js`, `/plugins/{id}/assets/…`
- API: `GET /api/plugins/active`, `GET /api/plugins/session`, `POST /api/plugins/control`
