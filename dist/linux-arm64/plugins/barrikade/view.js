window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const PLUGIN_ID = 'barrikade';
  let audioCtx = null;
  let lastEventSig = '';

  const PHASE_LABEL = {
    warmup: 'Einschießen',
    arming: 'Bereit',
    playing: 'Barrikade',
    finished: 'Beendet'
  };

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function fmt1(n) {
    if (n == null || !isFinite(Number(n))) return '—';
    return (Math.round(Number(n) * 10) / 10).toFixed(1);
  }

  function ensureAudio() {
    if (!audioCtx) {
      try { audioCtx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) { /* */ }
    }
    return audioCtx;
  }

  function beep(freq, dur, type, gain) {
    const ctx = ensureAudio();
    if (!ctx) return;
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = type || 'sine';
    o.frequency.value = freq;
    g.gain.value = gain == null ? 0.07 : gain;
    o.connect(g);
    g.connect(ctx.destination);
    o.start();
    g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + dur);
    o.stop(ctx.currentTime + dur);
  }

  function playEvents(events, focusRange) {
    if (!events || !events.length) return;
    const sig = JSON.stringify(events);
    if (sig === lastEventSig) return;
    lastEventSig = sig;
    events.forEach(function (ev) {
      if (!ev || !ev.type) return;
      const rn = ev.data && Number(ev.data.rangeNum);
      if (ev.type === 'walk') {
        if (!focusRange || rn === focusRange) beep(880, 0.12, 'sine', 0.09);
        else beep(620, 0.08, 'sine', 0.05);
      } else if (ev.type === 'lift') {
        beep(440, 0.16, 'triangle', 0.08);
      } else if (ev.type === 'miss' || ev.type === 'blocked') {
        if (!focusRange || rn === focusRange) beep(160, 0.18, 'square', 0.04);
      } else if (ev.type === 'win' || ev.type === 'match_finished') {
        beep(523, 0.12); setTimeout(function () { beep(659, 0.14); }, 120);
        setTimeout(function () { beep(784, 0.2); }, 260);
      } else if (ev.type === 'match_start') {
        beep(500, 0.1); setTimeout(function () { beep(700, 0.15); }, 100);
      }
    });
  }

  function ensureTargetRegistry(assetsBase) {
    return new Promise(function (resolve) {
      if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === PLUGIN_ID) {
        resolve();
        return;
      }
      const root = (assetsBase || ('/plugins/' + PLUGIN_ID + '/assets')).replace(/\/assets\/?$/, '/');
      const s = document.createElement('script');
      s.src = root + 'target-registry.js?t=' + Date.now();
      s.onload = function () {
        if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = PLUGIN_ID;
        resolve();
      };
      s.onerror = function () { resolve(); };
      document.head.appendChild(s);
    });
  }

  function profileFile(profileId) {
    const reg = window.SRTargetRegistry;
    const profiles = reg && (reg.profiles || reg.PROFILES);
    if (profiles && profiles[profileId]) return profiles[profileId].file;
    if (profileId === 'air_pistol_10m') return '10_m_Air_Pistol_target.svg';
    if (profileId && profileId.indexOf('smallbore') === 0) return '50_m_Smallbore_target.svg';
    return '10_m_Air_Rifle_target.svg';
  }

  function profileMeta(profileId) {
    const reg = window.SRTargetRegistry;
    const profiles = reg && (reg.profiles || reg.PROFILES);
    const p = (profiles && profiles[profileId]) || {};
    const coordMm = p.coordRadiusMm != null ? p.coordRadiusMm : 90;
    const diam = p.targetDiameterMm != null ? p.targetDiameterMm : 45.5;
    const ring8 = p.ring8RadiusMm != null ? p.ring8RadiusMm : diam * 0.12;
    const shotDiam = p.shotDiameterMm != null ? p.shotDiameterMm : 4.5;
    return {
      coordMm: coordMm, center: 100, svgSize: 200,
      targetDiameterMm: diam, ring8RadiusMm: ring8, shotRadiusSvg: shotDiam / 2
    };
  }

  function dsgToSvg(xDsg, yDsg, profileId) {
    const m = profileMeta(profileId);
    const dsgPer = 9000 / m.coordMm;
    return { x: m.center + xDsg / dsgPer, y: m.center - yDsg / dsgPer };
  }

  function scheibeViewBox(profileId, shots) {
    const m = profileMeta(profileId);
    const pad = 1.5;
    let half = m.targetDiameterMm / 2 + pad;
    if (shots && shots.length) {
      let maxDist = 0;
      shots.forEach(function (s) {
        const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0, profileId);
        const dist = Math.hypot(pt.x - m.center, pt.y - m.center) + m.shotRadiusSvg;
        if (dist > maxDist) maxDist = dist;
      });
      half = Math.max(half, maxDist + pad * 0.5, m.ring8RadiusMm);
    }
    half = Math.min(half, m.svgSize / 2);
    return { x: m.center - half, y: m.center - half, w: half * 2, h: half * 2 };
  }

  function resolveProfileId(game, rangeData, me) {
    const def = (game && game.defaultTargetProfile) || 'air_rifle_10m';
    const disc = (rangeData && (rangeData.discType || rangeData.discipline)) ||
      (me && me.discipline) || '';
    const reg = window.SRTargetRegistry;
    if (reg && typeof reg.resolveProfileId === 'function') {
      try { return reg.resolveProfileId(disc, def) || def; } catch (e) { /* */ }
    }
    const d = String(disc).toLowerCase();
    if (d.indexOf('lp') >= 0 || d.indexOf('pistole') >= 0) return 'air_pistol_10m';
    if (d.indexOf('kk') >= 0 || d.indexOf('klein') >= 0) return 'smallbore_50m_prone';
    return def;
  }

  async function renderScheibe(host, assetsBase, game, focusRange, rangeData, me) {
    if (!host) return;
    const profileId = resolveProfileId(game, rangeData, me);
    const file = profileFile(profileId);
    const url = (assetsBase || '').replace(/\/?$/, '/') + file;
    if (host.dataset.profile !== profileId || !host.querySelector('svg')) {
      host.dataset.profile = profileId;
      try {
        const res = await fetch(url);
        host.innerHTML = await res.text();
        const svg = host.querySelector('svg');
        if (svg) {
          svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
          svg.removeAttribute('width');
          svg.removeAttribute('height');
          svg.classList.add('br-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="br-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#br-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'br-shots');
      svg.appendChild(g);
    }
    g.innerHTML = '';
    const shots = (game && game.recentShots) || [];
    const vb = scheibeViewBox(profileId, shots);
    svg.setAttribute('viewBox', vb.x + ' ' + vb.y + ' ' + vb.w + ' ' + vb.h);
    const meta = profileMeta(profileId);
    shots.forEach(function (s) {
      const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0, profileId);
      const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      c.setAttribute('cx', String(pt.x));
      c.setAttribute('cy', String(pt.y));
      c.setAttribute('r', String(meta.shotRadiusSvg));
      const mine = Number(s.rangeNum) === Number(focusRange);
      const col = s.color || '#8a3b1a';
      if (mine) {
        c.setAttribute('fill', col);
        c.setAttribute('stroke', '#fff');
        c.setAttribute('stroke-width', '0.35');
      } else {
        c.setAttribute('fill', 'none');
        c.setAttribute('stroke', col);
        c.setAttribute('stroke-width', '0.7');
      }
      g.appendChild(c);
    });
  }

  function resultLabel(s) {
    if (!s) return '—';
    if (s.result === 'lift') return s.note || 'Mauer';
    if (s.result === 'walk') return s.note || 'Zug';
    return s.note || 'Fehl';
  }

  function shotBlock(title, shot) {
    if (!shot) {
      return '<div class="br-panel"><h3>' + esc(title) + '</h3><div class="br-muted">—</div></div>';
    }
    return '<div class="br-panel"><h3>' + esc(title) + '</h3>' +
      '<div class="br-shot-val">' + esc(fmt1(shot.raw)) + '</div>' +
      '<div class="br-muted">' + esc(resultLabel(shot)) +
      (shot.rangeNum != null ? ' · Stand ' + esc(shot.rangeNum) : '') +
      '</div></div>';
  }

  function recentHtml(game, focusRange) {
    const shots = (game && game.recentShots) || [];
    if (!shots.length) {
      return '<div class="br-panel"><h3>Letzte Schüsse</h3><div class="br-muted">Noch keine</div></div>';
    }
    const items = shots.slice().reverse().map(function (s) {
      const mine = Number(s.rangeNum) === Number(focusRange);
      return '<li class="' + (mine ? 'is-me' : '') + '">' +
        '<span class="br-swatch" style="background:' + esc(s.color || '#888') + '"></span>' +
        '<span>S' + esc(s.rangeNum) + ' · ' + esc(resultLabel(s)) + '</span>' +
        '<strong>' + esc(fmt1(s.raw)) + '</strong>' +
        '<span></span></li>';
    }).join('');
    return '<div class="br-panel"><h3>Letzte Schüsse</h3><ul class="br-recent">' + items + '</ul></div>';
  }

  function visiblePlayers(game) {
    const all = (game && game.players) || [];
    const seated = all.filter(function (p) { return p.seated; });
    if (seated.length) return seated;
    return all.filter(function (p) { return p.active; });
  }

  function standingsHtml(game, focusRange) {
    const citadel = Number(game && game.citadel) || 25;
    const list = visiblePlayers(game).slice().sort(function (a, b) {
      if (!!a.finished !== !!b.finished) return a.finished ? -1 : 1;
      return (b.cell || 0) - (a.cell || 0);
    });
    return '<ul class="br-standings">' + list.map(function (p) {
      const cls = [].concat(Number(p.rangeNum) === Number(focusRange) ? ['is-me'] : [])
        .concat(p.finished ? ['is-done'] : []).join(' ');
      return '<li class="' + cls + '">' +
        '<span class="br-swatch" style="background:' + esc(p.color || '#888') + '"></span>' +
        '<span>' + esc(p.label) + '</span>' +
        '<strong>' + (p.finished ? 'Burg' : esc(p.cell || 0) + '/' + esc(citadel)) + '</strong>' +
        '<span>' + esc(p.hint || '') + '</span></li>';
    }).join('') + '</ul>';
  }

  function cellPos(cell, citadel) {
    const cols = 5;
    const gapX = 56;
    const gapY = 52;
    const x0 = 40;
    if (cell <= 0) return { x: 152, y: 448 };
    if (cell >= citadel) return { x: 152, y: 36 };
    const idx = cell - 1;
    const row = Math.floor(idx / cols);
    const colRaw = idx % cols;
    const col = (row % 2 === 0) ? colRaw : (cols - 1 - colRaw);
    return { x: x0 + col * gapX, y: 400 - row * gapY };
  }

  function boardSvg(game, focusRange) {
    const citadel = Number(game && game.citadel) || 25;
    const slots = (game && game.slots) || [];
    const bars = (game && game.barricades) || [];
    const barSet = {};
    bars.forEach(function (b) { barSet[b] = true; });
    const slotSet = {};
    slots.forEach(function (s) { slotSet[s] = true; });
    const players = visiblePlayers(game);
    const byCell = {};
    players.forEach(function (p) {
      const c = p.finished ? citadel : (p.cell || 0);
      if (!byCell[c]) byCell[c] = [];
      byCell[c].push(p);
    });

    let path = '';
    for (let i = 1; i < citadel; i++) {
      const a = cellPos(i, citadel);
      const b = cellPos(i + 1, citadel);
      path += '<line x1="' + a.x + '" y1="' + a.y + '" x2="' + b.x + '" y2="' + b.y +
        '" stroke="rgba(42,26,18,0.22)" stroke-width="6" stroke-linecap="round"/>';
    }
    const y0 = cellPos(0, citadel);
    const y1 = cellPos(1, citadel);
    path += '<line x1="' + y0.x + '" y1="' + y0.y + '" x2="' + y1.x + '" y2="' + y1.y +
      '" stroke="rgba(42,26,18,0.18)" stroke-width="5" stroke-linecap="round"/>';

    let cells = '';
    for (let i = 0; i <= citadel; i++) {
      const p = cellPos(i, citadel);
      const isBar = !!barSet[i];
      const isSlot = !!slotSet[i];
      const isCit = i === citadel;
      const isYard = i === 0;
      const r = isCit ? 22 : (isYard ? 18 : 14);
      let fill = '#fff8f0';
      let stroke = isSlot ? '#c45c26' : '#8a6a4a';
      if (isBar) { fill = '#1a120e'; stroke = '#000'; }
      if (isCit) { fill = '#c9a227'; stroke = '#6b4a12'; }
      if (isYard) { fill = '#e8d5c4'; stroke = '#8a6a4a'; }
      cells += '<circle cx="' + p.x + '" cy="' + p.y + '" r="' + r + '" fill="' + fill +
        '" stroke="' + stroke + '" stroke-width="' + (isBar || isCit ? 3 : 1.5) + '"/>';
      if (isBar) {
        cells += '<rect x="' + (p.x - 8) + '" y="' + (p.y - 8) + '" width="16" height="16" rx="2" fill="#111"/>';
      }
      const lbl = isCit ? 'Burg' : (isYard ? 'Hof' : String(i));
      cells += '<text x="' + p.x + '" y="' + (p.y + (isBar ? 22 : 4)) +
        '" text-anchor="middle" font-size="' + (isCit || isYard ? 9 : 8) +
        '" font-weight="700" fill="' + (isBar ? '#fff8f0' : '#2a1a12') + '">' + esc(lbl) + '</text>';
    }

    let pawns = '';
    Object.keys(byCell).forEach(function (key) {
      const cell = Number(key);
      const group = byCell[key];
      const base = cellPos(cell, citadel);
      group.forEach(function (pl, i) {
        const ox = (i - (group.length - 1) / 2) * 12;
        const mine = Number(pl.rangeNum) === Number(focusRange);
        pawns += '<circle cx="' + (base.x + ox) + '" cy="' + (base.y - 2) + '" r="' + (mine ? 9 : 8) +
          '" fill="' + esc(pl.color || '#8a3b1a') + '" stroke="#fff8f0" stroke-width="' + (mine ? 2.5 : 1.5) + '"/>';
      });
    });

    const h = Math.max(480, 80 + Math.ceil((citadel - 1) / 5) * 52);
    return '<svg class="br-board-svg" viewBox="0 0 320 ' + h + '" preserveAspectRatio="xMidYMid meet" aria-label="Barrikade">' +
      path + cells + pawns + '</svg>';
  }

  function rangeDataFor(viewModel, rangeNum) {
    const core = window.SRCore;
    if (core && core.lastLiveData) {
      const hit = (core.lastLiveData.ranges || []).find(function (r) {
        return r.rangeNum === rangeNum;
      });
      if (hit) return hit;
    }
    return (viewModel && (viewModel.range || viewModel.liveRange)) || null;
  }

  /** Keep #shared-master-host identity; never wipe host className to only br-view. */
  function setBrSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'br-view');
    if (container.id === 'shared-master-host') {
      container.classList.add('shared-master-host');
    }
    if (mode === 'master') {
      container.classList.add('br-race-master');
      container.classList.remove('br-race-shooter');
    } else {
      container.classList.add('br-race-shooter');
      container.classList.remove('br-race-master');
    }
  }

  function beginPaint(container) {
    const gen = (container._brPaintGen || 0) + 1;
    container._brPaintGen = gen;
    return gen;
  }

  function brPaintStale(container, gen) {
    return !container || container._brPaintGen !== gen;
  }

  async function render(container, viewModel, assetsBase) {
    const paintGen = beginPaint(container);
    const vm = viewModel || {};
    const game = vm.game || {};
    const isShooter = document.body.classList.contains('shooter-display') ||
      (window.SRDisplay && window.SRDisplay.display === 'shooter');
    const isMaster = !isShooter;
    const focusRange = isShooter ? (Number(vm.rangeNum) || 0) : 0;
    const me = vm.me;

    playEvents(vm.events || [], focusRange || null);
    setBrSurfaceClasses(container, isMaster ? 'master' : 'shooter');

    const layoutCls = isMaster ? 'br-master-layout' : 'br-shooter-layout';
    const otherCls = isMaster ? 'br-shooter-layout' : 'br-master-layout';
    if (container.querySelector('.' + otherCls) || !container.querySelector('.' + layoutCls)) {
      container.innerHTML =
        '<div class="' + layoutCls + '">' +
        '<header class="br-header" data-header></header>' +
        '<div class="br-main">' +
        '<div class="br-target-col">' +
        '<div class="br-scheibe-wrap" data-scheibe></div>' +
        (isMaster ? '' : '<div class="br-shot-hud" data-shothud></div>') +
        '<div data-recent></div>' +
        '<div data-standings></div>' +
        '</div>' +
        '<div class="br-board-col">' +
        '<div class="br-board-host" data-board></div>' +
        '</div></div>' +
        '<footer class="br-footer" data-footer></footer></div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (brPaintStale(container, paintGen)) return;
    const core = window.SRCore;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="br-title">Barrikade</span>' +
      '<span class="br-badge">' + esc(PHASE_LABEL[game.phase] || game.phase || '') + '</span>' +
      (isShooter
        ? '<span class="br-meta">Stand ' + esc(focusRange) +
          (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>'
        : '') +
      '<span class="br-status">' + esc(game.statusLine || '') + '</span>';

    await renderScheibe(
      container.querySelector('[data-scheibe]'),
      assetsBase, game, isMaster ? null : focusRange,
      isMaster ? null : rangeDataFor(vm, focusRange), me
    );
    if (brPaintStale(container, paintGen)) return;

    if (!isMaster) {
      const hud = container.querySelector('[data-shothud]');
      if (hud) {
        hud.innerHTML = shotBlock('Dein letzter Schuss', vm.lastOwn) +
          shotBlock('Letzter Fremdschuss', vm.lastForeign);
      }
    }
    container.querySelector('[data-recent]').innerHTML = recentHtml(game, isMaster ? null : focusRange);
    container.querySelector('[data-standings]').innerHTML =
      '<div class="br-panel"><h3>Stand</h3>' + standingsHtml(game, isMaster ? null : focusRange) + '</div>';
    container.querySelector('[data-board]').innerHTML =
      (game.startBlockedReason ? '<div class="br-block">' + esc(game.startBlockedReason) + '</div>' : '') +
      boardSvg(game, isMaster ? null : focusRange);
    const footer = container.querySelector('[data-footer]');
    if (footer) {
      footer.innerHTML = '<span class="br-meta">9+ ein Feld · 10.0 hebt die Mauer · 10.5 zwei Felder · 6–7 bleiben stehen</span>';
    }
    if (container.id === 'shared-master-host') {
      container._sharedReady = true;
    }
  }

  window.SRPluginViews[PLUGIN_ID] = render;
  window.SRPlugins[PLUGIN_ID] = { render: render };
})();
