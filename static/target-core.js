const POLL_INTERVAL_MS = 1000;
/** Slow safety poll while WebSocket is connected (master/shooter gate their own timers). */
const SAFETY_POLL_MS = 20000;

// --- Target scale (DISAG OpticScore vs SVG) — see docs/target-scale-verification.md ---
// X, Y = shot coordinates (centre 0,0) in DISAG OpticScore units. Teiler (log "Distance") = sqrt(X^2 + Y^2). DecValue 10.9 = Teiler 0-25, 10.8 = 25-50, etc.
// ±9000 DSG units = 200 mm (confirmed). DSG_PER_MM = 90. SVG viewBox 0 0 200 200, centre 100.
const DSG_COORD_RANGE = 9000;           // max radius from DISAG OpticScore coordinate system
const RANGE_DIAMETER_MM = 200;             // physical diameter that fits log scores (9000 = 100 mm radius)
const DSG_PER_MM = 90;                  // 9000 / 100 (DISAG OpticScore units per mm)
// Default mapping for legacy targets: DSG_PER_SVG_UNIT = 90, SVG viewBox 0 0 200 200, centre 100.
const DEFAULT_DSG_PER_SVG_UNIT = 90;
const TARGET_DIAMETER_MM = 45.5;          // ISSF scoring target (sits inside 200 mm range)
const SHOT_DIAMETER_MM = 4.5;            // pellet diameter (mm)
const TEN_RING_DIAMETER_MM = 0.5;        // 10 ring (ISSF)
const DEFAULT_SVG_CENTER = 100;          // viewBox center (100, 100)
const DEFAULT_SVG_VIEW_SIZE = 200;       // default viewBox size for targets
// DISAG OpticScore → SVG (defaults): x_svg = SVG_CENTER + x_dsg/90, y_svg = SVG_CENTER - y_dsg/90
const SHOT_RADIUS_SVG = (SHOT_DIAMETER_MM / 2) / (RANGE_DIAMETER_MM / 2) * DEFAULT_SVG_CENTER;  // 4.5mm in 200mm range ≈ 2.25 SVG
/** Default pellet outline width (mm); overridden by config.shotStrokeWidth. */
const DEFAULT_SHOT_STROKE_SVG = 0.1;

// Zoom via SVG viewBox only (target image + shot circles in ONE svg). Never CSS-transform.
// viewBox units = mm = SVG units (1 SVG unit = 1 mm at DSG_PER_MM = 90).
const ONE_RING_SVG = 2.5;           // ISSF ring width (mm)
const RING_8_RADIUS_MM = 5.25;      // ISSF ring 8 outer radius — max zoom-in frames this as outer circle
const AUTO_ZOOM_PAD_FRAC = 0.35;    // padding when shots spill past ring 8
const SCORING_DISK_PAD_MM = 4;      // empty/reset: tight frame so the full scoring disk starts large (like yesterday)
/** Tightest allowed viewBox span: ring 8 fills the frame (outer circle). */
const MIN_ZOOM_SPAN_MM = RING_8_RADIUS_MM * 2;

// Shot order: 10 colors rainbow 0°–330° (almost full circle). HSL hue; s=75%, l=50%.
function hslToHex(h, s, l) {
  s /= 100; l /= 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n) => {
    const k = (n + h / 30) % 12;
    return l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
  };
  const r = Math.round(f(0) * 255), g = Math.round(f(8) * 255), b = Math.round(f(4) * 255);
  return '#' + [r, g, b].map((x) => x.toString(16).padStart(2, '0')).join('');
}
// 10 hues evenly from 0° to 330° (red → … → magenta, stopping short of 360°)
const DEFAULT_SHOT_ORDER_COLORS = Array.from({ length: 10 }, (_, i) => hslToHex((i / 9) * 330, 75, 50));

let lastLiveData = null;

function getShotOrderColors() {
  return DEFAULT_SHOT_ORDER_COLORS;
}

function getTargetScale() {
  return {
    dsgPerSvgUnit: DEFAULT_DSG_PER_SVG_UNIT,
    svgSize: DEFAULT_SVG_VIEW_SIZE,
    centerX: DEFAULT_SVG_CENTER,
    centerY: DEFAULT_SVG_CENTER
  };
}

let zoomStateByRange = {};   // rangeNum -> { x, y, w, h } SVG viewBox
let prevShotsLengthByRange = {};  // rangeNum -> number
let prevIsWarmupByRange = {};     // rangeNum -> last isWarmup (warmup→competition resets zoom)
let userZoomedByRange = {};  // rangeNum -> true if user zoomed via wheel/drag; cleared on target reset
let pinFullUntilNewShotByRange = {}; // dblclick: stay at full disk until another shot arrives

let config = {
  ranges: 6,
  layoutColumns: 4,
  defaultTarget: '10_m_Air_Rifle_target.svg',
  shotStrokeWidth: DEFAULT_SHOT_STROKE_SVG,
  footer: {
    currentShotValue: true,
    teiler: true,
    shotNumber: true,
    overallSumInt: true,
    overallSumDecimal: true,
    predictionInt: true,
    predictionDecimal: true,
    seriesSumsInt: true,
    seriesSumsDecimal: true,
    last10Int: true,
    last10Decimal: true
  }
};

function getShotStrokeWidth() {
  const w = Number(config && config.shotStrokeWidth);
  if (!Number.isFinite(w) || w <= 0) return DEFAULT_SHOT_STROKE_SVG;
  return Math.min(w, 2);
}

