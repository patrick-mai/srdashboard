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

function isAuflageLabel(label) {
  return /auflage|aufgelegt/i.test(String(label || ''));
}

function isAuflageDiscipline(rangeData) {
  return isAuflageLabel(rangeData && rangeData.discipline);
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

/** Dark hall colors; hashed from club, or from Mannschaft when a club fields more than one team. */
const CRC_HEADER_COLORS = [
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

function headerColorFromKey(name) {
  const key = String(name || '').trim().toLowerCase().replace(/\s+/g, ' ');
  if (!key) return '';
  let hash = 2166136261;
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return CRC_HEADER_COLORS[(hash >>> 0) % CRC_HEADER_COLORS.length];
}

function clubHeaderColor(clubName) {
  return headerColorFromKey(clubName);
}

function clubsWithMultipleTeams(model) {
  const teams = (model && model.teams) || [];
  const extras = extrasByTeamId(model, teams);
  const byClub = {};
  teams.forEach(function (t) {
    const extraN = extras[t.id] ? extras[t.id].length : 0;
    if (!(t.members && t.members.length) && !extraN) return;
    let club = String((t && t.club) || '').trim();
    if (!club && t.members && t.members[0]) club = String(t.members[0].club || '').trim();
    if (!club) return;
    const k = club.toLowerCase();
    byClub[k] = (byClub[k] || 0) + 1;
  });
  const multi = {};
  Object.keys(byClub).forEach(function (k) {
    if (byClub[k] > 1) multi[k] = true;
  });
  return multi;
}

function rangeHeaderColor(rangeData, model) {
  const club = String((rangeData && rangeData.clubName) || '').trim();
  const team = String((rangeData && rangeData.teamName) || '').trim();
  const multi = clubsWithMultipleTeams(model || (window.SRCore && window.SRCore.lastWettkampf));
  if (club && team && multi[club.toLowerCase()]) return headerColorFromKey(team);
  return clubHeaderColor(club);
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
  return rangeHeaderColor(rangeData, window.SRCore && window.SRCore.lastWettkampf) || '#3d4f4c';
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
  header.style.backgroundColor = rangeData.shooterName ? (rangeHeaderColor(rangeData) || '') : '';
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
  promoteLastShot(targetEl, rangeData);
}

const CRC_SHOT_BANDS = ['crc-shot-10', 'crc-shot-9', 'crc-shot-low'];

function currentShotDecValue(rangeData, fillEl) {
  if (fillEl) {
    const fromEl = Number(fillEl.getAttribute('data-dec-value'));
    if (Number.isFinite(fromEl)) return fromEl;
  }
  const shots = rangeData && rangeData.shots;
  if (shots && shots.length) {
    const last = shots[shots.length - 1];
    const dec = Number(last.decValue);
    if (Number.isFinite(dec)) return dec;
    const full = Number(last.fullValue);
    if (Number.isFinite(full)) return full;
  }
  const cur = Number(rangeData && rangeData.currentValue);
  return Number.isFinite(cur) ? cur : null;
}

function shotBandClass(value) {
  if (value == null || !Number.isFinite(value)) return 'crc-shot-10';
  if (value >= 10) return 'crc-shot-10';
  if (value >= 9) return 'crc-shot-9';
  return 'crc-shot-low';
}

function setShotBand(el, band) {
  if (!el || !el.classList) return;
  CRC_SHOT_BANDS.forEach(function (c) { el.classList.remove(c); });
  if (band) el.classList.add(band);
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
    setShotBand(c, null);
    if (fillG) fillG.appendChild(c);
  });
  root.querySelectorAll(':scope > circle.crc-last-ring').forEach(function (c) {
    c.classList.remove('crc-last-ring');
    setShotBand(c, null);
    if (ringG) ringG.appendChild(c);
  });
}

/**
 * Previous-shot rims stay above gray fills so overlaps stay outlined.
 * The current shot is lifted above that stack so other rims cannot cut it.
 */
