window.SRPluginShell = (function () {
  // Loads a plugin's view.js, then delegates to SRPlugins / SRPluginViews.
  const loadedScripts = {}; // pluginId -> <script> element
  const loadedThemes = {}; // pluginId -> <link> element

  function removeTheme(pluginId) {
    const link = loadedThemes[pluginId];
    if (link && link.parentNode) link.parentNode.removeChild(link);
    delete loadedThemes[pluginId];
  }

  function unloadOtherThemes(activePluginId) {
    Object.keys(loadedThemes).forEach(function (id) {
      if (id !== activePluginId) removeTheme(id);
    });
  }

  // Plugin views may only be loaded from this server's own /plugins/{id}/ tree,
  // so a bad API response cannot turn into arbitrary script execution.
  function resolveViewUrl(pluginId, viewUrl) {
    const url = new URL(viewUrl, location.origin);
    if (url.origin !== location.origin) return null;
    const prefix = '/plugins/' + encodeURIComponent(pluginId) + '/';
    if (url.pathname.indexOf(prefix) !== 0) return null;
    return url;
  }

  async function ensureViewScript(pluginId, viewUrl) {
    if (loadedScripts[pluginId]) return;
    const url = resolveViewUrl(pluginId, viewUrl);
    if (!url) throw new Error('refusing to load plugin view from ' + viewUrl);
    url.searchParams.set('t', String(Date.now()));
    const script = document.createElement('script');
    await new Promise(function (resolve, reject) {
      script.src = url.pathname + url.search;
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
    loadedScripts[pluginId] = script;
  }

  function ensureTheme(pluginId, themeUrl) {
    unloadOtherThemes(pluginId || '');
    if (!themeUrl) {
      if (pluginId) removeTheme(pluginId);
      return;
    }
    if (loadedThemes[pluginId]) return;
    const url = resolveViewUrl(pluginId, themeUrl);
    if (!url) return;
    url.searchParams.set('t', String(Date.now()));
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.dataset.pluginTheme = pluginId;
    link.href = url.pathname + url.search;
    document.head.appendChild(link);
    loadedThemes[pluginId] = link;
  }

  async function renderPluginView(container, pluginId, viewUrl, assetsBase, viewModel, themeUrl) {
    if (!container) return;
    try {
      if (container.dataset.pluginId !== pluginId) {
        container.innerHTML = '';
        container.dataset.pluginId = pluginId || '';
      }
      await ensureViewScript(pluginId, viewUrl);
      ensureTheme(pluginId, themeUrl);
      // Prefer the per-plugin registration: SRPlugins.render is a single global
      // that every loaded plugin overwrites, so the last script to load wins.
      const fn = window.SRPluginViews && window.SRPluginViews[pluginId];
      if (typeof fn === 'function') {
        fn(container, viewModel, assetsBase);
      } else if (window.SRPlugins && typeof window.SRPlugins.render === 'function') {
        window.SRPlugins.render(pluginId, container, viewModel, assetsBase);
      } else {
        // textContent, not innerHTML: the view model carries shooter names and
        // other values straight off the wire.
        container.replaceChildren();
        const wrap = document.createElement('div');
        wrap.className = 'plugin-fallback';
        const pre = document.createElement('pre');
        pre.textContent = JSON.stringify(viewModel, null, 2);
        wrap.appendChild(pre);
        container.appendChild(wrap);
      }
    } catch (e) {
      container.replaceChildren();
      const err = document.createElement('div');
      err.className = 'plugin-error';
      err.textContent = 'Failed to load plugin view';
      container.appendChild(err);
      console.warn('SRPluginShell.renderPluginView', e);
    }
  }

  // Also detaches the <script> element; leaving it behind accumulates a tag per
  // reload and lets a stale plugin IIFE keep its registrations alive.
  function removeScript(pluginId) {
    const script = loadedScripts[pluginId];
    if (script && script.parentNode) script.parentNode.removeChild(script);
    delete loadedScripts[pluginId];
  }

  function clearCache(pluginId) {
    if (pluginId) {
      removeScript(pluginId);
      removeTheme(pluginId);
    } else {
      Object.keys(loadedScripts).forEach(removeScript);
      Object.keys(loadedThemes).forEach(removeTheme);
    }
  }

  return { renderPluginView, clearCache, ensureViewScript, ensureTheme, unloadOtherThemes };
})();