function getShotFillRadius() {
  return Math.max(0.05, SHOT_RADIUS_SVG - getShotStrokeWidth() / 2);
}

/** Monotonic counter: bumped on WS apply so in-flight HTTP polls can discard stale snapshots. */
let liveGen = 0;

function bumpLiveGen() {
  liveGen += 1;
  return liveGen;
}

function getLiveGen() {
  return liveGen;
}

async function fetchConfig() {
  const res = await fetch('/api/config');
  if (res.ok) {
    config = await res.json();
    applyLayout();
    // Stroke width etc. may change — force panel/target resync.
    document.querySelectorAll('.range-panel').forEach(function (p) {
      delete p.dataset.chromeSig;
    });
    if (lastLiveData) {
      (lastLiveData.ranges || []).forEach(function (r) {
        const panel = document.querySelector('.range-panel[data-range="' + r.rangeNum + '"]');
        if (panel) syncRangePanel(panel, r);
      });
      document.querySelectorAll('.classic-range-view').forEach(function (el) {
        const rangeNum = el.dataset.range || (el.closest('[data-range]') && el.closest('[data-range]').dataset.range);
        const r = (lastLiveData.ranges || []).find(function (x) {
          return String(x.rangeNum) === String(rangeNum);
        });
        if (r) renderClassicRangeView(el, r);
      });
    }
  }
}

/** Optional base URL for the target face (e.g. plugin assetsBase). Empty → /assets/. */
let targetAssetBase = '';

function setTargetAssetBase(base) {
  const next = base ? String(base).replace(/\/?$/, '/') : '';
  if (next === targetAssetBase) return;
  targetAssetBase = next;
  // Force face remount on next render so the new href is picked up.
  document.querySelectorAll('svg.target-svg-root').forEach(function (el) {
    el.remove();
  });
}

function resolveTargetUrl(_rangeNum) {
  const file = (config.defaultTarget || '10_m_Air_Rifle_target.svg').replace(/^.*[\\/]/, '');
  const base = targetAssetBase || '/assets/';
  // Cache-bust when switching asset bases (plugin folder vs static)
  const v = targetAssetBase ? 'issf-scaled-original' : 'issf-scaled-original-static';
  return base + encodeURIComponent(file) + '?v=' + encodeURIComponent(v);
}

function applyLayout() {
  const grid = document.getElementById('ranges-grid');
  if (!grid) return;
  const cols = Math.max(1, config.layoutColumns || 4);
  grid.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;
}

async function fetchLive() {
  const res = await fetch('/api/live', { cache: 'no-store' });
  if (!res.ok) return null;
  return res.json();
}

/**
 * DISAG OpticScore → SVG mm (viewBox centre 100,100; Y flipped to screen/SVG).
 * x_svg = 100 + X/90
 * y_svg = 100 − Y/90
 */
function dsgToSvg(xDsg, yDsg) {
  const ts = getTargetScale();
  return {
    x: ts.centerX + xDsg / ts.dsgPerSvgUnit,
    y: ts.centerY - yDsg / ts.dsgPerSvgUnit
  };
}

/** Empty / reset: frame the ISSF scoring disk (not the full 200 mm range). */
function fullDiskZoom() {
  const ts = getTargetScale();
  const half = TARGET_DIAMETER_MM / 2 + SCORING_DISK_PAD_MM;
  return clampZoomWindow(
    ts.centerX - half, ts.centerX + half,
    ts.centerY - half, ts.centerY + half
  );
}

/** Entire DISAG 200 mm range (debug / max zoom-out). */
function rangeDiskZoom() {
  const ts = getTargetScale();
  return { x: 0, y: 0, w: ts.svgSize, h: ts.svgSize };
}

function resetRangeZoom(rangeNum) {
  zoomStateByRange[rangeNum] = fullDiskZoom();
  userZoomedByRange[rangeNum] = false;
  pinFullUntilNewShotByRange[rangeNum] = false;
}

function viewSpan(state) {
  if (!state || state.w == null) return TARGET_DIAMETER_MM + 2 * SCORING_DISK_PAD_MM;
  return Math.max(state.w, state.h);
}

/** Square viewBox window, clamped to zoom limits, centred on the given bounds. */
function clampZoomWindow(x0, x1, y0, y1) {
  const ts = getTargetScale();
  const minSpan = MIN_ZOOM_SPAN_MM;
  // Cap at the SVG range. span > svgSize makes [span/2, svgSize-span/2] inverted and
  // pins the viewBox at (0,0) — disk jumps to the upper-left.
  const maxSpan = ts.svgSize;
  let cx = (x0 + x1) / 2;
  let cy = (y0 + y1) / 2;
  let span = Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0), 1);
  span = Math.max(minSpan, Math.min(maxSpan, span));
  if (span >= ts.svgSize) {
    return { x: 0, y: 0, w: ts.svgSize, h: ts.svgSize };
  }
  cx = Math.max(span / 2, Math.min(ts.svgSize - span / 2, cx));
  cy = Math.max(span / 2, Math.min(ts.svgSize - span / 2, cy));
  return { x: cx - span / 2, y: cy - span / 2, w: span, h: span };
}

