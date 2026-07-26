window.SRPluginShell = (function () {
  // Loads a plugin's view.js, then delegates to SRPlugins / SRPluginViews.
  const loadedScripts = {};
  const loadedThemes = {};

  async function ensureViewScript(pluginId, viewUrl) {
    if (loadedScripts[pluginId]) return;
    await new Promise(function (resolve, reject) {
      const s = document.createElement('script');
      s.src = viewUrl + (viewUrl.indexOf('?') >= 0 ? '&' : '?') + 't=' + Date.now();
      s.onload = resolve;
      s.onerror = reject;
      document.head.appendChild(s);
    });
    loadedScripts[pluginId] = true;
  }

  function ensureTheme(pluginId, themeUrl) {
    if (!themeUrl || loadedThemes[pluginId]) return;
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = themeUrl + (themeUrl.indexOf('?') >= 0 ? '&' : '?') + 't=' + Date.now();
    document.head.appendChild(link);
    loadedThemes[pluginId] = true;
  }

  async function renderPluginView(container, pluginId, viewUrl, assetsBase, viewModel, themeUrl) {
    if (!container) return;
    try {
      await ensureViewScript(pluginId, viewUrl);
      ensureTheme(pluginId, themeUrl);
      if (window.SRPlugins && typeof window.SRPlugins.render === 'function') {
        window.SRPlugins.render(pluginId, container, viewModel, assetsBase);
        return;
      }
      const fn = window.SRPluginViews && window.SRPluginViews[pluginId];
      if (typeof fn === 'function') {
        fn(container, viewModel, assetsBase);
      } else {
        container.innerHTML = '<div class="plugin-fallback"><pre>' +
          JSON.stringify(viewModel, null, 2) + '</pre></div>';
      }
    } catch (e) {
      container.innerHTML = '<div class="plugin-error">Failed to load plugin view</div>';
      console.warn('SRPluginShell.renderPluginView', e);
    }
  }

  function clearCache(pluginId) {
    if (pluginId) {
      delete loadedScripts[pluginId];
      delete loadedThemes[pluginId];
    } else {
      Object.keys(loadedScripts).forEach(function (k) { delete loadedScripts[k]; });
      Object.keys(loadedThemes).forEach(function (k) { delete loadedThemes[k]; });
    }
  }

  return { renderPluginView, clearCache, ensureViewScript, ensureTheme };
})();
