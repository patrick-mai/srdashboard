window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

function ensureClassicTargetRegistry(assetsBase) {
  if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === 'classic-range') {
    return Promise.resolve();
  }
  // assetsBase is /plugins/<id>/assets — registry lives one level up.
  const root = (assetsBase || '/plugins/classic-range/assets')
    .replace(/\/assets\/?$/, '/')
    .replace(/\/?$/, '/');
  return new Promise(function (resolve, reject) {
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

window.SRPluginViews['classic-range'] = function render(container, viewModel, assetsBase) {
  const core = window.SRCore;
  if (!core || !container) return;

  const paint = function () {
    if (typeof core.setTargetAssetBase === 'function' && assetsBase) {
      core.setTargetAssetBase(assetsBase);
    }

    const rangeNum = viewModel && viewModel.rangeNum;
    let rangeData = null;
    if (core.lastLiveData) {
      rangeData = (core.lastLiveData.ranges || []).find(function (r) {
        return r.rangeNum === rangeNum;
      });
    }
    if (!rangeData && viewModel) rangeData = viewModel.range;

    if (container.dataset.pluginId && container.dataset.pluginId !== 'classic-range') {
      container.innerHTML = '';
    }
    container.className = 'range-plugin-view classic-range-view';
    container.dataset.pluginId = 'classic-range';
    if (rangeNum != null) container.dataset.range = String(rangeNum);

    if (!rangeData) {
      container.innerHTML = '<div class="classic-range-loading">Warte auf Livedaten…</div>';
      return;
    }

    if (typeof core.renderClassicRangeView === 'function') {
      core.renderClassicRangeView(container, rangeData);
    }
  };

  ensureClassicTargetRegistry(assetsBase).then(paint);
};

window.SRPlugins.render = function (id, container, viewModel, assetsBase) {
  const fn = window.SRPluginViews[id];
  if (typeof fn === 'function') {
    fn(container, viewModel, assetsBase);
  }
};