/** Tightest zoom that still shows all shots, always centred on the bullseye. */
function computeAutoFit(shots) {
  const ts = getTargetScale();
  if (!shots || shots.length === 0) {
    return fullDiskZoom();
  }
  let maxDist = 0;
  for (let i = 0; i < shots.length; i++) {
    const pt = dsgToSvg(Number(shots[i].x), Number(shots[i].y));
    const dist = Math.hypot(pt.x - ts.centerX, pt.y - ts.centerY) + SHOT_RADIUS_SVG;
    if (dist > maxDist) maxDist = dist;
  }
  // All shots inside ring 8 → zoom so 8 is the outer circle (max zoom-in).
  if (maxDist <= RING_8_RADIUS_MM) {
    const half = RING_8_RADIUS_MM;
    return clampZoomWindow(
      ts.centerX - half, ts.centerX + half,
      ts.centerY - half, ts.centerY + half
    );
  }
  // Otherwise frame the group with light pad; never tighter than ring 8.
  const pad = Math.max(ONE_RING_SVG, AUTO_ZOOM_PAD_FRAC * maxDist * 2);
  let half = Math.max(maxDist + pad, RING_8_RADIUS_MM);
  half = Math.min(half, ts.svgSize / 2);
  return clampZoomWindow(
    ts.centerX - half, ts.centerX + half,
    ts.centerY - half, ts.centerY + half
  );
}

/** True if a shot point lies outside (or on the edge of) the current viewBox. */
function shotOutsideView(pt, state) {
  if (!state || state.w == null) return true;
  const m = SHOT_RADIUS_SVG;
  return (
    pt.x - m < state.x ||
    pt.x + m > state.x + state.w ||
    pt.y - m < state.y ||
    pt.y + m > state.y + state.h
  );
}

function applyViewBox(svg, state) {
  if (!svg || !state) return;
  svg.setAttribute('viewBox', `${state.x} ${state.y} ${state.w} ${state.h}`);
}

function setupZoomHandlers(viewport, rangeNum) {
  if (viewport.dataset.zoomHandlers === '1') return;
  viewport.dataset.zoomHandlers = '1';

  function getState() {
    if (!zoomStateByRange[rangeNum]) zoomStateByRange[rangeNum] = fullDiskZoom();
    return zoomStateByRange[rangeNum];
  }

  function getSvg() {
    return viewport.querySelector('.target-svg-root');
  }

  viewport.addEventListener('wheel', (e) => {
    e.preventDefault();
    userZoomedByRange[rangeNum] = true;
    const state = getState();
    const span = viewSpan(state);
    const factor = e.deltaY > 0 ? 1.12 : 1 / 1.12;
    const half = (span * factor) / 2;
    // Always zoom about the bullseye — never pan/recenter via wheel.
    const ts = getTargetScale();
    zoomStateByRange[rangeNum] = clampZoomWindow(
      ts.centerX - half, ts.centerX + half,
      ts.centerY - half, ts.centerY + half
    );
    applyViewBox(getSvg(), zoomStateByRange[rangeNum]);
  }, { passive: false });

  // Pan disabled: view stays centred on the target; only zoom in/out + dblclick reset.
  viewport.addEventListener('dblclick', () => {
    zoomStateByRange[rangeNum] = fullDiskZoom();
    userZoomedByRange[rangeNum] = false;
    pinFullUntilNewShotByRange[rangeNum] = true;
    applyViewBox(getSvg(), zoomStateByRange[rangeNum]);
  });
}

function ensureTargetSvg(container) {
  let wrapper = container.querySelector('.target-wrapper');
  if (!wrapper) {
    wrapper = document.createElement('div');
    wrapper.className = 'target-wrapper';
    container.appendChild(wrapper);
  }
  // Strip legacy Plotly / CSS-zoom layers only — never remount a healthy SVG (cache-bust URLs vary).
  wrapper.querySelectorAll('.target-plot, .target-svg, .zoom-debug').forEach((el) => el.remove());
  wrapper.style.transform = '';

  let svg = wrapper.querySelector('svg.target-svg-root');
  if (svg && svg.querySelector('.target-face') && svg.querySelector('.target-shots')) {
    return svg;
  }
  if (svg) svg.remove();

  const ts = getTargetScale();
  svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('class', 'target-svg-root');
  svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
  svg.setAttribute('viewBox', `0 0 ${ts.svgSize} ${ts.svgSize}`);

  // ISSF artwork 1:1 into viewBox 0 0 200 200 — shots share this user space via viewBox zoom.
  const face = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  face.setAttribute('class', 'target-face');
  face.setAttribute('data-style', 'issf-original');
  const img = document.createElementNS('http://www.w3.org/2000/svg', 'image');
  img.setAttribute('href', resolveTargetUrl());
  img.setAttributeNS('http://www.w3.org/1999/xlink', 'href', resolveTargetUrl());
  img.setAttribute('x', '0');
  img.setAttribute('y', '0');
  img.setAttribute('width', String(ts.svgSize));
  img.setAttribute('height', String(ts.svgSize));
  img.setAttribute('preserveAspectRatio', 'xMidYMid meet');
  face.appendChild(img);
  const marker = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  marker.setAttribute('cx', String(ts.centerX));
  marker.setAttribute('cy', String(ts.centerY));
  marker.setAttribute('r', '22.75');
  marker.setAttribute('fill', 'none');
  marker.setAttribute('stroke', 'none');
  marker.setAttribute('pointer-events', 'none');
  face.appendChild(marker);
  svg.appendChild(face);

  const shotsGroup = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  shotsGroup.setAttribute('class', 'target-shots');
  svg.appendChild(shotsGroup);
  wrapper.appendChild(svg);
  return svg;
}