function promoteLastShot(targetEl, rangeData) {
  if (!targetEl) return;
  const root = targetEl.querySelector('.target-shots');
  if (!root) return;
  const fillLast = root.querySelector('.target-shots-fill circle.is-last');
  const ringLast = root.querySelector('.target-shots-ring circle.is-last');
  const band = shotBandClass(currentShotDecValue(rangeData, fillLast));
  if (fillLast) {
    fillLast.classList.add('crc-last-fill');
    setShotBand(fillLast, band);
    root.appendChild(fillLast);
  }
  if (ringLast) {
    ringLast.classList.add('crc-last-ring');
    setShotBand(ringLast, band);
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

function canEditWettkampf() {
  return !(window.SRMode && window.SRMode.canControl === false);
}

function fmtWkDec(n) {
  if (n == null || !Number.isFinite(Number(n))) return '–';
  return Number(n).toFixed(1).replace('.', ',');
}

function memberStatus(m, live) {
  if (!m) return '–';
  if (m.locked || (m.totalShots > 0 && m.shotsFired >= m.totalShots && m.hasWertung)) return 'fertig';
  const ranges = (live && live.ranges) || [];
  const name = String(m.name || '').trim();
  for (let i = 0; i < ranges.length; i++) {
    if (String(ranges[i].shooterName || '').trim() === name) {
      return 'Bahn ' + ranges[i].rangeNum;
    }
  }
  if (m.isWarmup) return 'Probe';
  if (m.rangeNum) return 'Bahn ' + m.rangeNum;
  return '–';
}

function teamHeaderColor(t, model) {
  const club = String((t && t.club) || '').trim();
  const teamName = String((t && t.name) || '').trim();
  const multi = clubsWithMultipleTeams(model);
  if (club && teamName && multi[club.toLowerCase()]) {
    const byTeam = headerColorFromKey(teamName);
    if (byTeam) return byTeam;
  }
  if (club) {
    const c = clubHeaderColor(club);
    if (c) return c;
  }
  const members = (t && t.members) || [];
  for (let i = 0; i < members.length; i++) {
    const c = clubHeaderColor(members[i].club);
    if (c) return c;
  }
  const roster = (model && model.roster) || [];
  const id = t && t.id;
  const name = t && t.name;
  for (let i = 0; i < roster.length; i++) {
    const r = roster[i];
    if ((id && r.teamId === id) || (name && r.teamHint === name)) {
      const c = clubHeaderColor(r.club);
      if (c) return c;
    }
  }
  return clubHeaderColor(name) || '#3d4f4c';
}

function wettkampfMemberRow(m, live, extra, decimal) {
  const useDec = decimal || isAuflageLabel(m && m.discipline);
  const score = (m && m.hasWertung)
    ? (useDec ? fmtWkDec(m.sumDec) : fmtInt(m.sumInt))
    : '–';
  return '<div class="crc-wk-member' + (extra ? ' is-extra' : '') + '">' +
    '<span class="crc-wk-mn">' + escapeHtml(m.name) + '</span>' +
    '<span class="crc-wk-ms">' + escapeHtml(memberStatus(m, live)) + '</span>' +
    '<span class="crc-wk-mv">' + escapeHtml(score) + '</span></div>';
}

function teamUsesDecimal(t, extras, model) {
  if (t && t.decimal) return true;
  const people = ((t && t.members) || []).concat(extras || []);
  for (let i = 0; i < people.length; i++) {
    if (isAuflageLabel(people[i].discipline) || isAuflageLabel(disciplineOf(people[i], model))) return true;
  }
  return false;
}

function disciplineOf(m, model) {
  if (m && m.discipline) return m.discipline;
  const roster = (model && model.roster) || [];
  const key = m && m.key;
  const name = m && m.name;
  for (let i = 0; i < roster.length; i++) {
    if ((key && roster[i].key === key) || (name && roster[i].name === name)) {
      return roster[i].discipline || '';
    }
  }
  return '';
}

function extrasByTeamId(model, teams) {
  const map = {};
  (teams || []).forEach(function (t) {
    if (t && t.id) map[t.id] = [];
  });
  ((model && model.roster) || []).forEach(function (r) {
    if (!r || !r.excluded) return;
    const dest = extraTeamId(r, teams);
    if (dest && map[dest]) map[dest].push(r);
  });
  Object.keys(map).forEach(function (id) {
    map[id].sort(function (a, b) {
      return String(a.name || '').localeCompare(String(b.name || ''), 'de');
    });
  });
  return map;
}

function extraTeamId(r, teams) {
  if (r.teamId) {
    for (let i = 0; i < teams.length; i++) {
      if (teams[i].id === r.teamId) return teams[i].id;
    }
  }
  const hint = String(r.teamHint || '').trim().toLowerCase();
  const club = String(r.club || '').trim().toLowerCase();
  for (let i = 0; i < teams.length; i++) {
    if (hint && hint === String(teams[i].name || '').trim().toLowerCase()) return teams[i].id;
  }
  if (!club) return '';
  for (let i = 0; i < teams.length; i++) {
    if (club === String(teams[i].club || '').trim().toLowerCase()) return teams[i].id;
    const members = teams[i].members || [];
    for (let j = 0; j < members.length; j++) {
      if (club === String(members[j].club || '').trim().toLowerCase()) return teams[i].id;
    }
  }
  return '';
}

function renderWettkampfBody(model, live) {
  const teams = (model && model.teams) || [];
  const roster = (model && model.roster) || [];
  const extras = extrasByTeamId(model, teams);
  const visible = teams.filter(function (t) {
    const clubExtras = extras[t.id] || [];
    return (t.members && t.members.length) || clubExtras.length;
  });
  if (!visible.length) {
    if (!roster.length) {
      return '<div class="crc-wk-empty">Keine Schützen — im Menü Vereine anlegen oder schießen lassen.</div>';
    }
    return '<div class="crc-wk-empty">Schützen vorhanden — im Menü Mannschaften zuordnen.</div>';
  }
  let html = '<div class="crc-wk-teams">';
  visible.forEach(function (t) {
    const exp = t.expected || (model && model.expectedPerTeam) || 0;
    const frac = exp > 0 ? (t.count || 0) + '/' + exp : String(t.count || 0);
    const bg = teamHeaderColor(t, model);
    const clubExtras = extras[t.id] || [];
    const decimal = teamUsesDecimal(t, clubExtras, model);
    const sum = decimal ? fmtWkDec(t.sumDec) : fmtInt(t.sumInt);
    const pred = decimal ? fmtWkDec(t.predDec) : fmtInt(t.predInt);
    html += '<div class="crc-wk-team">' +
      '<div class="crc-wk-team-head" style="background-color:' + bg + '">' +
      '<span class="crc-wk-team-name">' + escapeHtml(t.name) + '</span>' +
      '<span class="crc-wk-frac">' + escapeHtml(frac) + '</span></div>' +
      '<div class="crc-wk-scores' + (decimal ? ' is-decimal' : '') + '">' +
      '<div class="crc-wk-row"><span>Summe</span><strong>' + escapeHtml(sum) + '</strong>' +
      (decimal ? '' : '<span>' + escapeHtml(fmtWkDec(t.sumDec)) + '</span>') + '</div>' +
      '<div class="crc-wk-row"><span>Prognose</span><strong>' + escapeHtml(pred) + '</strong>' +
      (decimal ? '' : '<span>' + escapeHtml(fmtWkDec(t.predDec)) + '</span>') + '</div></div>' +
      '<div class="crc-wk-members">';
    (t.members || []).forEach(function (m) {
      html += wettkampfMemberRow(m, live, false, decimal);
    });
    html += '</div>';
    if (clubExtras.length) {
      html += '<div class="crc-wk-extras">';
      clubExtras.forEach(function (m) {
        html += wettkampfMemberRow(m, live, true, decimal);
      });
      html += '</div>';
    }
    html += '</div>';
  });
  html += '</div>';
  return html;
}

function fillWettkampfHeader(header) {
  if (!header) return;
  const gear = canEditWettkampf()
    ? '<button type="button" class="crc-wk-gear" title="Wettkampf einrichten" aria-label="Wettkampf einrichten">⚙</button>'
    : '';
  const html = '<div class="crc-header-line crc-wk-header-line">' +
    '<span class="crc-name">Wettkampf</span>' + gear + '</div>';
  if (header._crcWkHtml === html && header.querySelector('.crc-wk-header-line')) return;
  header.className = 'range-header crc-header crc-wk-header';
  header.style.backgroundColor = '#3d4f4c';
  header.innerHTML = html;
  header._crcWkHtml = html;
  const btn = header.querySelector('.crc-wk-gear');
  if (btn) btn.addEventListener('click', function (ev) {
    ev.preventDefault();
    ev.stopPropagation();
    openWettkampfModal();
  });
}

function paintWettkampf(container, header, model, live) {
  const hall = window.SRCore && window.SRCore.getHallPluginId && window.SRCore.getHallPluginId();
  if (hall && hall !== PLUGIN_ID) return;
  if (!container) return;
  fillWettkampfHeader(header);
  container.className = 'range-plugin-view crc-wettkampf-view';
  container.dataset.pluginId = PLUGIN_ID;
  const html = renderWettkampfBody(model, live);
  if (container._wkHtml !== html) {
    container.innerHTML = html;
    container._wkHtml = html;
  }
}

function wkAuthFetch(url, options) {
  if (window.SRAuth && window.SRAuth.fetchWithAuth) {
    return window.SRAuth.fetchWithAuth(url, options);
  }
  return fetch(url, options);
}

function ensureWettkampfModal() {
  let modal = document.getElementById('crc-wk-modal');
  if (modal) return modal;
  modal = document.createElement('div');
  modal.id = 'crc-wk-modal';
  modal.className = 'crc-wk-modal';
  modal.hidden = true;
  modal.innerHTML =
    '<div class="crc-wk-modal-backdrop" data-wk="close"></div>' +
    '<div class="crc-wk-dialog" role="dialog" aria-modal="true" aria-labelledby="crc-wk-title">' +
    '<h2 id="crc-wk-title">Wettkampf</h2>' +
    '<div class="crc-wk-dialog-body"></div>' +
    '<div class="crc-wk-dialog-actions">' +
    '<button type="button" class="btn" data-wk="assign">Nach Verein zuordnen</button>' +
    '<button type="button" class="btn" data-wk="reset">Wettkampf zurücksetzen</button>' +
    '<span class="crc-wk-dialog-spacer"></span>' +
    '<button type="button" class="btn" data-wk="close">Schließen</button>' +
    '<button type="button" class="btn btn-primary" data-wk="save">Speichern</button>' +
    '</div></div>';
  document.body.appendChild(modal);
  modal.addEventListener('click', function (ev) {
    const act = ev.target && ev.target.getAttribute && ev.target.getAttribute('data-wk');
    if (act === 'close') closeWettkampfModal();
    if (act === 'save') saveWettkampfModal();
    if (act === 'assign') assignWettkampfModal();
    if (act === 'reset') resetWettkampfModal();
    if (act === 'add-team') addWettkampfTeamRow(modal);
    if (act === 'del-team') {
      const row = ev.target.closest('.crc-wk-team-edit');
      if (row) row.remove();
    }
  });
  modal.addEventListener('change', function (ev) {
    const t = ev.target;
    if (!t || !t.classList || !t.classList.contains('crc-wk-exclude')) return;
    const tr = t.closest('tr');
    if (!tr) return;
    const sel = tr.querySelector('.crc-wk-assign');
    if (sel) sel.disabled = !!t.checked;
    tr.classList.toggle('is-excluded', !!t.checked);
  });
  document.addEventListener('keydown', function (ev) {
    if (ev.key === 'Escape' && modal && !modal.hidden) closeWettkampfModal();
  });
  return modal;
}

function closeWettkampfModal() {
  const modal = document.getElementById('crc-wk-modal');
  if (modal) modal.hidden = true;
}

function openWettkampfModal() {
  if (!canEditWettkampf()) return;
  const modal = ensureWettkampfModal();
  const body = modal.querySelector('.crc-wk-dialog-body');
  body.innerHTML = '<p>Laden…</p>';
  modal.hidden = false;
  fetch('/api/wettkampf', { cache: 'no-store' }).then(function (res) {
    return res.json();
  }).then(function (data) {
    paintWettkampfModal(modal, data);
  }).catch(function () {
    body.innerHTML = '<p>Wettkampf konnte nicht geladen werden.</p>';
  });
}

function paintWettkampfModal(modal, data) {
  const body = modal.querySelector('.crc-wk-dialog-body');
  const teams = (data && data.teams) || [];
  const roster = (data && data.roster) || [];
  const exp = data && data.expectedPerTeam != null ? data.expectedPerTeam : 4;
  let html = '<label class="crc-wk-field">Schützen pro Mannschaft (0 = ohne Bruch)' +
    '<input type="number" min="0" max="20" id="crc-wk-expected" value="' + escapeHtml(String(exp)) + '"></label>';
  html += '<div class="crc-wk-team-edits">';
  teams.forEach(function (t) {
    html += wettkampfTeamRow(t.id, t.name);
  });
  html += '</div>';
  html += '<div class="crc-wk-add-row"><input type="text" id="crc-wk-new-team" placeholder="Neue Mannschaft">' +
    '<button type="button" class="btn" data-wk="add-team">Hinzufügen</button></div>';
  html += '<table class="crc-wk-roster"><thead><tr>' +
    '<th>Schütze</th><th>Verein</th><th>UDP-Team</th><th>Mannschaft</th><th>Summe</th><th>Fixiert</th><th>ohne Mannschaft</th>' +
    '</tr></thead><tbody>';
  roster.forEach(function (r) {
    html += '<tr data-key="' + escapeHtml(r.key) + '"' + (r.excluded ? ' class="is-excluded"' : '') + '>' +
      '<td>' + escapeHtml(r.name) + '</td>' +
      '<td>' + escapeHtml(r.club) + '</td>' +
      '<td>' + escapeHtml(r.teamHint) + '</td>' +
      '<td><select class="crc-wk-assign"' + (r.excluded ? ' disabled' : '') + '>' +
      wettkampfTeamOptions(teams, r.excluded ? '' : r.teamId) + '</select></td>' +
      '<td>' + escapeHtml(isAuflageLabel(r.discipline)
        ? fmtWkDec(r.hasWertung ? r.sumDec : null)
        : (fmtInt(r.hasWertung ? r.sumInt : null) + ' / ' + fmtWkDec(r.hasWertung ? r.sumDec : null))) + '</td>' +
      '<td><input type="checkbox" class="crc-wk-lock" title="Gespeicherte Summe nicht überschreiben"' +
      (r.locked ? ' checked' : '') + '></td>' +
      '<td><input type="checkbox" class="crc-wk-exclude" title="Schütze bleibt auf der Bahn, zählt nicht zur Mannschaft"' +
      (r.excluded ? ' checked' : '') + '></td></tr>';
  });
  html += '</tbody></table>';
  if (!roster.length) html += '<p class="crc-wk-hint">Noch keine Schützen in diesem Wettkampf.</p>';
  body.innerHTML = html;
}

function wettkampfTeamRow(id, name) {
  return '<div class="crc-wk-team-edit" data-id="' + escapeHtml(id || '') + '">' +
    '<input type="text" class="crc-wk-team-name" value="' + escapeHtml(name || '') + '">' +
    '<button type="button" class="btn" data-wk="del-team">Entfernen</button></div>';
}

function wettkampfTeamOptions(teams, selected) {
  let html = '<option value="">—</option>';
  (teams || []).forEach(function (t) {
    html += '<option value="' + escapeHtml(t.id) + '"' + (t.id === selected ? ' selected' : '') + '>' +
      escapeHtml(t.name) + '</option>';
  });
  return html;
}

function addWettkampfTeamRow(modal) {
  const inp = modal.querySelector('#crc-wk-new-team');
  const name = inp && inp.value ? inp.value.trim() : '';
  if (!name) return;
  const wrap = modal.querySelector('.crc-wk-team-edits');
  if (wrap) wrap.insertAdjacentHTML('beforeend', wettkampfTeamRow('', name));
  if (inp) inp.value = '';
}

function collectWettkampfPatch(modal, extra) {
  const patch = extra || {};
  const expEl = modal.querySelector('#crc-wk-expected');
  if (expEl) patch.expectedPerTeam = parseInt(expEl.value, 10) || 0;
  patch.teams = [];
  modal.querySelectorAll('.crc-wk-team-edit').forEach(function (row) {
    const name = (row.querySelector('.crc-wk-team-name') || {}).value || '';
    patch.teams.push({ id: row.getAttribute('data-id') || '', name: name.trim() });
  });
  patch.roster = [];
  modal.querySelectorAll('.crc-wk-roster tbody tr').forEach(function (tr) {
    const sel = tr.querySelector('.crc-wk-assign');
    const lock = tr.querySelector('.crc-wk-lock');
    const excl = tr.querySelector('.crc-wk-exclude');
    const excluded = !!(excl && excl.checked);
    patch.roster.push({
      key: tr.getAttribute('data-key') || '',
      teamId: sel ? sel.value : '',
      locked: !!(lock && lock.checked),
      excluded: excluded
    });
  });
  return patch;
}

function putWettkampf(patch) {
  return wkAuthFetch('/api/wettkampf', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch)
  }).then(function (res) {
    if (!res.ok) return res.text().then(function (t) { throw new Error(t || res.status); });
    return res.json();
  }).then(function (data) {
    if (window.SRCore) window.SRCore.lastWettkampf = data;
    if (window.SRCore && window.SRCore.paintWettkampfTile) window.SRCore.paintWettkampfTile();
    return data;
  });
}

