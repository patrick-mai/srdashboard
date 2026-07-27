(function () {
  const core = window.SRCore;
  if (!core) return;

  let controlToken = '';
  let activePlugin = null;
  let installedPlugins = [];
  let pluginSessions = {};
  let activeMeta = { active: false, pluginId: '' };
  let ws = null;
  let wsOpen = false;
  let reconnectTimer = null;

  function controlHeaders(extra) {
    const h = Object.assign({ 'Content-Type': 'application/json' }, extra || {});
    if (controlToken) h['X-SR-Control-Token'] = controlToken;
    return h;
  }

  async function fetchActivePlugin() {
    const res = await fetch('/api/plugins/active');
    if (!res.ok) return;
    const list = await res.json();
    activePlugin = (list && list[0]) || null;
    if (activePlugin && activePlugin.id === 'classic-range' && !window.SRTargetRegistry) {
      await new Promise(function (resolve, reject) {
        const script = document.createElement('script');
        script.src = '/plugins/classic-range/target-registry.js';
        script.onload = resolve;
        script.onerror = reject;
        document.head.appendChild(script);
      }).catch(function () {});
    }
    if (activePlugin && activePlugin.defaults && core.setPluginTargetConfig) {
      core.setPluginTargetConfig(activePlugin.defaults);
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
      for (let i = 0; i < nums.length; i++) {
        const n = parseInt(nums[i], 10);
        const r = pendingLive[n];
        delete pendingLive[n];
        paintLiveRange(r);
      }
    });
  }

  /** In-place classic update — no plugin remount / no script reload. */
  function paintLiveRange(range) {
    if (!range) return;
    core.updatePluginPanelHeader(range.rangeNum, range);
    const panel = document.querySelector('.range-panel[data-range="' + range.rangeNum + '"]');
    if (!panel) return;
    const mount = panel.querySelector('.range-plugin-view');
    if (!mount) return;

    const id = (activePlugin && activePlugin.id) || activeMeta.pluginId || '';
    if (id === 'classic-range' && typeof core.renderClassicRangeView === 'function') {
      if (activePlugin && activePlugin.assetsBase && core.setTargetAssetBase) {
        core.setTargetAssetBase(activePlugin.assetsBase);
      }
      mount.className = 'range-plugin-view classic-range-view';
      mount.dataset.range = String(range.rangeNum);
      core.renderClassicRangeView(mount, range);
      return;
    }
    // Non-classic plugins: remount (async, fire-and-forget).
    mountRangePlugin(range.rangeNum);
  }

  async function mountRangePlugin(rangeNum) {
    if (!activePlugin || !window.SRPluginShell) return;
    const grid = document.getElementById('ranges-grid');
    if (!grid) return;
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
      range: live || (session.viewModel && session.viewModel.range) || null
    });

    await window.SRPluginShell.renderPluginView(
      mount,
      activePlugin.id,
      activePlugin.viewUrl,
      activePlugin.assetsBase,
      viewModel,
      activePlugin.themeUrl || ''
    );
  }

  async function mountAllPluginViews() {
    const n = (core.config && core.config.ranges) || 1;
    core.ensurePluginPanels(n);
    const tasks = [];
    for (let i = 1; i <= n; i++) tasks.push(mountRangePlugin(i));
    await Promise.all(tasks);
  }

  function connectWS() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (ws) {
      try {
        ws.onclose = null;
        ws.onmessage = null;
        ws.onopen = null;
        ws.close();
      } catch (_) {}
      ws = null;
    }
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(proto + '//' + location.host + '/ws');
    ws.onopen = function () { wsOpen = true; };
    ws.onmessage = function (ev) {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === 'plugin_session' && msg.session) {
          pluginSessions[msg.session.rangeNum] = msg.session;
          // Display plugins already paint from live; skip remount to avoid poll-like lag.
          const id = (activePlugin && activePlugin.id) || activeMeta.pluginId || '';
          if (id !== 'classic-range') {
            mountRangePlugin(msg.session.rangeNum);
          }
        }
        if (msg.type === 'active_plugin' && msg.active) {
          activeMeta = msg.active;
          updateStatus();
        }
        if (msg.type === 'match' && msg.match) {
          activeMeta = { active: !!msg.match.active, pluginId: msg.match.pluginId || '' };
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
          queueLiveRange(msg.range);
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
      reconnectTimer = setTimeout(connectWS, 3000);
    };
  }

  async function activatePlugin(id) {
    const res = await fetch('/api/plugins/activate', {
      method: 'POST',
      headers: controlHeaders(),
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

  async function reloadPlugins() {
    const res = await fetch('/api/plugins/reload', { method: 'POST', headers: controlHeaders() });
    if (!res.ok) {
      alert('Reload failed: ' + (await res.text()));
      return;
    }
    if (window.SRPluginShell) window.SRPluginShell.clearCache();
    await refreshAll();
  }

  function buildControls() {
    const strip = document.getElementById('control-strip');
    if (!strip) return;
    strip.innerHTML =
      '<div class="plugin-quick">' +
      '<label class="plugin-active-label">Aktiv' +
      '<select id="plugin-active-select"></select></label>' +
      '<button type="button" class="btn btn-primary" id="plugin-activate-btn">Aktivieren</button>' +
      '<button type="button" class="btn" id="plugin-reload-btn">Neu laden</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-settings-drawer">Einstellungen</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-theme-toggle">Dunkelmodus</button>' +
      '<button type="button" class="btn btn-ghost" id="btn-fullscreen-toggle">Vollbild</button>' +
      '</div>';

    document.getElementById('plugin-activate-btn').onclick = function () {
      const sel = document.getElementById('plugin-active-select');
      if (sel && sel.value) activatePlugin(sel.value);
    };
    document.getElementById('plugin-reload-btn').onclick = function () { reloadPlugins(); };
    document.getElementById('btn-settings-drawer').onclick = function () {
      if (window.SRConfigEditor) {
        window.SRConfigEditor.open({
          getRanges: function () { return (core.config && core.config.ranges) || 1; },
          controlToken: controlToken,
          pluginId: (activePlugin && activePlugin.id) || activeMeta.pluginId || ''
        });
      }
    };
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
    const activeId = (activePlugin && activePlugin.id) || activeMeta.pluginId || '';
    sel.innerHTML = installedPlugins.map(function (p) {
      return '<option value="' + escapeHtml(p.id) + '"' +
        (p.id === activeId ? ' selected' : '') + '>' +
        escapeHtml(p.label || p.id) + ' (' + escapeHtml(p.kind || '') + ')</option>';
    }).join('');
  }

  async function refreshAll() {
    await core.fetchConfig();
    controlToken = (core.config && core.config.controlToken) || '';
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
      (live.ranges || []).forEach(function (r) { queueLiveRange(r); });
    } else if (!wsOpen) {
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
    const bar = document.getElementById('top-bar');
    if (toggle && menu) {
      toggle.onclick = function () {
        const open = menu.hidden;
        menu.hidden = !open;
        toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
        if (bar) bar.classList.toggle('is-menu-open', open);
      };
    }
    const stopSlim = document.getElementById('plugin-stop-slim');
    if (stopSlim) stopSlim.hidden = true;
  }

  function refreshThemeDependentViews() {
    if (core.lastLiveData) core.render(core.lastLiveData);
  }

  async function init() {
    wireMenu();
    buildControls();
    document.addEventListener('srdashboard:themechange', refreshThemeDependentViews);
    await refreshAll();
    connectWS();
    setInterval(function () {
      if (!wsOpen) pollFallback();
    }, 1000);
    setInterval(function () {
      if (wsOpen) pollFallback();
    }, core.SAFETY_POLL_MS || 20000);
  }

  init();
})();
