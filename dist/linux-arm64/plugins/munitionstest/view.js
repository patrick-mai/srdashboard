(function () {
  const PLUGIN_ID = 'munitionstest';
  const CLASSIC_REGISTRY = '/plugins/classic-range/target-registry.js';
  const CLASSIC_ASSETS = '/plugins/classic-range/assets';

  window.SRPlugins = window.SRPlugins || {};
  window.SRPluginViews = window.SRPluginViews || {};

  const DISC = {
    Luftgewehr: 'air_rifle_10m',
    LG: 'air_rifle_10m',
    Luftpistole: 'air_pistol_10m',
    LP: 'air_pistol_10m',
    Kleinkaliber: 'smallbore_50m_prone',
    'KK-Gewehr': 'smallbore_50m_prone',
    KK: 'smallbore_50m_prone'
  };

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function mm(n) {
    const v = Number(n);
    if (!isFinite(v)) return '–';
    return v.toFixed(1).replace('.', ',') + ' mm';
  }

  function mm3(n) {
    const v = Number(n);
    if (!isFinite(v)) return '–';
    const text = Math.abs(v).toFixed(3).replace('.', ',');
    if (v < 0) return '−' + text + ' mm';
    if (v > 0) return '+' + text + ' mm';
    return text + ' mm';
  }

  function teilerText(n) {
    const v = Number(n);
    if (!isFinite(v)) return '–';
    return v.toFixed(1).replace('.', ',');
  }

  function groupText(row) {
    if (!row || !row.hasGroup) return '–';
    if (isFinite(Number(row.teiler))) return mm3(Number(row.teiler) / 100).replace(/^[+−]/, '') + ' · Teiler ' + teilerText(row.teiler);
    return mm(row.diameterMm);
  }

  function shotRows(shots) {
    if (!shots || !shots.length) return '';
    const body = shots.map(function (s, i) {
      return '<tr><td>' + (i + 1) + '</td><td>' + mm3(s.dxMm) + '</td><td>' + mm3(s.dyMm) + '</td><td>' + teilerText(s.teiler) + '</td></tr>';
    }).join('');
    return '<table class="mt-shot-table"><thead><tr><th>Schuss</th><th>X</th><th>Y</th><th>Teiler</th></tr></thead><tbody>' +
      body + '</tbody></table>';
  }

  function canControl() {
    return !(window.SRMode && window.SRMode.canControl === false);
  }

  function send(action, params) {
    const body = JSON.stringify({ action: action, params: params || {} });
    const opts = { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body };
    const fetchFn = (window.SRAuth && window.SRAuth.fetchWithAuth) ? window.SRAuth.fetchWithAuth : fetch;
    return fetchFn('/api/plugins/control', opts).then(function (res) {
      if (!res.ok) return res.text().then(function (t) { window.alert(t || 'Fehler'); });
    });
  }

  function bind(container) {
    if (container._mtBound) return;
    container._mtBound = true;
    container.addEventListener('submit', function (ev) {
      const form = ev.target;
      if (!form || !form.classList || !form.classList.contains('mt-add')) return;
      ev.preventDefault();
      const input = form.querySelector('input[name="lot"]');
      const name = input ? input.value.trim() : '';
      const vm = container._mtVM || {};
      const rangeNum = Number(form.getAttribute('data-range') || vm.rangeNum || 1);
      send('addLot', {
        name: name,
        rangeNum: rangeNum,
        select: form.getAttribute('data-select') === '1'
      });
      if (input) input.value = '';
    });
    container.addEventListener('click', function (ev) {
      if (ev.target.closest && ev.target.closest('#mt-print')) {
        window.print();
        return;
      }
      const btn = ev.target.closest && ev.target.closest('[data-action]');
      if (btn && container.contains(btn)) {
        const action = btn.getAttribute('data-action');
        if (!action || action === 'setWeapon') return;
        const vm = container._mtVM || {};
        const rangeNum = Number(btn.getAttribute('data-range') || vm.rangeNum || 1);
        if (action === 'selectLot') {
          send('selectLot', { rangeNum: rangeNum, lotId: Number(btn.getAttribute('data-lot') || 0) });
        } else if (action === 'removeLot') {
          send('removeLot', { rangeNum: rangeNum, lotId: Number(btn.getAttribute('data-lot') || 0) });
        } else if (action === 'abort') {
          send('abort', { rangeNum: rangeNum });
        } else if (action === 'undo') {
          send('undo', { rangeNum: rangeNum });
        }
        return;
      }
      const row = ev.target.closest && ev.target.closest('tr[data-pick]');
      if (!row || !container.contains(row) || ev.target.closest('input, textarea, button')) return;
      container._mtPick = row.getAttribute('data-pick');
      paintMaster(container, container._mtVM || {});
    });
    container.addEventListener('change', function (ev) {
      const el = ev.target;
      if (!el || el.getAttribute('data-action') !== 'setWeapon') return;
      send('setWeapon', {
        rangeNum: Number(el.getAttribute('data-range')),
        lotId: Number(el.getAttribute('data-lot') || 0),
        weapon: el.value
      });
    });
  }

  function ensureRegistry() {
    if (window.SRTargetRegistry && window.SRTargetRegistry.buildScale) {
      return Promise.resolve();
    }
    return new Promise(function (resolve) {
      const s = document.createElement('script');
      s.src = CLASSIC_REGISTRY + '?t=' + Date.now();
      s.onload = function () { resolve(); };
      s.onerror = function () { resolve(); };
      document.head.appendChild(s);
    });
  }

  function scaleFor(range) {
    const reg = window.SRTargetRegistry;
    if (!reg || !reg.buildScale || !reg.getProfile) return null;
    const key = (range && (range.discipline || range.discType)) || '';
    return reg.buildScale(reg.getProfile(DISC[key] || 'air_rifle_10m'));
  }

  function dsgToSvg(scale, x, y) {
    return {
      x: scale.centerX + (Number(x) || 0) / scale.dsgPerSvgUnit,
      y: scale.centerY - (Number(y) || 0) / scale.dsgPerSvgUnit
    };
  }

  function captureDrafts(container) {
    const saved = [];
    const nodes = container.querySelectorAll('.mt-draft');
    for (let i = 0; i < nodes.length; i++) {
      if (document.activeElement === nodes[i]) {
        saved.push({ id: nodes[i].id, value: nodes[i].value, sel: nodes[i].selectionStart });
      }
    }
    return saved;
  }

  function restoreDrafts(container, saved) {
    saved.forEach(function (s) {
      if (!s.id || !window.CSS || !CSS.escape) return;
      const el = container.querySelector('#' + CSS.escape(s.id));
      if (!el) return;
      el.value = s.value;
      el.focus();
      if (el.setSelectionRange && s.sel != null) el.setSelectionRange(s.sel, s.sel);
    });
  }

  function lotChips(lots, activeId, rangeNum) {
    if (!lots || !lots.length) {
      return '<p class="mt-muted">Noch keine Lose. Auf dem Master anlegen oder hier ergänzen.</p>';
    }
    if (!canControl()) {
      return '<div class="mt-chips">' + lots.map(function (lot) {
        const on = Number(lot.id) === Number(activeId);
        return '<span class="mt-chip' + (on ? ' is-on' : '') + '">' + esc(lot.name) + '</span>';
      }).join('') + '</div>';
    }
    return '<div class="mt-chips">' + lots.map(function (lot) {
      const on = Number(lot.id) === Number(activeId);
      return '<span class="mt-chip-wrap">' +
        '<button type="button" class="mt-chip' + (on ? ' is-on' : '') + '" data-action="selectLot" data-lot="' +
        esc(lot.id) + '" data-range="' + esc(rangeNum) + '">' + esc(lot.name) + '</button>' +
        '<button type="button" class="mt-chip-x" data-action="removeLot" data-lot="' + esc(lot.id) +
        '" data-range="' + esc(rangeNum) + '" aria-label="Los löschen">' + iconTrash() + '</button></span>';
    }).join('') + '</div>';
  }

  function lage(active) {
    if (!active || !active.hasCenter) return '';
    const x = Number(active.centerMmX) || 0;
    const y = Number(active.centerMmY) || 0;
    const h = (x >= 0 ? mm(x) + ' rechts' : mm(-x) + ' links');
    const v = (y >= 0 ? mm(y) + ' hoch' : mm(-y) + ' tief');
    return 'Schuss 1 sitzt ' + h + ', ' + v;
  }

  function abortHtml(active, rangeNum, inline) {
    if (!active || !active.hasAbort || !canControl()) return '';
    const steps = Number(active.abortSteps) || 0;
    const scale = Number(active.abortScale) || 1;
    let note = '';
    if (steps > 0 && active.hasBest) {
      note = '<p class="mt-abort-note">Größer als ' + esc(active.bestName) + ' (' +
        (isFinite(Number(active.bestTeiler)) ? groupText({ hasGroup: true, teiler: active.bestTeiler }) : mm(active.bestMm)) + ')</p>';
    }
    return '<div class="mt-abort-wrap">' + note +
      '<button type="button" class="btn mt-abort' + (inline ? ' mt-abort-inline' : '') + (steps > 0 ? ' is-worse' : '') +
      '" data-action="abort" data-range="' + esc(rangeNum) + '" data-steps="' + steps +
      '" style="--mt-abort-scale:' + scale + '">Abbruch</button></div>';
  }

  function statusText(row) {
    if (row.full) return 'fertig';
    if (row.closed) return 'abgebrochen';
    if (row.open) return 'läuft';
    return '';
  }

  // Numbers sit outside the group. A tight lot would otherwise stack every
  // digit on the same few tenths of a millimetre.
  function spreadLabels(origin, holes, font) {
    const gap = font * 1.45;
    const placed = [];
    return holes.map(function (hole, idx) {
      const vx = hole.x - origin.x;
      const vy = hole.y - origin.y;
      const rad = Math.hypot(vx, vy);
      const baseAng = rad < font * 0.25
        ? (-Math.PI / 2) + idx * (2 * Math.PI / Math.max(holes.length, 1))
        : Math.atan2(vy, vx);
      const baseDist = rad + font * 1.7;
      let x = origin.x + Math.cos(baseAng) * baseDist;
      let y = origin.y + Math.sin(baseAng) * baseDist;
      function clear(px, py) {
        for (let i = 0; i < placed.length; i++) {
          if (Math.hypot(placed[i].x - px, placed[i].y - py) < gap) return false;
        }
        for (let i = 0; i < holes.length; i++) {
          if (Math.hypot(holes[i].x - px, holes[i].y - py) < font * 0.95) return false;
        }
        if (Math.hypot(origin.x - px, origin.y - py) < font * 0.9) return false;
        return true;
      }
      for (let n = 0; n < 24 && !clear(x, y); n++) {
        const step = Math.floor(n / 2) + 1;
        const spin = (n % 2 === 0 ? 1 : -1) * step * 0.32;
        const dist = baseDist + step * font * 0.45;
        x = origin.x + Math.cos(baseAng + spin) * dist;
        y = origin.y + Math.sin(baseAng + spin) * dist;
      }
      const spot = { x: x, y: y, hole: hole };
      placed.push(spot);
      return spot;
    });
  }

  function paintScheibe(host, scale, shots, diameterMm) {
    if (!host || !scale) return Promise.resolve();
    const file = scale.file || '10_m_Air_Rifle_target.svg';
    const url = CLASSIC_ASSETS.replace(/\/?$/, '/') + file;
    const have = host.dataset.file === file && host.querySelector('svg');
    const load = have ? Promise.resolve() : fetch(url).then(function (res) { return res.text(); }).then(function (text) {
      host.dataset.file = file;
      host.innerHTML = text;
      const svg = host.querySelector('svg');
      if (!svg) return;
      svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
      svg.removeAttribute('width');
      svg.removeAttribute('height');
      svg.classList.add('mt-scheibe-svg');
    }).catch(function () {
      host.innerHTML = '<p class="mt-muted">Scheibe nicht ladbar</p>';
    });
    return load.then(function () {
      const svg = host.querySelector('svg');
      if (!svg) return;
      let g = svg.querySelector('#mt-shots');
      if (!g) {
        g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        g.setAttribute('id', 'mt-shots');
        svg.appendChild(g);
      }
      g.innerHTML = '';
      const list = shots || [];
      const ring = Number(scale.ringWidthMm) || 2.5;
      let reach = 0;
      if (list.length) {
        const c0 = dsgToSvg(scale, list[0].x, list[0].y);
        reach = Math.hypot(c0.x - scale.centerX, c0.y - scale.centerY);
        list.forEach(function (s) {
          const pt = dsgToSvg(scale, s.x, s.y);
          const d = Math.hypot(pt.x - scale.centerX, pt.y - scale.centerY);
          if (d > reach) reach = d;
        });
        if (isFinite(Number(diameterMm))) {
          const edge = Math.hypot(c0.x - scale.centerX, c0.y - scale.centerY) + Number(diameterMm) / 2;
          if (edge > reach) reach = edge;
        }
      }
      const frame = Math.max(ring, reach + Math.max(ring * 0.15, reach * 0.35));
      const font = Math.max(0.22, frame * 0.11);
      const dot = Math.max(0.05, frame * 0.02);
      const origin = list.length ? dsgToSvg(scale, list[0].x, list[0].y) : { x: scale.centerX, y: scale.centerY };
      const holes = [];
      for (let i = 1; i < list.length; i++) holes.push(dsgToSvg(scale, list[i].x, list[i].y));
      const spots = spreadLabels(origin, holes, font);
      let edge = reach;
      spots.forEach(function (s) {
        const d = Math.hypot(s.x - scale.centerX, s.y - scale.centerY) + font;
        if (d > edge) edge = d;
      });
      const half = Math.max(frame, edge + font * 0.6);
      const cx = scale.centerX;
      const cy = scale.centerY;
      svg.setAttribute('viewBox', (cx - half) + ' ' + (cy - half) + ' ' + (half * 2) + ' ' + (half * 2));
      if (list.length >= 2 && isFinite(Number(diameterMm))) {
        const c0 = dsgToSvg(scale, list[0].x, list[0].y);
        const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        circle.setAttribute('cx', String(c0.x));
        circle.setAttribute('cy', String(c0.y));
        circle.setAttribute('r', String(Number(diameterMm) / 2));
        circle.setAttribute('stroke-width', String(Math.max(0.02, half * 0.012)));
        circle.setAttribute('class', 'mt-group');
        g.appendChild(circle);
      }
      list.forEach(function (s, i) {
        const pt = dsgToSvg(scale, s.x, s.y);
        if (i === 0) {
          const mark = document.createElementNS('http://www.w3.org/2000/svg', 'path');
          const d = dot * 2.4;
          mark.setAttribute('d', 'M ' + (pt.x - d) + ' ' + pt.y + ' H ' + (pt.x + d) + ' M ' + pt.x + ' ' + (pt.y - d) + ' V ' + (pt.y + d));
          mark.setAttribute('stroke-width', String(Math.max(0.02, half * 0.012)));
          mark.setAttribute('class', 'mt-centre');
          g.appendChild(mark);
          return;
        }
        const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        c.setAttribute('cx', String(pt.x));
        c.setAttribute('cy', String(pt.y));
        c.setAttribute('r', String(dot));
        c.setAttribute('stroke-width', String(Math.max(0.015, dot * 0.25)));
        c.setAttribute('class', 'mt-hole');
        g.appendChild(c);
      });
      const stroke = String(Math.max(0.015, font * 0.06));
      spots.forEach(function (s, i) {
        const ang = Math.atan2(s.y - s.hole.y, s.x - s.hole.x);
        const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
        line.setAttribute('x1', String(s.hole.x + Math.cos(ang) * dot * 1.6));
        line.setAttribute('y1', String(s.hole.y + Math.sin(ang) * dot * 1.6));
        line.setAttribute('x2', String(s.x - Math.cos(ang) * font * 0.55));
        line.setAttribute('y2', String(s.y - Math.sin(ang) * font * 0.55));
        line.setAttribute('stroke-width', stroke);
        line.setAttribute('class', 'mt-leader');
        g.appendChild(line);
        const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        label.setAttribute('x', String(s.x));
        label.setAttribute('y', String(s.y));
        label.setAttribute('font-size', String(font));
        label.setAttribute('class', 'mt-hole-no');
        label.textContent = String(i + 2);
        g.appendChild(label);
      });
    });
  }

  function ensureLane(container, rangeNum) {
    if (container.dataset.mtSurface === 'lane') {
      const form = container.querySelector('form.mt-add');
      if (form) form.setAttribute('data-range', String(rangeNum));
      return;
    }
    container.dataset.mtSurface = 'lane';
    container.className = 'range-plugin-view mt-lane';
    container.innerHTML =
      '<div class="mt-lane-body">' +
      '<div class="mt-lane-bar">' +
      '<div id="mt-lots"></div>' +
      '<form class="mt-add" data-select="1" data-range="' + esc(rangeNum) + '">' +
      '<input class="mt-draft" id="mt-new-lot" name="lot" placeholder="Los ergänzen" autocomplete="off">' +
      '<button type="submit" class="mt-icon-btn" aria-label="Los anlegen">' + iconPlus() + '</button></form></div>' +
      '<div class="mt-lane-main">' +
      '<div class="mt-scheibe" id="mt-scheibe"></div>' +
      '<aside class="mt-side">' +
      '<div id="mt-stats"></div>' +
      '<div id="mt-abort"></div>' +
      '<div id="mt-actions"></div>' +
      '</aside></div></div>';
  }

  function paintLane(container, vm) {
    const gen = (container._mtGen = (container._mtGen || 0) + 1);
    const rangeNum = vm.rangeNum || 1;
    ensureLane(container, rangeNum);
    const form = container.querySelector('form.mt-add');
    if (form) form.hidden = !canControl();
    return ensureRegistry().then(function () {
      if (container._mtGen !== gen) return;
      const lots = container.querySelector('#mt-lots');
      if (lots) lots.innerHTML = lotChips(vm.lots, vm.activeLotId, rangeNum);
      const active = vm.active;
      const stats = container.querySelector('#mt-stats');
      if (stats) {
        if (!active) {
          stats.innerHTML = '<p class="mt-muted">Los wählen, dann schießen. Der erste Schuss legt das Zentrum fest.</p>';
        } else {
          const group = groupText(active);
          stats.innerHTML = '<p class="mt-stats-line"><strong>' + esc(active.name) + '</strong> · ' +
            esc(active.shotCount) + '/' + esc(vm.shotsPer) + ' · Streukreis ' + group + '</p>' +
            '<p class="mt-muted">' + esc(lage(active)) + '</p>' +
            shotRows(active.shots);
        }
      }
      const abort = container.querySelector('#mt-abort');
      if (abort) abort.innerHTML = abortHtml(active, rangeNum, false);
      const actions = container.querySelector('#mt-actions');
      if (actions) {
        actions.innerHTML = (canControl() && active && active.shotCount > 0)
          ? '<button type="button" class="btn" data-action="undo" data-range="' + esc(rangeNum) + '">Letzten Schuss zurück</button>'
          : '';
      }
      return paintScheibe(container.querySelector('#mt-scheibe'), scaleFor(vm.range), active && active.shots, active && active.diameterMm);
    });
  }

  function rowsOf(lanes) {
    const rows = [];
    const used = {};
    (lanes || []).forEach(function (lane) {
      (lane.results || []).forEach(function (r) {
        used[r.lotId] = true;
        rows.push({
          rangeNum: lane.rangeNum,
          shotsPer: lane.shotsPer,
          row: r,
          active: lane.active && Number(lane.active.lotId) === Number(r.lotId) ? lane.active : null,
          pending: false
        });
      });
    });
    const seen = {};
    (lanes || []).forEach(function (lane) {
      (lane.lots || []).forEach(function (lot) {
        if (seen[lot.id] || used[lot.id]) return;
        seen[lot.id] = true;
        rows.push({
          rangeNum: '–',
          shotsPer: lane.shotsPer,
          pending: true,
          active: null,
          row: { lotId: lot.id, name: lot.name, shotCount: 0, weapon: '', hasGroup: false }
        });
      });
    });
    return rows;
  }

  function pickKey(rangeNum, lotId) {
    return String(rangeNum) + ':' + String(lotId);
  }

  function detailSource(item) {
    if (item.active && Number(item.active.lotId) === Number(item.row.lotId)) return item.active;
    return item.row;
  }

  function chosenItem(container, rows) {
    const key = container._mtPick;
    for (let i = 0; i < rows.length; i++) {
      if (pickKey(rows[i].rangeNum, rows[i].row.lotId) === key) return rows[i];
    }
    for (let i = 0; i < rows.length; i++) {
      if (!rows[i].pending && rows[i].row.hasGroup) return rows[i];
    }
    return rows[0] || null;
  }

  function lotHeading(item) {
    const r = detailSource(item);
    const status = item.pending ? 'bereit' : statusText(r);
    return '<h3>' + esc(r.name || '') + '</h3>' +
      '<p class="mt-muted">Bahn ' + esc(item.rangeNum) +
      ' · Waffe ' + esc(r.weapon || '–') +
      ' · ' + (item.pending ? '–' : esc(r.shotCount) + '/' + esc(item.shotsPer)) +
      ' · Streukreis ' + groupText(r) +
      (status ? ' · ' + esc(status) : '') + '</p>';
  }

  function printListHtml(rows) {
    const body = rows.map(function (item) {
      const r = detailSource(item);
      const shots = r.shots && r.shots.length ? shotRows(r.shots) : '<p class="mt-muted">Keine Schüsse.</p>';
      return '<section class="mt-print-lot">' + lotHeading(item) + shots + '</section>';
    }).join('');
    return '<div class="mt-print-head"><h2>Gesamtliste</h2>' +
      '<button type="button" class="btn" id="mt-print">Drucken</button></div>' +
      (body || '<p class="mt-muted">Noch keine Lose.</p>');
  }

  function iconPlus() {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>';
  }

  function iconTrash() {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16M9 7V5h6v2M7 7l1 13h8l1-13M10 11v6M14 11v6"/></svg>';
  }

  function removeLotButton(lotId, rangeNum) {
    if (!canControl()) return '';
    const rangeAttr = isFinite(Number(rangeNum)) ? ' data-range="' + esc(rangeNum) + '"' : '';
    return '<button type="button" class="mt-icon-btn mt-icon-quiet mt-delete" data-action="removeLot" data-lot="' +
      esc(lotId) + '"' + rangeAttr + ' aria-label="Los löschen">' + iconTrash() + '</button>';
  }

  function weaponCell(rangeNum, row, pending) {
    if (pending) return '–';
    if (!canControl()) return esc(row.weapon || '');
    return '<input class="mt-draft mt-weapon" id="mt-w-' + esc(rangeNum) + '-' + esc(row.lotId) +
      '" data-action="setWeapon" data-range="' + esc(rangeNum) + '" data-lot="' + esc(row.lotId) +
      '" value="' + esc(row.weapon || '') + '" placeholder="Waffe" autocomplete="off">';
  }

  function ensureMaster(container) {
    if (container.dataset.mtSurface === 'master' && container.querySelector('#mt-table + form.mt-add')) return;
    container.dataset.mtSurface = 'master';
    container.classList.add('shared-master-host');
    container.classList.add('mt-master');
    container.innerHTML =
      '<div class="mt-master-body">' +
      '<header class="mt-head"><h1>Munitionstest</h1>' +
      '<p class="mt-muted">Zeile wählen zeigt das Los. Die Gesamtliste ist zum Drucken.</p></header>' +
      '<div id="mt-table"></div>' +
      '<form class="mt-add" data-select="0" data-range="1">' +
      '<input class="mt-draft" id="mt-new-lot-master" name="lot" placeholder="Neues Los" autocomplete="off">' +
      '<button type="submit" class="mt-icon-btn" aria-label="Los anlegen">' + iconPlus() + '</button></form>' +
      '<section class="mt-detail" id="mt-detail" hidden>' +
      '<div class="mt-scheibe" id="mt-detail-scheibe"></div>' +
      '<div class="mt-detail-side" id="mt-detail-side"></div></section>' +
      '<section class="mt-print-list" id="mt-print-list"></section>' +
      '</div>';
  }

  function paintMaster(container, vm) {
    const gen = (container._mtGen = (container._mtGen || 0) + 1);
    ensureMaster(container);
    const form = container.querySelector('form.mt-add');
    if (form) form.hidden = !canControl();
    const drafts = captureDrafts(container);
    const lanes = vm.lanes || [];
    const rows = rowsOf(lanes);
    const picked = chosenItem(container, rows);
    if (picked) container._mtPick = pickKey(picked.rangeNum, picked.row.lotId);
    const table = container.querySelector('#mt-table');
    if (table) {
      if (!rows.length) {
        table.innerHTML = '<p class="mt-muted">Noch kein Ergebnis. Lose anlegen, auf der Bahn auswählen, schießen.</p>';
      } else {
        table.innerHTML = '<table class="mt-table"><thead><tr>' +
          '<th>Los</th><th>Bahn</th><th>Waffe</th><th>Schüsse</th><th>Streukreis</th><th></th><th class="mt-act"></th>' +
          '</tr></thead><tbody>' + rows.map(function (item) {
            const r = item.row;
            const on = picked && pickKey(item.rangeNum, r.lotId) === container._mtPick;
            return '<tr data-pick="' + esc(pickKey(item.rangeNum, r.lotId)) + '"' +
              (on ? ' class="is-picked"' : '') + '>' +
              '<td>' + esc(r.name) + '</td>' +
              '<td>' + esc(item.rangeNum) + '</td>' +
              '<td>' + weaponCell(item.rangeNum, r, item.pending) + '</td>' +
              '<td>' + (item.pending ? '–' : esc(r.shotCount) + '/' + esc(item.shotsPer)) + '</td>' +
              '<td>' + groupText(r) + '</td>' +
              '<td class="mt-status">' + (item.pending ? 'bereit' : esc(statusText(r))) + abortHtml(item.active, item.rangeNum, true) + '</td>' +
              '<td class="mt-act">' + removeLotButton(r.lotId, item.rangeNum) + '</td>' +
              '</tr>';
          }).join('') + '</tbody></table>';
      }
    }
    const list = container.querySelector('#mt-print-list');
    if (list) list.innerHTML = printListHtml(rows);
    const detail = container.querySelector('#mt-detail');
    const side = container.querySelector('#mt-detail-side');
    if (detail) detail.hidden = !picked;
    if (side) {
      side.innerHTML = picked
        ? lotHeading(picked) + (detailSource(picked).shots && detailSource(picked).shots.length
          ? shotRows(detailSource(picked).shots)
          : '<p class="mt-muted">Keine Schüsse.</p>')
        : '';
    }
    restoreDrafts(container, drafts);
    return ensureRegistry().then(function () {
      if (container._mtGen !== gen) return;
      const host = container.querySelector('#mt-detail-scheibe');
      if (!host || !picked) return;
      const lane = lanes.filter(function (l) { return Number(l.rangeNum) === Number(picked.rangeNum); })[0] || lanes[0];
      const src = detailSource(picked);
      return paintScheibe(host, scaleFor(lane && lane.range), src.shots, src.diameterMm);
    });
  }

  window.SRPluginViews[PLUGIN_ID] = function (container, viewModel) {
    if (!container) return;
    const vm = viewModel || {};
    container._mtVM = vm;
    bind(container);
    if (vm.surface === 'master') return paintMaster(container, vm);
    return paintLane(container, vm);
  };
})();
