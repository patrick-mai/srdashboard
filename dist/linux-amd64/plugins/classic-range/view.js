window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

window.SRPluginViews['classic-range'] = function render(container, viewModel, assetsBase) {
  const core = window.SRCore;
  if (!core || !container) return;

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

  container.className = 'range-plugin-view classic-range-view';
  if (rangeNum != null) container.dataset.range = String(rangeNum);

  if (!rangeData) {
    container.innerHTML = '<div class="classic-range-loading">Warte auf Livedaten…</div>';
    return;
  }

  if (typeof core.renderClassicRangeView === 'function') {
    core.renderClassicRangeView(container, rangeData);
  }
};

window.SRPlugins.render = function (id, container, viewModel, assetsBase) {
  const fn = window.SRPluginViews[id];
  if (typeof fn === 'function') {
    fn(container, viewModel, assetsBase);
  }
};
