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

Host code lives under `host/` (`loader/`, `rangestate/`, `logicapi/`).  
Shipped plugins go out as folders in each release tree (`dist/<platform>/plugins/{id}/`). Extra / third-party plugins are added the same way: place a folder under `plugins/{id}/` and reload (or restart) the host.

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

## Extra plugin zip (optional)

Shipped plugins are not zipped. For a plugin that is not part of the default set, you can pack a single folder:

```bash
go run ./cmd/zip-plugin plugins/my-extra-plugin
```

That writes `{id}-{version}.srplugin.zip` (excludes `config.xml` and `logic/`). Drop the unpacked folder under `plugins/`, or install the zip through the host loader.

## Load / activate

- Shipped and custom plugins live as folders under `plugins/{id}/`
- Startup and `POST /api/plugins/reload` rescan that directory (dynamic load; no web upload)
- Activate via the master selector or `POST /api/plugins/activate` with `{"id":"…"}`

## Runtime URLs

- Assets: `/plugins/{id}/view.js`, `/plugins/{id}/assets/…`
- API: `GET /api/plugins/active`, `GET /api/plugins/session`, `POST /api/plugins/control`