function saveWettkampfModal() {
  const modal = document.getElementById('crc-wk-modal');
  if (!modal) return;
  putWettkampf(collectWettkampfPatch(modal, { action: 'save' })).then(function (data) {
    paintWettkampfModal(modal, data);
  }).catch(function (err) {
    alert('Speichern: ' + (err && err.message ? err.message : err));
  });
}

function assignWettkampfModal() {
  const modal = document.getElementById('crc-wk-modal');
  if (!modal) return;
  putWettkampf(collectWettkampfPatch(modal, { action: 'assignByClub', assignByClub: true })).then(function (data) {
    paintWettkampfModal(modal, data);
  }).catch(function (err) {
    alert('Zuordnen: ' + (err && err.message ? err.message : err));
  });
}

function resetWettkampfModal() {
  if (!window.confirm('Wettkampf zurücksetzen? Schützen und Ergebnisse werden gelöscht, Mannschaftsnamen bleiben.')) {
    return;
  }
  const modal = document.getElementById('crc-wk-modal');
  if (!modal) return;
  putWettkampf({ reset: true }).then(function (data) {
    paintWettkampfModal(modal, data);
  }).catch(function (err) {
    alert('Zurücksetzen: ' + (err && err.message ? err.message : err));
  });
}

window.SRClassicRangeCondensed = {
  fillHeader: fillCondensedHeader,
  paint: renderCondensedView,
  paintWettkampf: paintWettkampf
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