function upsertShotCircles(shotsGroup, shots) {
  const colors = getShotOrderColors();
  // Keep the whole shot stack above the target face.
  if (shotsGroup.parentNode && shotsGroup.parentNode.lastElementChild !== shotsGroup) {
    shotsGroup.parentNode.appendChild(shotsGroup);
  }

  let fillG = shotsGroup.querySelector('.target-shots-fill');
  let ringG = shotsGroup.querySelector('.target-shots-ring');
  if (!fillG) {
    fillG = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    fillG.setAttribute('class', 'target-shots-fill');
    shotsGroup.insertBefore(fillG, shotsGroup.firstChild);
  }
  if (!ringG) {
    ringG = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    ringG.setAttribute('class', 'target-shots-ring');
    shotsGroup.appendChild(ringG);
  } else {
    shotsGroup.appendChild(ringG); // outlines always above fills
  }

  // Migrate legacy single-circle children into the fill layer once.
  Array.prototype.slice.call(shotsGroup.children).forEach(function (n) {
    if (n === fillG || n === ringG) return;
    if (n.tagName && n.tagName.toLowerCase() === 'circle') fillG.appendChild(n);
    else shotsGroup.removeChild(n);
  });

  const fills = [];
  const rings = [];
  Array.prototype.forEach.call(fillG.children, function (n) {
    if (n.tagName && n.tagName.toLowerCase() === 'circle') fills.push(n);
  });
  Array.prototype.forEach.call(ringG.children, function (n) {
    if (n.tagName && n.tagName.toLowerCase() === 'circle') rings.push(n);
  });

  shots.forEach((s, i) => {
    const pt = dsgToSvg(Number(s.x), Number(s.y));
    let fill = fills[i];
    if (!fill) {
      fill = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      fill.appendChild(document.createElementNS('http://www.w3.org/2000/svg', 'title'));
      fillG.appendChild(fill);
    }
    fill.setAttribute('cx', String(pt.x));
    fill.setAttribute('cy', String(pt.y));
    fill.setAttribute('r', String(getShotFillRadius()));
    fill.setAttribute('fill', colors[i % colors.length]);
    fill.setAttribute('fill-opacity', '1');
    fill.setAttribute('stroke', 'none');
    const title = fill.querySelector('title');
    if (title) {
      title.textContent = `#${i + 1}: ${Number(s.decValue).toFixed(1)} (T ${Number(s.distance).toFixed(1)})`;
    }

    let ring = rings[i];
    if (!ring) {
      ring = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      ring.setAttribute('fill', 'none');
      ringG.appendChild(ring);
    }
    ring.setAttribute('cx', String(pt.x));
    ring.setAttribute('cy', String(pt.y));
    ring.setAttribute('r', String(getShotFillRadius()));
    ring.setAttribute('stroke', colors[i % colors.length]);
    ring.setAttribute('stroke-width', String(getShotStrokeWidth()));
    ring.setAttribute('stroke-opacity', '1');
    ring.setAttribute('pointer-events', 'none');
  });

  for (let i = fills.length - 1; i >= shots.length; i--) {
    fillG.removeChild(fills[i]);
  }
  for (let i = rings.length - 1; i >= shots.length; i--) {
    ringG.removeChild(rings[i]);
  }
}

function renderTarget(container, rangeData, isWarmup) {
  if (!container || !rangeData) return;

  container.querySelectorAll('.target-plot').forEach((el) => el.remove());

  let svg = container.querySelector('svg.target-svg-root');
  if (!svg || !svg.querySelector('.target-face') || !svg.querySelector('.target-shots')) {
    svg = ensureTargetSvg(container);
  }
  const shotsGroup = svg.querySelector('.target-shots');
  if (!shotsGroup) return;

  const shots = rangeData.shots || [];
  const rangeNum = rangeData.rangeNum;
  const prevLen = prevShotsLengthByRange[rangeNum] ?? 0;
  const currentLen = shots.length;
  const warmupFlag = !!(isWarmup != null ? isWarmup : rangeData.isWarmup);
  const wasWarmup = prevIsWarmupByRange[rangeNum];
  const shotsCleared = currentLen < prevLen;
  const warmupToCompetition = wasWarmup === true && warmupFlag === false;

  if (shotsCleared || warmupToCompetition) {
    resetRangeZoom(rangeNum);
  }
  if (currentLen > prevLen) {
    pinFullUntilNewShotByRange[rangeNum] = false;
  }

  if (currentLen === 0 || pinFullUntilNewShotByRange[rangeNum]) {
    if (!userZoomedByRange[rangeNum]) {
      zoomStateByRange[rangeNum] = fullDiskZoom();
    }
  } else if (!userZoomedByRange[rangeNum]) {
    zoomStateByRange[rangeNum] = computeAutoFit(shots);
  } else {
    // Keep bullseye-centred user zoom, but expand if a shot would be clipped.
    const state = zoomStateByRange[rangeNum];
    const latest = shots[shots.length - 1];
    if (latest) {
      const pt = dsgToSvg(Number(latest.x), Number(latest.y));
      if (shotOutsideView(pt, state)) {
        const fitted = computeAutoFit(shots);
        if (viewSpan(fitted) > viewSpan(state)) {
          zoomStateByRange[rangeNum] = fitted;
        }
      }
    }
  }
  prevShotsLengthByRange[rangeNum] = currentLen;
  prevIsWarmupByRange[rangeNum] = warmupFlag;

  if (!zoomStateByRange[rangeNum] || zoomStateByRange[rangeNum].w == null) {
    zoomStateByRange[rangeNum] = fullDiskZoom();
  }
  applyViewBox(svg, zoomStateByRange[rangeNum]);
  setupZoomHandlers(container, rangeNum);
  upsertShotCircles(shotsGroup, shots);
}

