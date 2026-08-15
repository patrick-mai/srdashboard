(function () {
  const core = window.SRCore;
  if (!core) return;

  let activePlugin = null;
  let installedPlugins = [];
  let pluginSessions = {};
  let activeMeta = { active: false, pluginId: '' };
  const WS_BASE_RECONNECT_MS = 1000;
  const WS_MAX_RECONNECT_MS = 30000;
  let ws = null;
  let wsOpen = false;
  let reconnectTimer = null;
  let reconnectDelay = WS_BASE_RECONNECT_MS;
  let pollTimers = [];
  let stopped = false;
  /** Bumps on every activate / full remount so late async mounts cannot resurrect a prior plugin. */
  let mountGen = 0;

  function controlFetch(url, options) {
    return window.SRAuth.fetchWithAuth(url, options);
  }

  function currentPluginId() {
    return (activePlugin && activePlugin.id) || activeMeta.pluginId || '';
  }

  function isSharedPlugin() {
    const id = currentPluginId();
    if (!id) return false;
    if (activePlugin && activePlugin.id === id) {
      return activePlugin.mode === 'shared';
    }
    const installed = installedPlugins.find(function (p) { return p.id === id; });
    if (installed) return installed.mode === 'shared';
    return id === 'autorennen' || id === 'fox-on-the-run' ||
      id === 'zehner-bingo' || id === 'barrikade' || id === 'ludo';
  }

  function teardownSharedHost() {
    const host = document.getElementById('shared-master-host');
    if (host) {
      host.innerHTML = '';
      host.hidden = true;
      delete host._sharedReady;
      delete host._arLastVM;
      host.removeAttribute('style');
      if (host.parentNode) host.parentNode.removeChild(host);
    }
    const grid = document.getElementById('ranges-grid');
    if (grid) {
      grid.hidden = false;
      grid.style.display = '';
    }
  }

  /** Shared host is ready for in-place live updates (any shared plugin, not Autorennen-only). */
  function sharedHostReady(host) {
    return !!(host && !host.hidden && (host._sharedReady || host._arLastVM));
  }

  function clearRangePluginMounts() {
    const grid = document.getElementById('ranges-grid');
    if (!grid) return;
    grid.querySelectorAll('.range-plugin-view').forEach(function (mount) {
      mount.innerHTML = '';
      delete mount.dataset.pluginId;
      mount.className = 'range-plugin-view';
    });
  }

  async function loadTargetRegistry(assetsBase) {
    // assetsBase is /plugins/<id>/assets — registry lives one level up.
    const root = (assetsBase || '/plugins/classic-range/assets')
      .replace(/\/assets\/?$/, '/')
      .replace(/\/?$/, '/');
    await new Promise(function (resolve, reject) {
      const script = document.createElement('script');
      script.src = root + 'target-registry.js?t=' + Date.now();
      script.onload = function () {
        if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = 'classic-range';
        resolve();
      };
      script.onerror = reject;
      document.head.appendChild(script);
    }).catch(function () {});
  }

  async function fetchActivePlugin() {
    const res = await fetch('/api/plugins/active');
    if (!res.ok) return;
    const list = await res.json();
    activePlugin = (list && list[0]) || null;
    if (activePlugin) {
      activeMeta = { active: true, pluginId: activePlugin.id || '' };
    }
    if (activePlugin && activePlugin.id === 'classic-range') {
      await loadTargetRegistry(activePlugin.assetsBase);
      if (activePlugin.assetsBase && core.setTargetAssetBase) {
        core.setTargetAssetBase(activePlugin.assetsBase);
      }
    }
    if (activePlugin && activePlugin.defaults && core.setPluginTargetConfig) {
      core.setPluginTargetConfig(activePlugin.defaults);
    }
    if (window.SRPluginShell && window.SRPluginShell.unloadOtherThemes) {
      window.SRPluginShell.unloadOtherThemes(activePlugin && activePlugin.id);
    }
  }

  async function fetchInstalledPlugins() {
    const res = await fetch('/api/plugins');
    if (res.ok) installedPlugins = await res.json();
  }

  async function fetchSessions() {
    const res = await fetch('/api/plugins/session');
    if (!res.ok) return;
    const data = await res.json();
    pluginSessions = {};
    (data.sessions || []).forEach(function (s) { pluginSessions[s.rangeNum] = s; });
    if (data.active) activeMeta = data.active;
    else if (data.match) activeMeta = { active: !!data.match.active, pluginId: data.match.pluginId || '' };
  }

  function statusText() {
    if (activePlugin) {
      return activePlugin.label + ' (' + activePlugin.id + ')';
    }
    if (activeMeta.pluginId) return activeMeta.pluginId;
    return 'Kein Plugin';
  }

  function updateStatus() {
    const text = statusText();
    ['match-status-strip', 'match-status-compact'].forEach(function (id) {
      const el = document.getElementById(id);
      if (!el) return;
      el.innerHTML = '<span class="status-active">' + escapeHtml(text) + '</span>';
    });
  }

  function escapeHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  /** Pending live ranges to paint on the next animation frame (avoids WS await backlog). */
  const pendingLive = {};
  let rafPaint = 0;

  function queueLiveRange(range) {
    if (!range || range.rangeNum == null) return;
    pendingLive[range.rangeNum] = range;
    if (rafPaint) return;
    rafPaint = requestAnimationFrame(function () {
      rafPaint = 0;
      const nums = Object.keys(pendingLive);
      const batch = [];
      for (let i = 0; i < nums.length; i++) {
        const n = parseInt(nums[i], 10);
        const r = pendingLive[n];
        delete pendingLive[n];
        batch.push(r);
      }
      // Unhide newly active stands before paint so target layout gets real box size.
      if (typeof core.syncRangeVisibility === 'function') {
        core.syncRangeVisibility(core.lastLiveData);
      }
      for (let i = 0; i < batch.length; i++) {
        paintLiveRange(batch[i]);
      }
    });
  }

  /** In-place classic update — no plugin remount / no script reload. */
  function paintLiveRange(range) {
    if (!range) return;
    core.updatePluginPanelHeader(range.rangeNum, range);
    // Shared plugins own the stage; never paint into hidden per-range mounts.
    if (isSharedPlugin()) return;

    const panel = document.querySelector('.range-panel[data-range="' + range.rangeNum + '"]');
    if (!panel) return;
    const mount = panel.querySelector('.range-plugin-view');
    if (!mount) return;

    const id = currentPluginId();
    if (id === 'classic-range' && typeof core.renderClassicRangeView === 'function') {
      if (activePlugin && activePlugin.assetsBase && core.setTargetAssetBase) {
        core.setTargetAssetBase(activePlugin.assetsBase);
      }
      mount.className = 'range-plugin-view classic-range-view';
      mount.dataset.range = String(range.rangeNum);
      core.renderClassicRangeView(mount, range);
      return;
    }
    // Per-range non-classic plugins: remount (async, fire-and-forget).
    if (id) mountRangePlugin(range.rangeNum, mountGen);
  }

  async function mountRangePlugin(rangeNum, gen) {
    if (!activePlugin || !window.SRPluginShell) return;
    if (isSharedPlugin()) return;
    if (gen != null && gen !== mountGen) return;
    const grid = document.getElementById('ranges-grid');
    if (!grid || grid.hidden) return;
    const panel = grid.querySelector('.range-panel[data-range="' + rangeNum + '"]');
    if (!panel) return;
    const mount = panel.querySelector('.range-plugin-view');
    if (!mount) return;

    const session = pluginSessions[rangeNum] || {};
    const live = (core.lastLiveData && core.lastLiveData.ranges || []).find(function (r) {
      return r.rangeNum === rangeNum;
    });
    const viewModel = Object.assign({}, session.viewModel || {}, {
      rangeNum: rangeNum,
      // Prefer live range data — session viewModel.range can be stale or PascalCase from Go.
      range: live || (session.viewModel && session.viewModel.range) || null,
      events: session.events || []
    });

    await window.SRPluginShell.renderPluginView(
      mount,
      activePlugin.id,
      activePlugin.viewUrl,
      activePlugin.assetsBase,
      viewModel,
      activePlugin.themeUrl || ''
    );
    if (gen != null && gen !== mountGen) {
      mount.innerHTML = '';
      delete mount.dataset.pluginId;
    }
  }

  async function mountAllPluginViews() {
    const gen = ++mountGen;
    const n = (core.config && core.config.ranges) || 1;
    const shared = isSharedPlugin();

    if (shared) {
      clearRangePluginMounts();
      const grid = document.getElementById('ranges-grid');
      if (!grid || !activePlugin) return;
      let host = document.getElementById('shared-master-host');
      if (!host) {
        host = document.createElement('div');
        host.id = 'shared-master-host';
        host.className = 'shared-master-host';
        grid.parentNode.insertBefore(host, grid);
      }
      grid.hidden = true;
      grid.style.display = 'none';
      host.hidden = false;
      host.style.cssText = 'position:absolute;inset:0;z-index:2;';
      const stage = document.getElementById('stage');
      if (stage) stage.style.position = 'relative';
      const session = pluginSessions[1] || Object.values(pluginSessions)[0] || {};
      const viewModel = buildSharedViewModel(session);
      await window.SRPluginShell.renderPluginView(
        host,
        activePlugin.id,
        activePlugin.viewUrl,
        activePlugin.assetsBase,
        viewModel,
        activePlugin.themeUrl || ''
      );
      // Stale mount must NOT tear down a newer host — that unhides #ranges-grid
      // under/through the absolute fox/Autorennen surface (meadow + Scheibe ghosting).
      if (gen !== mountGen) return;
      host._sharedReady = true;
      return;
    }

    teardownSharedHost();
    if (gen !== mountGen) return;

    core.ensurePluginPanels(n);
    const tasks = [];
    for (let i = 1; i <= n; i++) tasks.push(mountRangePlugin(i, gen));
    await Promise.all(tasks);
  }

  function buildSharedViewModel(session) {
    // rangeNum 0 = master overview. Plugins that key off rangeNum (fox, tannebaum)
    // must not see a lane id here or they paint the shooter layout on the hall screen.
    const viewModel = Object.assign({}, (session && session.viewModel) || {}, {
      rangeNum: 0,
      events: (session && session.events) || []
    });
    if (viewModel.race && viewModel.race.cars && core.lastLiveData) {
      const lives = core.lastLiveData.ranges || [];
      viewModel.race.cars = viewModel.race.cars.map(function (c) {
        const live = lives.find(function (r) { return r.rangeNum === c.rangeNum; });
        if (!live) return c;
        const copy = Object.assign({}, c);
        if (live.shooterName) copy.shooterName = live.shooterName;
        if (live.currentValue != null && live.currentValue > 0) copy.lastShotValue = live.currentValue;
        return copy;
      });
    }
    if (viewModel.hunt && viewModel.hunt.players && core.lastLiveData) {
      const lives = core.lastLiveData.ranges || [];
      viewModel.hunt.players = viewModel.hunt.players.map(function (p) {
        const live = lives.find(function (r) { return r.rangeNum === p.rangeNum; });
        if (!live) return p;
        const copy = Object.assign({}, p);
        if (live.shooterName) copy.shooterName = live.shooterName;
        if (live.discipline) copy.discipline = live.discipline;
        if (live.discType) copy.discipline = copy.discipline || live.discType;
        return copy;
      });
    }
    return viewModel;
  }

  /** In-place shared update — never tear down the track SVG. */
  function updateSharedPluginView(session) {
    if (!activePlugin || !window.SRPluginShell) return;
    const host = document.getElementById('shared-master-host');
    if (!host || host.hidden) {
      mountAllPluginViews();
      return;
    }
    const viewModel = buildSharedViewModel(session || {});
    window.SRPluginShell.renderPluginView(
      host,
      activePlugin.id,
      activePlugin.viewUrl,
      activePlugin.assetsBase,
      viewModel,
      activePlugin.themeUrl || ''
    );
  }

  function scheduleReconnect() {
    if (stopped || reconnectTimer) return;
    // Back off so a server that stays down is not hit every 3s forever.
    reconnectDelay = Math.min(reconnectDelay * 2, WS_MAX_RECONNECT_MS);
    reconnectTimer = setTimeout(connectWS, reconnectDelay);
  }

  function connectWS() {
    if (stopped) return;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (ws) {
      try {
        ws.onclose = null;
        ws.onmessage = null;
        ws.onopen = null;
        ws.onerror = null;
        ws.close();
      } catch (_) {}
      ws = null;
    }
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(proto + '//' + location.host + '/ws');
    ws.onopen = function () {
      wsOpen = true;
      reconnectDelay = WS_BASE_RECONNECT_MS;
    };
    ws.onerror = function (ev) {
      console.warn('websocket error', ev);
    };
    ws.onmessage = function (ev) {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === 'plugin_session' && msg.session) {
          pluginSessions[msg.session.rangeNum] = msg.session;
          if (isSharedPlugin()) {
            updateSharedPluginView(msg.session);
          } else if (currentPluginId() !== 'classic-range') {
            mountRangePlugin(msg.session.rangeNum, mountGen);
          }
        }
        if (msg.type === 'active_plugin' && msg.active) {
          activeMeta = msg.active;
          if (activePlugin && activeMeta.pluginId && activePlugin.id !== activeMeta.pluginId) {
            // Stale until refreshAll finishes — prefer activeMeta for routing.
            activePlugin = null;
            teardownSharedHost();
            refreshControls();
          }
          updateStatus();
        }
        if (msg.type === 'match' && msg.match) {
          activeMeta = { active: !!msg.match.active, pluginId: msg.match.pluginId || '' };
          if (activePlugin && activeMeta.pluginId && activePlugin.id !== activeMeta.pluginId) {
            activePlugin = null;
            teardownSharedHost();
            refreshControls();
          }
          updateStatus();
        }
        if (msg.type === 'live' && msg.range) {
          const live = core.lastLiveData || { ranges: [] };
          const ranges = (live.ranges || []).slice();
          const idx = ranges.findIndex(function (r) { return r.rangeNum === msg.range.rangeNum; });
          if (idx >= 0) ranges[idx] = msg.range;
          else ranges.push(msg.range);
          core.lastLiveData = { ranges: ranges };
          core.bumpLiveGen();
          if (isSharedPlugin()) {
            // Live scores update in place; remounting every shot races mountGen
            // and can briefly expose the classic range grid under the shared host.
            const host = document.getElementById('shared-master-host');
            if (sharedHostReady(host)) {
              updateSharedPluginView(pluginSessions[1] || Object.values(pluginSessions)[0] || {});
            } else {
              mountAllPluginViews();
            }
          } else {
            queueLiveRange(msg.range);
          }
        }
        if (msg.type === 'plugins_changed' || msg.type === 'config_changed') {
          refreshAll();
        }
      } catch (e) {
        console.warn('ws message', e);
      }
    };
    ws.onclose = function () {
      wsOpen = false;
      scheduleReconnect();
    };
  }

  async function activatePlugin(id) {
    // Optimistic isolation: drop prior plugin UI before the round-trip returns.
    mountGen += 1;
    activeMeta = { active: true, pluginId: id || '' };
    if (activePlugin && activePlugin.id !== id) activePlugin = null;
    teardownSharedHost();
    clearRangePluginMounts();
    refreshControls();
    updateStatus();

    const res = await controlFetch('/api/plugins/activate', {
      method: 'POST',
      body: JSON.stringify({ id: id })
    });
    if (!res.ok) {
      const t = await res.text();
      alert('Activate failed: ' + t);
      return;
    }
    if (window.SRPluginShell) window.SRPluginShell.clearCache();
    await refreshAll();
  }

  function selectedResetRanges() {
    const list = document.getElementById('range-reset-select');
    const out = [];
    if (!list) return out;
    const boxes = list.querySelectorAll('input[data-range]');
    for (let i = 0; i < boxes.length; i++) {
      if (!boxes[i].checked) continue;
      const n = parseInt(boxes[i].getAttribute('data-range'), 10);
      if (n >= 1) out.push(n);
    }
    return out;
  }

  function formatBahnList(nums) {
    if (!nums.length) return '';
    if (nums.length === 1) return 'Bahn ' + nums[0];
    const parts = [];
    let start = nums[0];
    let prev = nums[0];
    for (let i = 1; i <= nums.length; i++) {
      const cur = nums[i];
      if (cur === prev + 1) {
        prev = cur;
        continue;
      }
      parts.push(start === prev ? String(start) : start + '–' + prev);
      start = prev = cur;
    }
    return 'Bahnen ' + parts.join(', ');
  }

  function clearRangeResetSelection() {
    const list = document.getElementById('range-reset-select');
    if (!list) return;
    const boxes = list.querySelectorAll('input[data-range]');
    for (let i = 0; i < boxes.length; i++) boxes[i].checked = false;
  }

  function setSideMenuOpen(open) {
    const toggle = document.getElementById('menu-toggle');
    const menu = document.getElementById('top-bar-menu');
    const bar = document.getElementById('top-bar');
    if (!menu) return;
    menu.hidden = !open;
    if (toggle) toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (bar) bar.classList.toggle('is-menu-open', !!open);
  }

  function fillRangeResetSelect() {
    const list = document.getElementById('range-reset-select');
    if (!list) return;
    const n = Math.max(1, (core.config && core.config.ranges) || 1);
    const prev = selectedResetRanges();
    const boxes = list.querySelectorAll('input[data-range]');
    if (boxes.length === n) return;
    let html = '';
    for (let i = 1; i <= n; i++) {
      html += '<label class="range-reset-item"><input type="checkbox" data-range="' + i + '"' +
        (prev.indexOf(i) >= 0 ? ' checked' : '') + '>Bahn ' + i + '</label>';
    }
    list.innerHTML = html;
  }

  function buildControls() {
    const strip = document.getElementById('control-strip');
    if (!strip) return;
    strip.innerHTML =
      '<div class="plugin-quick">' +
      '<label class="plugin-active-label">Aktiv' +
      '<select id="plugin-active-select"></select></label>' +
      '<a class="btn btn-ghost" id="plugin-config-link" href="/config">Einstellungen</a>' +
      '<button type="button" class="btn btn-ghost" id="rulebook-menu-btn" hidden>Regelbuch</button>' +
      '<span id="race-controls" class="race-controls" hidden>' +
      '<button type="button" class="btn btn-primary" id="race-start-btn">Start</button>' +
      '<button type="button" class="btn" id="race-reset-btn">Reset</button>' +
      '<button type="button" class="btn" id="race-puncture-btn" hidden>Reifenplatzer</button>' +
      '<button type="button" class="btn" id="race-oil-btn" hidden>Ölverlust</button>' +
      '</span>' +
      '<div class="plugin-active-label">Bahnen zurücksetzen' +
      '<div id="range-reset-select" class="range-reset-list" role="group" aria-label="Bahnen"></div></div>' +
      '<button type="button" class="btn btn-danger" id="btn-range-reset">Bahnen zurücksetzen</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-theme-toggle">Dunkelmodus</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-fullscreen-toggle">Vollbild</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-control-token">Control-Token</button>' +
      '<span class="tablet-hint">Tablet: /BahnNr z.B. ' + location.origin + '/3</span>' +
      '</div>';

    const sel = document.getElementById('plugin-active-select');
    if (sel) {
      sel.onchange = function () {
        if (sel.value) activatePlugin(sel.value);
      };
    }
    async function raceControl(action, type) {
      const body = { action: action };
      if (type) body.type = type;
      const res = await controlFetch('/api/plugins/control', {
        method: 'POST',
        body: JSON.stringify(body)
      });
      if (!res.ok) alert(await res.text());
    }
    document.getElementById('race-start-btn').onclick = function () { raceControl('start'); };
    document.getElementById('race-reset-btn').onclick = function () { raceControl('reset'); };
    document.getElementById('race-puncture-btn').onclick = function () { raceControl('field_event', 'puncture'); };
    document.getElementById('race-oil-btn').onclick = function () { raceControl('field_event', 'oil_leak'); };
    const rangeResetBtn = document.getElementById('btn-range-reset');
    if (rangeResetBtn) {
      rangeResetBtn.onclick = async function () {
        const nums = selectedResetRanges();
        if (!nums.length) {
          alert('Bitte mindestens eine Bahn wählen.');
          return;
        }
        const label = formatBahnList(nums);
        if (!confirm(label + ' wirklich zurücksetzen? Schütze, Schüsse und Summen werden gelöscht.')) {
          return;
        }
        const failed = [];
        for (let i = 0; i < nums.length; i++) {
          const n = nums[i];
          const res = await controlFetch('/api/live/reset?range=' + encodeURIComponent(n), {
            method: 'POST',
            body: '{}'
          });
          if (!res.ok) failed.push(n);
        }
        if (failed.length) {
          alert('Zurücksetzen fehlgeschlagen: ' + formatBahnList(failed));
        }
        clearRangeResetSelection();
        setSideMenuOpen(false);
      };
    }
    const tokenBtn = document.getElementById('btn-control-token');
    if (tokenBtn) {
      function syncTokenButton() {
        const set = !!window.SRAuth.get();
        tokenBtn.textContent = set ? 'Control-Token ✓' : 'Control-Token';
        tokenBtn.title = set
          ? 'Token ist auf diesem Gerät hinterlegt — klicken zum Ändern'
          : 'Token für Steuerbefehle hinterlegen';
      }
      syncTokenButton();
      tokenBtn.onclick = function () {
        window.SRAuth.promptForToken('Control-Token für dieses Gerät (leer = löschen):');
        syncTokenButton();
      };
    }
    const themeBtn = document.getElementById('btn-theme-toggle');
    if (themeBtn && window.SRTheme) {
      function syncThemeButton() {
        const dark = window.SRTheme.get() === 'dark';
        themeBtn.textContent = dark ? 'Hellmodus' : 'Dunkelmodus';
        themeBtn.setAttribute('aria-pressed', dark ? 'true' : 'false');
        themeBtn.title = dark ? 'Zum hellen Modus wechseln' : 'Zum dunklen Modus wechseln';
      }
      syncThemeButton();
      themeBtn.onclick = function () {
        window.SRTheme.toggle();
        syncThemeButton();
      };
      document.addEventListener('srdashboard:themechange', syncThemeButton);
    }
    const fsBtn = document.getElementById('btn-fullscreen-toggle');
    if (fsBtn) {
      function syncFullscreenButton() {
        const on = isFullscreen();
        fsBtn.textContent = on ? 'Vollbild beenden' : 'Vollbild';
        fsBtn.setAttribute('aria-pressed', on ? 'true' : 'false');
      }
      syncFullscreenButton();
      fsBtn.onclick = function () {
        toggleFullscreen().then(syncFullscreenButton).catch(function () {
          syncFullscreenButton();
        });
      };
      document.addEventListener('fullscreenchange', syncFullscreenButton);
      document.addEventListener('webkitfullscreenchange', syncFullscreenButton);
    }
    refreshControls();
  }

  function refreshControls() {
    const sel = document.getElementById('plugin-active-select');
    if (!sel) return;
    const activeId = currentPluginId();
    sel.innerHTML = installedPlugins.map(function (p) {
      return '<option value="' + escapeHtml(p.id) + '"' +
        (p.id === activeId ? ' selected' : '') + '>' +
        escapeHtml(p.label || p.id) + ' (' + escapeHtml(p.kind || '') + ')</option>';
    }).join('');
    const race = document.getElementById('race-controls');
    if (race) {
      const shared = isSharedPlugin();
      const id = currentPluginId();
      race.hidden = !shared;
      const startBtn = document.getElementById('race-start-btn');
      const punctureBtn = document.getElementById('race-puncture-btn');
      const oilBtn = document.getElementById('race-oil-btn');
      if (id === 'autorennen') {
        if (startBtn) startBtn.textContent = 'Rennen starten';
        if (punctureBtn) punctureBtn.hidden = false;
        if (oilBtn) oilBtn.hidden = false;
      } else if (id === 'fox-on-the-run') {
        if (startBtn) startBtn.textContent = 'Jagd starten';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      } else if (id === 'tannebaum-einzel' || id === 'tannebaum-team') {
        if (startBtn) startBtn.textContent = 'Tannebaum starten';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      } else if (id === 'zehner-bingo') {
        if (startBtn) startBtn.textContent = 'Bingo starten';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      } else if (id === 'barrikade') {
        if (startBtn) startBtn.textContent = 'Barrikade starten';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      } else if (id === 'ludo') {
        if (startBtn) startBtn.textContent = 'Ludo starten';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      } else {
        if (startBtn) startBtn.textContent = 'Start';
        if (punctureBtn) punctureBtn.hidden = true;
        if (oilBtn) oilBtn.hidden = true;
      }
    }
    fillRangeResetSelect();
    syncRulebookButtons();
  }

  function syncRulebookButtons() {
    const rb = window.SRRulebook;
    if (!rb) return;
    const slim = document.getElementById('rulebook-btn');
    const menuBtn = document.getElementById('rulebook-menu-btn');
    rb.wireButton(slim);
    rb.wireButton(menuBtn);
    rb.syncButton(slim, activePlugin);
    rb.syncButton(menuBtn, activePlugin);
  }

  async function refreshAll() {
    await core.fetchConfig();
    await fetchActivePlugin();
    await fetchInstalledPlugins();
    await fetchSessions();
    updateStatus();
    refreshControls();
    core.applyLayout();
    const live = await core.fetchLive();
    if (live) core.render(live);
    else core.ensurePluginPanels((core.config && core.config.ranges) || 1);
    await mountAllPluginViews();
  }

  async function pollFallback() {
    const gen = core.getLiveGen();
    await fetchSessions();
    const live = await core.fetchLive();
    if (core.getLiveGen() !== gen) return; // WS already newer
    if (live) {
      core.lastLiveData = live;
      if (isSharedPlugin()) {
        // Prefer in-place update — full remount every SAFETY_POLL_MS flickered.
        updateSharedPluginView(pluginSessions[1] || Object.values(pluginSessions)[0] || {});
      } else {
        (live.ranges || []).forEach(function (r) { queueLiveRange(r); });
      }
    } else if (!wsOpen) {
      if (isSharedPlugin()) {
        const host = document.getElementById('shared-master-host');
        if (host && !host.hidden) {
          updateSharedPluginView(pluginSessions[1] || Object.values(pluginSessions)[0] || {});
          return;
        }
      }
      await mountAllPluginViews();
    }
  }

  function isFullscreen() {
    return !!(document.fullscreenElement || document.webkitFullscreenElement);
  }

  function toggleFullscreen() {
    if (isFullscreen()) {
      const exit = document.exitFullscreen || document.webkitExitFullscreen;
      return exit ? exit.call(document) : Promise.resolve();
    }
    const el = document.documentElement;
    const req = el.requestFullscreen || el.webkitRequestFullscreen;
    return req ? req.call(el) : Promise.resolve();
  }

  function wireMenu() {
    const toggle = document.getElementById('menu-toggle');
    const menu = document.getElementById('top-bar-menu');
    if (toggle && menu) {
      toggle.onclick = function () {
        setSideMenuOpen(menu.hidden);
      };
    }
    const stopSlim = document.getElementById('plugin-stop-slim');
    if (stopSlim) stopSlim.hidden = true;
  }

  function refreshThemeDependentViews() {
    if (core.lastLiveData) core.render(core.lastLiveData);
  }

  function teardown() {
    stopped = true;
    pollTimers.forEach(clearInterval);
    pollTimers = [];
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (ws) {
      try {
        ws.onclose = null;
        ws.close();
      } catch (_) {}
      ws = null;
    }
  }

  async function init() {
    wireMenu();
    buildControls();
    document.addEventListener('srdashboard:themechange', refreshThemeDependentViews);
    window.addEventListener('pagehide', teardown);
    await refreshAll();
    connectWS();
    pollTimers.push(setInterval(function () {
      if (!wsOpen) pollFallback();
    }, 1000));
    pollTimers.push(setInterval(function () {
      if (wsOpen) pollFallback();
    }, core.SAFETY_POLL_MS || 20000));
  }

  init().catch(function (e) {
    console.error('master init failed', e);
  });
})();
