(function () {
window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

const PLUGIN_ID = 'classic-range-condensed';

function ensureTargetRegistry(assetsBase) {
  if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === PLUGIN_ID) {
    return Promise.resolve();
  }
  const root = (assetsBase || '/plugins/classic-range-condensed/assets')
    .replace(/\/assets\/?$/, '/')
    .replace(/\/?$/, '/');
  return new Promise(function (resolve, reject) {
    const script = document.createElement('script');
    script.src = root + 'target-registry.js?t=' + Date.now();
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

function resolveRangeData(viewModel) {
  const core = window.SRCore;
  const rangeNum = viewModel && viewModel.rangeNum;
  if (core && core.lastLiveData) {
    const live = (core.lastLiveData.ranges || []).find(function (r) {
      return r.rangeNum === rangeNum;
    });
    if (live) return live;
  }
  return viewModel && viewModel.range ? viewModel.range : null;
}

function fmtInt(n) {
  if (n == null || !Number.isFinite(Number(n))) return '–';
  return String(Math.round(Number(n)));
}

function isAuflageDiscipline(rangeData) {
  const label = String((rangeData && rangeData.discipline) || '').toLowerCase();
  return /auflage|aufgelegt/.test(label);
}

function lastShotInt(rangeData) {
  const shots = rangeData.shots || [];
  if (shots.length) {
    const f = Number(shots[shots.length - 1].fullValue);
    if (Number.isFinite(f)) return f;
  }
  const v = Number(rangeData.currentValue);
  return Number.isFinite(v) ? Math.floor(v) : 0;
}

function seriesLine(rangeData, decimal) {
  const shots = rangeData.shots || [];
  const SERIES_LEN = 10;
  const liveOpen = shots.length > 0 && shots.length < SERIES_LEN;
  const ints = (rangeData.seriesSumsInt || []).slice();
  const seriesShots = rangeData.seriesShots || [];
  const sumsHaveOpen = ints.length > seriesShots.length;
  if (decimal) {
    const decs = (rangeData.seriesSums || []).slice();
    if (liveOpen && !sumsHaveOpen && shots.length) {
      let sumDec = 0;
      shots.forEach(function (s) {
        sumDec += Number(s.decValue) || 0;
      });
      decs.push(sumDec);
    }
    if (!decs.length) return '–';
    return decs.map(fmtDec).join('|');
  }
  if (liveOpen && !sumsHaveOpen && shots.length) {
    let sumInt = 0;
    shots.forEach(function (s) {
      sumInt += Number(s.fullValue) || 0;
    });
    ints.push(sumInt);
  }
  if (!ints.length) return '–';
  return ints.map(fmtInt).join('|');
}

function renderCondensedFooter(rangeData) {
  const hasShots = (Number(rangeData.shotNumber) || 0) > 0 ||
    (rangeData.shots && rangeData.shots.length > 0);
  const decimal = isAuflageDiscipline(rangeData);
  const lastShot = !hasShots ? '–' : (decimal ? fmtDec(rangeData.currentValue) : fmtInt(lastShotInt(rangeData)));
  const total = !hasShots ? '–' : (decimal ? fmtDec(rangeData.overallSumDecimal) : fmtInt(rangeData.overallSumInt));
  const teiler = hasShots && rangeData.currentTeiler != null ? fmtDec(rangeData.currentTeiler) + 'T' : '–';
  const hr = total;
  const shotNum = hasShots && rangeData.shotNumber != null ? String(rangeData.shotNumber) : '–';
  const series = hasShots ? seriesLine(rangeData, decimal) : '–';

  return (
    '<div class="crc-footer-grid">' +
    '<div class="crc-footer-row crc-footer-primary">' +
    '<span class="crc-val crc-last-shot">' + escapeHtml(lastShot) + '</span>' +
    '<span class="crc-val crc-total">' + escapeHtml(total) + '</span>' +
    '</div>' +
    '<div class="crc-footer-row crc-footer-secondary">' +
    '<span class="crc-val crc-teiler">' + escapeHtml(teiler) + '</span>' +
    '<span class="crc-val crc-hr">HR: ' + escapeHtml(hr) + '</span>' +
    '</div>' +
    '<div class="crc-footer-row crc-footer-tertiary">' +
    '<span class="crc-val crc-shot-num">' + escapeHtml(shotNum) + '</span>' +
    '<span class="crc-val crc-series">' + escapeHtml(series) + '</span>' +
    '</div>' +
    '</div>'
  );
}

function shotCountFromLabel(label) {
  const m = String(label || '').match(/(\d+)\s*schuss/i);
  return m ? parseInt(m[1], 10) : 0;
}

/** Hall abbreviations: LG40, LGA30, LP40, KK40 — count from the program, not a fixed label. */
function disciplineAbbrev(rangeData) {
  if (!rangeData) return '';
  const disc = String(rangeData.discType || '').toUpperCase();
  const label = String(rangeData.discipline || '');
  const low = label.toLowerCase();
  const n = Number(rangeData.totalShotsToFire) || shotCountFromLabel(label);
  const auflage = isAuflageDiscipline(rangeData);

  let code = '';
  if (disc === 'LP' || /(^|[^a-z])lp([^a-z]|$)/.test(low) || low.indexOf('luftpistole') !== -1) {
    code = 'LP';
  } else if (disc === 'KK' || /(^|[^a-z])kk([^a-z]|$)/.test(low) || low.indexOf('kleinkaliber') !== -1) {
    code = 'KK';
  } else if (disc === 'LG' || /(^|[^a-z])lg([^a-z]|$)/.test(low) || low.indexOf('luftgewehr') !== -1) {
    code = auflage ? 'LGA' : 'LG';
  } else if (auflage) {
    code = 'LGA';
  } else if (disc) {
    code = disc;
  }
  if (!code) return '';
  return n > 0 ? code + String(n) : code;
}

/** Dark hall colors; same club always maps to the same swatch. */
const CRC_CLUB_COLORS = [
  '#3d5c4a',
  '#3d4a5c',
  '#5c3d4a',
  '#4a4f3d',
  '#4a3d5c',
  '#5c4a3d',
  '#3d555c',
  '#5c3d3d',
  '#3d5c5c',
  '#4f3d5c',
  '#3d5c3d',
  '#5c553d'
];

function clubHeaderColor(clubName) {
  const key = String(clubName || '').trim().toLowerCase().replace(/\s+/g, ' ');
  if (!key) return '';
  let hash = 2166136261;
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return CRC_CLUB_COLORS[(hash >>> 0) % CRC_CLUB_COLORS.length];
}

const lastShotSigByRange = {};

function shotFlashSig(rangeData) {
  return [
    rangeData.isWarmup ? 'w' : 'c',
    rangeData.shotNumber || 0,
    rangeData.currentValue,
    rangeData.currentTeiler
  ].join('|');
}

function headerFlashColor(header, rangeData) {
  if (header) {
    const painted = header.style.backgroundColor;
    if (painted) return painted;
    const computed = getComputedStyle(header).backgroundColor;
    if (computed && computed !== 'rgba(0, 0, 0, 0)' && computed !== 'transparent') return computed;
  }
  return clubHeaderColor(rangeData.clubName) || '#3d4f4c';
}

function clearFooterHit(footerEl) {
  if (!footerEl) return;
  footerEl.classList.remove('crc-footer-hit');
  footerEl.style.backgroundColor = '';
  footerEl.style.color = '';
  footerEl.style.borderTopColor = '';
  footerEl._crcHitTimer = null;
}

function flashFooterOnShot(footerEl, rangeData, header) {
  if (!footerEl || !rangeData || rangeData.rangeNum == null) return;
  const n = rangeData.rangeNum;
  const sig = shotFlashSig(rangeData);
  const prev = lastShotSigByRange[n];
  lastShotSigByRange[n] = sig;
  const hasShots = (Number(rangeData.shotNumber) || 0) > 0 ||
    (rangeData.shots && rangeData.shots.length > 0);
  if (prev == null || prev === sig || !hasShots) return;

  const color = headerFlashColor(header, rangeData);
  footerEl.style.setProperty('--crc-hit-bg', color);
  footerEl.style.backgroundColor = color;
  footerEl.style.color = '#fff';
  footerEl.style.borderTopColor = 'transparent';
  footerEl.classList.add('crc-footer-hit');
  if (footerEl._crcHitTimer) clearTimeout(footerEl._crcHitTimer);
  footerEl._crcHitTimer = setTimeout(function () {
    clearFooterHit(footerEl);
  }, 1000);
}

function shotInstantMs(shot) {
  if (!shot) return 0;
  const at = Date.parse(shot.at || '');
  if (Number.isFinite(at) && at > 0) return at;
  const recv = Date.parse(shot.receivedAt || '');
  return Number.isFinite(recv) && recv > 0 ? recv : 0;
}

function earliestShotMs(rangeData) {
  let best = 0;
  function consider(list) {
    if (!list) return;
    for (let i = 0; i < list.length; i++) {
      const t = shotInstantMs(list[i]);
      if (t && (!best || t < best)) best = t;
    }
  }
  consider(rangeData.warmupShots);
  const series = rangeData.seriesShots || [];
  for (let s = 0; s < series.length; s++) consider(series[s]);
  consider(rangeData.shots);
  return best;
}

function pad2(n) {
  return (n < 10 ? '0' : '') + n;
}

function formatStartClock(rangeData) {
  let ms = 0;
  if (rangeData.startedAt) ms = Date.parse(rangeData.startedAt) || 0;
  if (!ms) ms = earliestShotMs(rangeData);
  if (!ms) return '';
  const d = new Date(ms);
  if (isNaN(d.getTime())) return '';
  return pad2(d.getHours()) + ':' + pad2(d.getMinutes()) + ':' + pad2(d.getSeconds());
}

function shooterTipHtml(rangeData) {
  if (!rangeData || !rangeData.shooterName) return '';
  const club = String(rangeData.clubName || '').trim();
  const start = formatStartClock(rangeData);
  const parts = [];
  if (club) parts.push('<span class="crc-tip-club">' + escapeHtml(club) + '</span>');
  if (start) parts.push('<span class="crc-tip-start">Start: ' + escapeHtml(start) + '</span>');
  return parts.join('');
}

function fillCondensedHeader(header, rangeData) {
  if (!header || !rangeData) return;
  const hall = window.SRCore && window.SRCore.getHallPluginId && window.SRCore.getHallPluginId();
  if (hall && hall !== PLUGIN_ID) return;
  const num = rangeData.rangeNum;
  const name = rangeData.shooterName || ('Stand ' + num + ' – kein Schütze');
  const disc = rangeData.shooterName ? disciplineAbbrev(rangeData) : '';
  const club = String(rangeData.clubName || '').trim();
  const tip = shooterTipHtml(rangeData);
  header.className = 'range-header crc-header' + (rangeData.shooterName ? '' : ' empty');
  header.style.backgroundColor = rangeData.shooterName && club ? clubHeaderColor(club) : '';
  header.removeAttribute('title');
  const html =
    '<div class="crc-header-line">' +
    '<span class="crc-lane">' + escapeHtml(String(num)) + '</span>' +
    '<span class="crc-sep" aria-hidden="true">|</span>' +
    '<span class="crc-name-wrap">' +
    '<span class="crc-name">' + escapeHtml(name) + '</span>' +
    (tip ? '<span class="crc-tip" role="tooltip">' + tip + '</span>' : '') +
    '</span>' +
    (disc ? '<span class="crc-disc">' + escapeHtml(disc) + '</span>' : '') +
    '</div>';
  const stale = !header.querySelector('.crc-header-line') || header.querySelector('.range-header-top');
  if (stale || header._crcHtml !== html) {
    header.innerHTML = html;
    header._crcHtml = html;
  }
}

function renderCondensedView(container, rangeData) {
  const core = window.SRCore;
  if (!core || !container || !rangeData) return;
  const hall = core.getHallPluginId && core.getHallPluginId();
  if (hall && hall !== PLUGIN_ID) return;

  if (
    container.querySelector('.ar-master-layout, .ar-shooter-layout, .plugin-fallback, .plugin-error') ||
    (container.dataset.pluginId && container.dataset.pluginId !== PLUGIN_ID)
  ) {
    container.innerHTML = '';
  }

  container.className = 'range-plugin-view crc-view';
  container.dataset.pluginId = PLUGIN_ID;
  if (rangeData.rangeNum != null) container.dataset.range = String(rangeData.rangeNum);

  const panel = container.closest('.range-panel');
  if (panel) panel.classList.add('crc-panel');

  let targetEl = container.querySelector(':scope > .range-target');
  if (!targetEl) {
    targetEl = document.createElement('div');
    targetEl.className = 'range-target';
    container.appendChild(targetEl);
  }
  targetEl.classList.toggle('warmup', !!rangeData.isWarmup);

  container.querySelectorAll(':scope > .last10-chart-wrap').forEach(function (el) {
    el.remove();
  });

  let footerEl = container.querySelector(':scope > .range-footer');
  if (!footerEl) {
    footerEl = document.createElement('div');
    footerEl.className = 'range-footer crc-footer';
    container.appendChild(footerEl);
  } else {
    const hit = footerEl.classList.contains('crc-footer-hit');
    footerEl.className = 'range-footer crc-footer' + (hit ? ' crc-footer-hit' : '');
  }
  footerEl.innerHTML = renderCondensedFooter(rangeData);
  const header = panel ? panel.querySelector('.range-header') : null;
  flashFooterOnShot(footerEl, rangeData, header);

  restorePromotedLastShot(targetEl);
  if (typeof core.renderTarget === 'function') {
    core.renderTarget(targetEl, rangeData, rangeData.isWarmup);
  }
  promoteLastShot(targetEl);
}

/** Put last-shot circles back in fill/ring groups so core upsert can update them. */
function restorePromotedLastShot(targetEl) {
  if (!targetEl) return;
  const fillG = targetEl.querySelector('.target-shots-fill');
  const ringG = targetEl.querySelector('.target-shots-ring');
  const root = targetEl.querySelector('.target-shots');
  if (!root) return;
  root.querySelectorAll(':scope > circle.crc-last-fill').forEach(function (c) {
    c.classList.remove('crc-last-fill');
    if (fillG) fillG.appendChild(c);
  });
  root.querySelectorAll(':scope > circle.crc-last-ring').forEach(function (c) {
    c.classList.remove('crc-last-ring');
    if (ringG) ringG.appendChild(c);
  });
}

/**
 * Previous-shot rims stay above gray fills so overlaps stay outlined.
 * The current shot is lifted above that stack so other rims cannot cut it.
 */
function promoteLastShot(targetEl) {
  if (!targetEl) return;
  const root = targetEl.querySelector('.target-shots');
  if (!root) return;
  const fillLast = root.querySelector('.target-shots-fill circle.is-last');
  const ringLast = root.querySelector('.target-shots-ring circle.is-last');
  if (fillLast) {
    fillLast.classList.add('crc-last-fill');
    root.appendChild(fillLast);
  }
  if (ringLast) {
    ringLast.classList.add('crc-last-ring');
    root.appendChild(ringLast);
  }
  const halo = root.querySelector('.target-shot-hover-halo');
  const label = root.querySelector('.target-shot-hover-label');
  if (halo) root.appendChild(halo);
  if (label) root.appendChild(label);
}

function paint(container, viewModel, assetsBase) {
  const core = window.SRCore;
  if (!core || !container) return;
  const hall = core.getHallPluginId && core.getHallPluginId();
  if (hall && hall !== PLUGIN_ID) return;

  if (typeof core.setTargetAssetBase === 'function' && assetsBase) {
    core.setTargetAssetBase(assetsBase);
  }

  const rangeData = resolveRangeData(viewModel);
  const rangeNum = viewModel && viewModel.rangeNum;

  if (container.dataset.pluginId && container.dataset.pluginId !== PLUGIN_ID) {
    container.innerHTML = '';
  }

  if (!rangeData) {
    container.className = 'range-plugin-view crc-view';
    container.dataset.pluginId = PLUGIN_ID;
    if (rangeNum != null) container.dataset.range = String(rangeNum);
    container.innerHTML = '<div class="classic-range-loading">Warte auf Livedaten…</div>';
    return;
  }

  const panel = container.closest('.range-panel');
  if (panel) {
    const header = panel.querySelector('.range-header');
    if (header) fillCondensedHeader(header, rangeData);
  }

  renderCondensedView(container, rangeData);
}

window.SRClassicRangeCondensed = {
  fillHeader: fillCondensedHeader,
  paint: renderCondensedView
};

window.SRPluginViews[PLUGIN_ID] = function render(container, viewModel, assetsBase) {
  return ensureTargetRegistry(assetsBase).then(function () {
    paint(container, viewModel, assetsBase);
  });
};

window.SRPlugins.render = function (id, container, viewModel, assetsBase) {
  const fn = window.SRPluginViews[id];
  if (typeof fn === 'function') {
    fn(container, viewModel, assetsBase);
  }
};
})();