const DEFAULT_CHART_COLORS = {
  bar: '#0a7a8c',
  paperBg: '#f7fafc',
  plotBg: '#f7fafc',
  font: '#5a6b7a',
  border: '#b8c6d1'
};

function getChartColors() {
  const styles = getComputedStyle(document.documentElement);
  const pick = (name, fallback) => {
    const v = styles.getPropertyValue(name).trim();
    return v || fallback;
  };
  return {
    bar: pick('--accent', DEFAULT_CHART_COLORS.bar),
    paperBg: pick('--panel', DEFAULT_CHART_COLORS.paperBg),
    plotBg: pick('--panel', DEFAULT_CHART_COLORS.plotBg),
    font: pick('--muted', DEFAULT_CHART_COLORS.font),
    border: pick('--line', DEFAULT_CHART_COLORS.border)
  };
}

/** Absolute ceiling for decimal ring values (10.9). */
const LAST10_VALUE_MAX = 10.9;

/** Series-relative Y range so tight high scores still use the full plot height. */
function computeValueRange(last10Values) {
  if (!last10Values || last10Values.length === 0) return [0, LAST10_VALUE_MAX];
  const vals = last10Values.map(Number).filter((n) => !Number.isNaN(n));
  if (vals.length === 0) return [0, LAST10_VALUE_MAX];
  let minV = Math.min(...vals);
  let maxV = Math.max(...vals);
  const padding = 0.15;
  if (minV === maxV) {
    minV = Math.max(0, minV - 0.5);
    maxV = Math.min(LAST10_VALUE_MAX, maxV + 0.5);
  } else {
    const span = maxV - minV;
    minV = Math.max(0, minV - padding * span);
    maxV = Math.min(LAST10_VALUE_MAX, maxV + padding * span);
  }
  if (maxV <= minV) maxV = Math.min(LAST10_VALUE_MAX, minV + 0.5);
  return [minV, maxV];
}

function renderLast10Chart(container, last10Values) {
  if (!container) return;
  const colors = getChartColors();
  const shotColors = getShotOrderColors();
  const values = (last10Values || []).map(Number).filter((n) => !Number.isNaN(n));
  const [yMin, yMax] = computeValueRange(last10Values);
  const ySpan = Math.max(0.01, yMax - yMin);

  // Match viewBox to the laid-out size so labels are not stretched by preserveAspectRatio=none.
  const rect = container.getBoundingClientRect();
  const W = Math.max(120, Math.round(rect.width) || 200);
  const H = Math.max(40, Math.round(rect.height) || 56);
  const padL = 28;
  const padR = 6;
  const padT = 6;
  const padB = 16;
  const plotW = W - padL - padR;
  const plotH = H - padT - padB;
  const slotW = plotW / 10;
  const fontSize = Math.max(9, Math.round(H * 0.16));

  let svg = container.querySelector('svg.last10-svg');
  if (!svg) {
    container.innerHTML = '';
    svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'last10-svg');
    svg.setAttribute('aria-hidden', 'true');
    container.appendChild(svg);
  }
  svg.setAttribute('viewBox', `0 0 ${W} ${H}`);
  svg.setAttribute('preserveAspectRatio', 'none');

  const yToSvg = (v) => padT + plotH - ((v - yMin) / ySpan) * plotH;
  // Baseline at yMin of the zoomed scale (may be > 0 when scores are clustered high).
  const baselineY = yToSvg(yMin);
  let html = '';
  html += `<line x1="${padL}" y1="${baselineY}" x2="${W - padR}" y2="${baselineY}" stroke="${colors.border}" stroke-width="1"/>`;
  for (let i = 0; i < 10; i++) {
    const v = values[i];
    if (v == null || Number.isNaN(v)) continue;
    const barH = Math.max(1, ((v - yMin) / ySpan) * plotH);
    const x = padL + i * slotW + slotW * 0.15;
    const w = slotW * 0.7;
    const y = yToSvg(v);
    const fill = shotColors[i % shotColors.length] || colors.bar;
    html += `<rect x="${x}" y="${y}" width="${w}" height="${barH}" fill="${fill}"/>`;
  }
  for (let i = 1; i <= 10; i++) {
    const x = padL + (i - 0.5) * slotW;
    html += `<text x="${x}" y="${H - 3}" text-anchor="middle" fill="${colors.font}" font-size="${fontSize}">${i}</text>`;
  }
  html += `<text x="3" y="${padT + fontSize}" fill="${colors.font}" font-size="${fontSize}">${yMax.toFixed(1)}</text>`;
  html += `<text x="3" y="${padT + plotH}" fill="${colors.font}" font-size="${fontSize}">${yMin.toFixed(1)}</text>`;
  svg.innerHTML = html;
}

function footerItem(label, value, visible) {
  if (!visible) return '';
  return `<span class="footer-item"><span class="label">${label}:</span><span class="value">${value}</span></span>`;
}

