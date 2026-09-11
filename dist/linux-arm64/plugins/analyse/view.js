(function () {
window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

const PLUGIN_ID = 'analyse';
const CLASSIC_ASSETS = '/plugins/classic-range/assets';

function ensureClassicTargetRegistry() {
  if (window.SRTargetRegistry && (
    window.SRTargetRegistry.ownerPluginId === PLUGIN_ID ||
    window.SRTargetRegistry.ownerPluginId === 'classic-range'
  )) {
    if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = PLUGIN_ID;
    return Promise.resolve();
  }
  return new Promise(function (resolve, reject) {
    const script = document.createElement('script');
    script.src = '/plugins/classic-range/target-registry.js?t=' + Date.now();
    script.onload = function () {
      if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = PLUGIN_ID;
      resolve();
    };
    script.onerror = reject;
    document.head.appendChild(script);
  }).catch(function () {});
}

function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function fmtDec(n) {
  if (n == null || !Number.isFinite(Number(n))) return '–';
  return Number(n).toFixed(1).replace('.', ',');
}

function pad2(n) {
  return (n < 10 ? '0' : '') + n;
}

function formatClock(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  return pad2(d.getHours()) + ':' + pad2(d.getMinutes());
}

function resultLabel(row) {
  const bahn = 'Bahn ' + (row.rangeNum || '?');
  const name = String(row.shooterName || '').trim() || 'ohne Namen';
  const sum = fmtDec(row.overallSumDecimal);
  const shots = row.shotNumber || 0;
  const total = row.totalShotsToFire || 0;
  const prog = total > 0 ? (shots + '/' + total) : String(shots);
  if (row.live) {
    const phase = row.isWarmup ? 'Probe' : 'läuft';
    return bahn + ' · ' + name + ' · ' + phase + ' · ' + prog + ' · ' + sum;
  }
  const clock = formatClock(row.startedAt) || formatClock(row.archivedAt);
  return bahn + ' · ' + name + (clock ? ' · ' + clock : '') + ' · ' + sum;
}

function setAnalyseSurfaceClasses(container) {
  if (!container || !container.classList) return;
  container.classList.add('shared-master-host', 'analyse-view');
  container._sharedReady = true;
}

function preferRange(viewModel) {
  const n = viewModel && Number(viewModel.rangeNum);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function pickDefaultId(results, rangeNum, previous) {
  if (previous) {
    for (let i = 0; i < results.length; i++) {
      if (results[i].id === previous) return previous;
    }
  }
  if (rangeNum > 0) {
    for (let i = 0; i < results.length; i++) {
      if (results[i].live && results[i].rangeNum === rangeNum) return results[i].id;
    }
    for (let i = 0; i < results.length; i++) {
      if (results[i].rangeNum === rangeNum) return results[i].id;
    }
  }
  return results.length ? results[0].id : '';
}

function liveRangeFromCore(rangeNum) {
  const core = window.SRCore;
  const ranges = (core && core.lastLiveData && core.lastLiveData.ranges) || [];
  for (let i = 0; i < ranges.length; i++) {
    if (ranges[i].rangeNum === rangeNum) return ranges[i];
  }
  return null;
}

function withSessionId(rangeData, id) {
  const copy = Object.assign({}, rangeData || {});
  copy.sessionResultId = id;
  return copy;
}

function ensureShell(container) {
  let layout = container.querySelector(':scope > .analyse-layout');
  if (layout) return layout;
  container.innerHTML = '';
  layout = document.createElement('div');
  layout.className = 'analyse-layout';
  layout.innerHTML =
    '<div class="analyse-toolbar">' +
      '<label class="analyse-toolbar-label" for="analyse-result-select">Ergebnis</label>' +
      '<select class="analyse-select" id="analyse-result-select"></select>' +
    '</div>' +
    '<p class="analyse-empty" hidden>Noch keine Ergebnisse in dieser Sitzung.</p>' +
    '<div class="range-panel analyse-panel plugin-hosted" hidden>' +
      '<div class="range-header"></div>' +
      '<div class="range-plugin-view classic-range-view"></div>' +
    '</div>';
  container.appendChild(layout);
  const sel = layout.querySelector('#analyse-result-select');
  sel.addEventListener('change', function () {
    container.dataset.analyseId = sel.value || '';
    paintSelected(container);
  });
  return layout;
}

function fillSelect(sel, results, selectedId) {
  const html = results.map(function (row) {
    return '<option value="' + escapeHtml(row.id) + '"' +
      (row.id === selectedId ? ' selected' : '') + '>' +
      escapeHtml(resultLabel(row)) + '</option>';
  }).join('');
  if (sel.innerHTML !== html) sel.innerHTML = html;
  if (selectedId && sel.value !== selectedId) sel.value = selectedId;
}

function showEmpty(layout, empty) {
  const emptyEl = layout.querySelector('.analyse-empty');
  const panel = layout.querySelector('.analyse-panel');
  const toolbar = layout.querySelector('.analyse-toolbar');
  if (emptyEl) emptyEl.hidden = !empty;
  if (panel) panel.hidden = empty;
  if (toolbar) toolbar.hidden = empty;
}

function paintClassic(layout, rangeData) {
  const core = window.SRCore;
  if (!core || !rangeData) return;
  const panel = layout.querySelector('.analyse-panel');
  const header = layout.querySelector('.range-header');
  const mount = layout.querySelector('.range-plugin-view');
  if (!panel || !header || !mount) return;
  panel.hidden = false;
  panel.dataset.range = String(rangeData.rangeNum || '');
  if (typeof core.fillRangeHeader === 'function') {
    core.fillRangeHeader(header, rangeData);
  }
  if (typeof core.renderClassicRangeView === 'function') {
    core.renderClassicRangeView(mount, rangeData);
  }
}

async function loadRangeForId(container, id, summary) {
  if (summary && summary.live && summary.rangeNum) {
    const live = liveRangeFromCore(summary.rangeNum);
    if (live) return withSessionId(live, id);
  }
  if (summary && !summary.live && container._analyseSnap && container._analyseSnap[id]) {
    return withSessionId(container._analyseSnap[id], id);
  }
  const res = await fetch('/api/analyse/results?id=' + encodeURIComponent(id), { cache: 'no-store' });
  if (!res.ok) return null;
  const data = await res.json();
  if (data.range && !data.live) {
    container._analyseSnap = container._analyseSnap || {};
    container._analyseSnap[id] = data.range;
  }
  return withSessionId(data.range || null, id);
}

async function paintSelected(container) {
  const layout = container.querySelector(':scope > .analyse-layout');
  if (!layout) return;
  const id = container.dataset.analyseId || '';
  const results = container._analyseResults || [];
  let summary = null;
  for (let i = 0; i < results.length; i++) {
    if (results[i].id === id) { summary = results[i]; break; }
  }
  if (!id || !summary) {
    showEmpty(layout, true);
    return;
  }
  showEmpty(layout, false);
  container._analysePaint = (container._analysePaint || 0) + 1;
  const token = container._analysePaint;
  const rangeData = await loadRangeForId(container, id, summary);
  if (container._analysePaint !== token) return;
  if (!rangeData) {
    showEmpty(layout, true);
    return;
  }
  paintClassic(layout, rangeData);
}

async function paint(container, viewModel, assetsBase) {
  const core = window.SRCore;
  if (!core || !container) return;
  setAnalyseSurfaceClasses(container);
  if (typeof core.setTargetAssetBase === 'function') {
    core.setTargetAssetBase(CLASSIC_ASSETS);
  }
  const layout = ensureShell(container);
  container._analyseToken = (container._analyseToken || 0) + 1;
  const token = container._analyseToken;
  let results = [];
  try {
    const res = await fetch('/api/analyse/results', { cache: 'no-store' });
    if (res.ok) {
      const data = await res.json();
      results = data.results || [];
    }
  } catch (e) {}
  if (container._analyseToken !== token) return;
  container._analyseResults = results;
  if (!results.length) {
    container.dataset.analyseId = '';
    showEmpty(layout, true);
    return;
  }
  const selected = pickDefaultId(results, preferRange(viewModel), container.dataset.analyseId);
  container.dataset.analyseId = selected;
  fillSelect(layout.querySelector('#analyse-result-select'), results, selected);
  await paintSelected(container);
}

window.SRPluginViews[PLUGIN_ID] = function render(container, viewModel, assetsBase) {
  return ensureClassicTargetRegistry().then(function () {
    return paint(container, viewModel, assetsBase);
  });
};
})();
