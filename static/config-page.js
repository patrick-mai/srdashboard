(function () {
  const app = document.getElementById('config-app');
  if (!app || !window.SRConfigEditor) return;

  document.body.classList.add('config-display');

  app.innerHTML =
    '<div class="config-page">' +
    '<header class="config-page-head">' +
    '<div class="config-page-brand">' +
    '<a class="config-page-back" href="/">← Live</a>' +
    '<div>' +
    '<h1>Einstellungen</h1>' +
    '<p class="config-page-sub">Standort und aktives Plugin</p>' +
    '</div></div>' +
    '<div class="config-page-actions">' +
    '<button type="button" class="btn btn-ghost" id="config-page-theme">Dunkelmodus</button>' +
    '</div>' +
    '</header>' +
    '<div id="config-page-panel" class="config-page-panel"></div>' +
    '</div>';

  const panel = document.getElementById('config-page-panel');
  const themeBtn = document.getElementById('config-page-theme');
  if (themeBtn && window.SRTheme) {
    function sync() {
      const dark = window.SRTheme.get() === 'dark';
      themeBtn.textContent = dark ? 'Hellmodus' : 'Dunkelmodus';
    }
    sync();
    themeBtn.onclick = function () {
      window.SRTheme.toggle();
      sync();
    };
  }

  let activePluginId = '';

  async function boot() {
    try {
      const act = await fetch('/api/plugins/active');
      if (act.ok) {
        const list = await act.json();
        if (list && list[0]) activePluginId = list[0].id || '';
      }
    } catch (e) { /* ignore */ }

    window.SRConfigEditor.mountPage(panel, {
      pluginId: activePluginId
    });
  }

  boot();
})();