function footerItemTwoLines(label, line1, line2, visible) {
  if (!visible) return '';
  return `<span class="footer-item footer-item-twolines"><span class="label">${label}:</span><span class="value"><span class="value-line">${line1}</span><span class="value-line">${line2}</span></span></span>`;
}

function renderFooter(rangeData) {
  const f = config.footer || {};
  const w = rangeData.currentValue != null && f.currentShotValue ? rangeData.currentValue.toFixed(1) : '–';
  const t = rangeData.currentTeiler != null && f.teiler ? rangeData.currentTeiler.toFixed(1) : '–';
  const sumInt = f.overallSumInt || f.overallSumDecimal ? String(rangeData.overallSumInt ?? 0) : '–';
  const sumDec = f.overallSumInt || f.overallSumDecimal ? (rangeData.overallSumDecimal ?? 0).toFixed(1) : '–';
  const hasPred = (rangeData.predictionInt != null && rangeData.predictionInt > 0) || (rangeData.predictionDecimal != null && rangeData.predictionDecimal > 0);
  const predInt = f.predictionInt || f.predictionDecimal ? (hasPred ? String(rangeData.predictionInt ?? 0) : '–') : '–';
  const predDec = f.predictionInt || f.predictionDecimal ? (hasPred ? (rangeData.predictionDecimal ?? 0).toFixed(1) : '–') : '–';
  const sharp = f.shotNumber ? (rangeData.shotNumber ?? '–') : '–';

  const thLabel = (label) => '<th class="footer-th footer-th-label"><span class="footer-label">' + label + ':</span></th>';
  const thValue = (val) => '<th class="footer-th footer-th-value"><span class="footer-value">' + val + '</span></th>';
  const thValueLeft = (val) => '<th class="footer-th footer-th-value footer-value-left"><span class="footer-value">' + val + '</span></th>';
  const tdLabel = (label) => '<td class="footer-td footer-td-label"><span class="footer-label">' + (label ? label + ':' : '') + '</span></td>';
  const tdValue = (val) => '<td class="footer-td footer-td-value"><span class="footer-value">' + val + '</span></td>';
  const tdValueLeft = (val) => '<td class="footer-td footer-td-value footer-value-left"><span class="footer-value">' + val + '</span></td>';

  let serienHtml = '';
  if (f.seriesSumsInt || f.seriesSumsDecimal) {
    const ints = rangeData.seriesSumsInt || [];
    const decs = rangeData.seriesSums || [];
    const n = Math.max(ints.length, decs.length);
    const showInt = !!f.seriesSumsInt;
    const showDec = !!f.seriesSumsDecimal;
    let rows = '';
    if (n === 0) {
      rows = '<span class="serien-row serien-idx"><span class="serien-cell">–</span></span>';
      if (showInt) rows += '<span class="serien-row serien-ints"><span class="serien-cell">–</span></span>';
      if (showDec) rows += '<span class="serien-row serien-decs"><span class="serien-cell">–</span></span>';
    } else {
      const idxCells = [];
      for (let i = 1; i <= n; i++) idxCells.push('<span class="serien-cell">' + i + '</span>');
      rows += '<span class="serien-row serien-idx">' + idxCells.join('') + '</span>';
      if (showInt) {
        const cells = [];
        for (let i = 0; i < n; i++) {
          const v = ints[i] != null ? String(ints[i]) : '–';
          // Invisible fractional slot so ints line up with floats at the decimal point.
          const pad = showDec && v !== '–' ? '<span class="serien-frac-slot" aria-hidden="true">.0</span>' : '';
          cells.push('<span class="serien-cell">' + v + pad + '</span>');
        }
        rows += '<span class="serien-row serien-ints">' + cells.join('') + '</span>';
      }
      if (showDec) {
        const cells = [];
        for (let i = 0; i < n; i++) {
          const v = decs[i] != null ? Number(decs[i]).toFixed(1) : '–';
          cells.push('<span class="serien-cell">' + v + '</span>');
        }
        rows += '<span class="serien-row serien-decs">' + cells.join('') + '</span>';
      }
    }
    serienHtml =
      '<div class="footer-serien-row' + (n > 4 ? ' serien-many' : '') + '">' +
      '<span class="footer-label">Serien:</span>' +
      '<span class="serien-grid">' + rows + '</span>' +
      '</div>';
  }

  const innerTableWithValues =
    '<table class="footer-inner-table"><thead><tr>' +
    thLabel('Wert') + thValue(w) + thLabel('Teiler') + thValue(t) + thLabel('Summe') + thValueLeft(sumInt) + thLabel('Prognose') + thValueLeft(predInt) +
    '</tr></thead><tbody><tr>' +
    tdLabel('#') + tdValue(sharp) + tdLabel('') + tdValue('') + tdLabel('') + tdValueLeft(sumDec) + tdLabel('') + tdValueLeft(predDec) +
    '</tr></tbody></table>';

  return (
    '<div class="footer-stack">' +
    innerTableWithValues +
    serienHtml +
    '</div>'
  );
}

function formatRangeHeader(r) {
  const line1 = r.shooterName || ('Stand ' + r.rangeNum + ' – kein Schütze');
  const line2 = r.clubName || '';
  const line3 = r.discipline || '';
  const stand = 'Stand ' + r.rangeNum;
  return { line1, line2, line3, stand };
}

function formatShotChip(r) {
  const val = r.currentValue != null ? Number(r.currentValue).toFixed(1) : '–';
  const n = r.shotNumber != null ? r.shotNumber : (r.shots && r.shots.length) || '–';
  const teiler = r.currentTeiler != null ? Number(r.currentTeiler).toFixed(1) : '';
  return {
    val: val,
    meta: '#' + n + (teiler !== '' ? ' · T ' + teiler : '') + (r.isWarmup ? ' · Probe' : '')
  };
}

function rangeChromeSignature(r) {
  const shots = r.shots || [];
  const last = shots.length ? shots[shots.length - 1] : null;
  const seriesN = Math.max(
    (r.seriesSumsInt || []).length,
    (r.seriesSums || []).length
  );
  return [
    r.rangeNum,
    r.shotNumber,
    shots.length,
    last ? `${last.x}:${last.y}:${last.decValue}` : '',
    r.overallSumInt,
    r.overallSumDecimal,
    r.currentValue,
    r.currentTeiler,
    r.predictionInt,
    r.predictionDecimal,
    r.isWarmup ? 1 : 0,
    r.shooterName || '',
    r.clubName || '',
    r.discipline || '',
    (r.last10Values || []).join(','),
    seriesN,
    getShotStrokeWidth()
  ].join('|');
}

function fillRangeHeader(header, rangeData) {
  header.className = 'range-header' + (rangeData.shooterName ? '' : ' empty');
  const h = formatRangeHeader(rangeData);
  const chip = formatShotChip(rangeData);

  let top = header.querySelector('.range-header-top');
  let metaRow = header.querySelector('.range-header-meta-row');
  let line3 = header.querySelector(':scope > .range-header-line3');

  if (!top || !metaRow || !line3) {
    header.innerHTML = '';
    top = document.createElement('div');
    top.className = 'range-header-top';
    top.innerHTML =
      '<div class="range-header-text"><div class="range-header-line1"></div></div>' +
      '<div class="range-shot-chip"><span class="shot-val"></span></div>';
    header.appendChild(top);

    metaRow = document.createElement('div');
    metaRow.className = 'range-header-meta-row';
    metaRow.innerHTML = '<span class="range-club"></span><span class="shot-meta"></span>';
    header.appendChild(metaRow);

    line3 = document.createElement('div');
    line3.className = 'range-header-line3';
    line3.innerHTML = '<span class="range-discipline"></span><span class="range-stand"></span>';
    header.appendChild(line3);
  }

  header.querySelector('.range-sum-row')?.remove();

  const line1 = header.querySelector('.range-header-line1');
  const clubEl = metaRow.querySelector('.range-club');
  const discEl = line3.querySelector('.range-discipline');
  const standEl = line3.querySelector('.range-stand');
  const shotVal = header.querySelector('.shot-val');
  const shotMeta = metaRow.querySelector('.shot-meta');

  if (line1) line1.textContent = h.line1;
  if (clubEl) {
    clubEl.textContent = h.line2;
    clubEl.hidden = !h.line2;
  }
  metaRow.hidden = !h.line2 && !chip.meta;
  if (discEl) discEl.textContent = h.line3;
  if (standEl) standEl.textContent = h.stand;
  line3.hidden = false;
  if (shotVal) shotVal.textContent = chip.val;
  if (shotMeta) shotMeta.textContent = chip.meta;
}

function renderRangePanel(rangeData) {
  const panel = document.createElement('div');
  panel.className = 'range-panel';
  panel.dataset.range = rangeData.rangeNum;

  const header = document.createElement('div');
  fillRangeHeader(header, rangeData);
  panel.appendChild(header);

  const targetContainer = document.createElement('div');
  targetContainer.className = 'range-target' + (rangeData.isWarmup ? ' warmup' : '');
  panel.appendChild(targetContainer);

  const f = config.footer || {};
  const showLast10 = f.last10Int || f.last10Decimal;
  if (showLast10) {
    const chartWrap = document.createElement('div');
    chartWrap.className = 'last10-chart-wrap';
    panel.appendChild(chartWrap);
  }

  const footer = document.createElement('div');
  footer.className = 'range-footer';
  footer.innerHTML = renderFooter(rangeData);
  footer.dataset.seriesN = String(Math.max(
    (rangeData.seriesSumsInt || []).length,
    (rangeData.seriesSums || []).length
  ));
  panel.appendChild(footer);

  renderTarget(targetContainer, rangeData, rangeData.isWarmup);
  if (showLast10) {
    const chartWrap = panel.querySelector('.last10-chart-wrap');
    if (chartWrap) renderLast10Chart(chartWrap, rangeData.last10Values);
  }
  panel.dataset.chromeSig = rangeChromeSignature(rangeData);

  return panel;
}

function renderClassicRangeView(container, rangeData) {
  if (!container || !rangeData) return;

  let targetEl = container.querySelector('.range-target');
  if (!targetEl) {
    targetEl = document.createElement('div');
    targetEl.className = 'range-target';
    container.appendChild(targetEl);
  }
  targetEl.classList.toggle('warmup', rangeData.isWarmup);

  const f = config.footer || {};
  const showLast10 = f.last10Int || f.last10Decimal;
  let chartWrap = container.querySelector('.last10-chart-wrap');
  if (showLast10) {
    if (!chartWrap) {
      chartWrap = document.createElement('div');
      chartWrap.className = 'last10-chart-wrap';
      const footerEl = container.querySelector('.range-footer');
      if (footerEl) container.insertBefore(chartWrap, footerEl);
      else container.appendChild(chartWrap);
    }
    renderLast10Chart(chartWrap, rangeData.last10Values);
  } else if (chartWrap) {
    chartWrap.remove();
  }

  let footerEl = container.querySelector('.range-footer');
  if (!footerEl) {
    footerEl = document.createElement('div');
    footerEl.className = 'range-footer';
    container.appendChild(footerEl);
  }
  const seriesN = Math.max(
    (rangeData.seriesSumsInt || []).length,
    (rangeData.seriesSums || []).length
  );
  footerEl.innerHTML = renderFooter(rangeData);
  footerEl.dataset.seriesN = String(seriesN);
  renderTarget(targetEl, rangeData, rangeData.isWarmup);
}

function syncRangePanel(panel, r) {
  if (!panel || !r) return;
  const sig = rangeChromeSignature(r);
  if (panel.dataset.chromeSig === sig) return;
  panel.dataset.chromeSig = sig;

  const header = panel.querySelector('.range-header');
  const targetEl = panel.querySelector('.range-target');
  const footerEl = panel.querySelector('.range-footer');
  const f = config.footer || {};
  const showLast10 = f.last10Int || f.last10Decimal;
  if (header) fillRangeHeader(header, r);
  if (targetEl) targetEl.classList.toggle('warmup', r.isWarmup);

  const seriesN = Math.max(
    (r.seriesSumsInt || []).length,
    (r.seriesSums || []).length
  );
  if (footerEl) {
    footerEl.innerHTML = renderFooter(r);
    footerEl.dataset.seriesN = String(seriesN);
  }
  if (showLast10) {
    let chartWrap = panel.querySelector('.last10-chart-wrap');
    if (!chartWrap) {
      chartWrap = document.createElement('div');
      chartWrap.className = 'last10-chart-wrap';
      panel.insertBefore(chartWrap, footerEl);
    }
    renderLast10Chart(chartWrap, r.last10Values);
  } else {
    const chartWrap = panel.querySelector('.last10-chart-wrap');
    if (chartWrap) chartWrap.remove();
  }
  renderTarget(targetEl, r, r.isWarmup);
}

/** Empty range shell for plugin-hosted UI: header + .range-plugin-view mount. */
function ensurePluginPanels(numRanges) {
  const grid = document.getElementById('ranges-grid');
  if (!grid) return;
  const n = Math.max(1, numRanges || (config && config.ranges) || 1);
  const keep = new Set();
  for (let i = 1; i <= n; i++) {
    keep.add(i);
    let panel = grid.querySelector(`.range-panel[data-range="${i}"]`);
    if (!panel) {
      panel = document.createElement('div');
      panel.className = 'range-panel plugin-hosted';
      panel.dataset.range = String(i);
      const header = document.createElement('div');
      header.className = 'range-header';
      fillRangeHeader(header, { rangeNum: i });
      panel.appendChild(header);
      const mount = document.createElement('div');
      mount.className = 'range-plugin-view';
      panel.appendChild(mount);
      grid.appendChild(panel);
    } else if (!panel.querySelector('.range-plugin-view')) {
      const mount = document.createElement('div');
      mount.className = 'range-plugin-view';
      panel.appendChild(mount);
    }
  }
  grid.querySelectorAll('.range-panel').forEach((panel) => {
    const num = parseInt(panel.dataset.range, 10);
    if (!keep.has(num)) panel.remove();
  });
}

function updatePluginPanelHeader(rangeNum, rangeData) {
  const grid = document.getElementById('ranges-grid');
  if (!grid) return;
  const panel = grid.querySelector(`.range-panel[data-range="${rangeNum}"]`);
  if (!panel) return;
  const header = panel.querySelector('.range-header');
  if (header) fillRangeHeader(header, rangeData || { rangeNum: rangeNum });
}

function render(data) {
  lastLiveData = data;
  const grid = document.getElementById('ranges-grid');
  if (!grid || !data) return;

  // Prefer config.ranges so empty stands still appear; merge live payloads into them.
  const configured = Math.max(1, (config && config.ranges) || (data.ranges && data.ranges.length) || 1);
  ensurePluginPanels(configured);

  const byNum = {};
  (data.ranges || []).forEach((r) => { byNum[r.rangeNum] = r; });
  for (let i = 1; i <= configured; i++) {
    updatePluginPanelHeader(i, byNum[i] || { rangeNum: i });
  }
}

async function poll() {
  try {
    const data = await fetchLive();
    if (data) render(data);
  } catch (e) {
    console.warn('Poll failed:', e);
  }
}

window.SRCore = {
  POLL_INTERVAL_MS,
  SAFETY_POLL_MS,
  get config() { return config; },
  get lastLiveData() { return lastLiveData; },
  set lastLiveData(v) { lastLiveData = v; },
  bumpLiveGen,
  getLiveGen,
  fetchConfig,
  fetchLive,
  applyLayout,
  render,
  ensurePluginPanels,
  updatePluginPanelHeader,
  renderRangePanel,
  syncRangePanel,
  renderClassicRangeView,
  renderTarget,
  renderFooter,
  formatRangeHeader,
  getShotOrderColors,
  getTargetScale,
  setTargetAssetBase,
  dsgToSvg
};
