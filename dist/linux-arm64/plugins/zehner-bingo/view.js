window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const PLUGIN_ID = 'zehner-bingo';
  let audioCtx = null;
  let lastEventSig = '';

  const PHASE_LABEL = {
    warmup: 'Einschießen',
    arming: 'Bereit',
    playing: 'Bingo',
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
      if (ev.type === 'mark') {
        if (!focusRange || rn === focusRange) beep(880, 0.12, 'sine', 0.09);
        else beep(620, 0.08, 'sine', 0.05);
      } else if (ev.type === 'miss') {
        if (!focusRange || rn === focusRange) beep(160, 0.18, 'square', 0.04);
      } else if (ev.type === 'bingo' || ev.type === 'match_finished') {
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
          svg.classList.add('zb-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="zb-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#zb-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'zb-shots');
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
      const col = s.color || '#2f6b3a';
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
    if (s.result === 'mark') return 'Zelle ' + fmt1(s.value);
    return 'Fehl';
  }

  function shotBlock(title, shot) {
    if (!shot) {
      return '<div class="zb-panel"><h3>' + esc(title) + '</h3><div class="zb-muted">—</div></div>';
    }
    return '<div class="zb-panel"><h3>' + esc(title) + '</h3>' +
      '<div class="zb-shot-val">' + esc(fmt1(shot.raw)) + '</div>' +
      '<div class="zb-muted">' + esc(resultLabel(shot)) +
      (shot.rangeNum != null ? ' · Stand ' + esc(shot.rangeNum) : '') +
      '</div></div>';
  }

  function recentHtml(game, focusRange) {
    const shots = (game && game.recentShots) || [];
    if (!shots.length) {
      return '<div class="zb-panel"><h3>Letzte Schüsse</h3><div class="zb-muted">Noch keine</div></div>';
    }
    const items = shots.slice().reverse().map(function (s) {
      const mine = Number(s.rangeNum) === Number(focusRange);
      return '<li class="' + (mine ? 'is-me' : '') + '">' +
        '<span class="zb-swatch" style="background:' + esc(s.color || '#888') + '"></span>' +
        '<span>S' + esc(s.rangeNum) + ' · ' + esc(resultLabel(s)) + '</span>' +
        '<strong>' + esc(fmt1(s.raw)) + '</strong>' +
        '<span></span></li>';
    }).join('');
    return '<div class="zb-panel"><h3>Letzte Schüsse</h3><ul class="zb-recent">' + items + '</ul></div>';
  }

  function visiblePlayers(game) {
    const all = (game && game.players) || [];
    const seated = all.filter(function (p) { return p.seated; });
    if (seated.length) return seated;
    return all.filter(function (p) { return p.active; });
  }

  function standingsHtml(game, focusRange) {
    const list = visiblePlayers(game).slice().sort(function (a, b) {
      if (!!a.finished !== !!b.finished) return a.finished ? -1 : 1;
      return (b.markedCount || 0) - (a.markedCount || 0);
    });
    return '<ul class="zb-standings">' + list.map(function (p) {
      const cls = [].concat(Number(p.rangeNum) === Number(focusRange) ? ['is-me'] : [])
        .concat(p.finished ? ['is-done'] : []).join(' ');
      return '<li class="' + cls + '">' +
        '<span class="zb-swatch" style="background:' + esc(p.color || '#888') + '"></span>' +
        '<span>' + esc(p.label) + '</span>' +
        '<strong>' + (p.finished ? 'Bingo' : esc(p.markedCount || 0) + '/9') + '</strong>' +
        '<span></span></li>';
    }).join('') + '</ul>';
  }

  function cardSvg(player, cardValues) {
    const marked = (player && player.marked) || [];
    const line = {};
    ((player && player.bingoLine) || []).forEach(function (i) { line[i] = true; });
    const accent = (player && player.color) || '#2f6b3a';
    let cells = '';
    for (let i = 0; i < 9; i++) {
      const r = Math.floor(i / 3);
      const c = i % 3;
      const x = 8 + c * 54;
      const y = 8 + r * 54;
      const on = !!marked[i];
      const win = !!line[i];
      const fill = on ? accent : '#fffdf8';
      const stroke = win ? '#c9a227' : (on ? '#1e2f24' : '#c5b89a');
      const text = on ? '#fffdf8' : '#1e2f24';
      cells += '<g><rect x="' + x + '" y="' + y + '" width="50" height="50" rx="8" fill="' + fill +
        '" stroke="' + stroke + '" stroke-width="' + (win ? 3 : 1.5) + '"/>' +
        '<text x="' + (x + 25) + '" y="' + (y + 31) + '" text-anchor="middle" font-size="13" font-weight="700" fill="' + text + '">' +
        esc(fmt1(cardValues[i])) + '</text></g>';
    }
    return '<svg class="zb-card-svg" viewBox="0 0 178 178" aria-label="Bingo-Karte">' + cells + '</svg>';
  }

  function cardsHtml(game, focusRange, isMaster) {
    const cardValues = (game && game.cardValues) || [];
    const players = visiblePlayers(game);
    if (!players.length) {
      return '<div class="zb-muted">Keine Karten</div>';
    }
    const ordered = players.slice();
    if (!isMaster && focusRange) {
      ordered.sort(function (a, b) {
        const am = Number(a.rangeNum) === Number(focusRange) ? 0 : 1;
        const bm = Number(b.rangeNum) === Number(focusRange) ? 0 : 1;
        return am - bm;
      });
    }
    return '<div class="zb-cards">' + ordered.map(function (p) {
      const mine = Number(p.rangeNum) === Number(focusRange);
      const cls = ['zb-card']
        .concat(mine ? ['is-me'] : [])
        .concat(p.finished ? ['is-win'] : [])
        .join(' ');
      return '<div class="' + cls + '">' +
        '<div class="zb-card-label">' +
        '<span class="zb-swatch" style="background:' + esc(p.color || '#888') + '"></span>' +
        esc(p.label) +
        (p.finished ? ' · Bingo' : '') +
        '</div>' +
        cardSvg(p, cardValues) +
        '</div>';
    }).join('') + '</div>';
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

  /** Keep #shared-master-host identity; never wipe host className to only zb-view. */
  function setZbSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'zb-view');
    if (container.id === 'shared-master-host') {
      container.classList.add('shared-master-host');
    }
    if (mode === 'master') {
      container.classList.add('zb-race-master');
      container.classList.remove('zb-race-shooter');
    } else {
      container.classList.add('zb-race-shooter');
      container.classList.remove('zb-race-master');
    }
  }

  function beginPaint(container) {
    const gen = (container._zbPaintGen || 0) + 1;
    container._zbPaintGen = gen;
    return gen;
  }

  function zbPaintStale(container, gen) {
    return !container || container._zbPaintGen !== gen;
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
    setZbSurfaceClasses(container, isMaster ? 'master' : 'shooter');

    const layoutCls = isMaster ? 'zb-master-layout' : 'zb-shooter-layout';
    const otherCls = isMaster ? 'zb-shooter-layout' : 'zb-master-layout';
    if (container.querySelector('.' + otherCls) || !container.querySelector('.' + layoutCls)) {
      container.innerHTML =
        '<div class="' + layoutCls + '">' +
        '<header class="zb-header" data-header></header>' +
        '<div class="zb-main">' +
        '<div class="zb-target-col">' +
        '<div class="zb-scheibe-wrap" data-scheibe></div>' +
        (isMaster ? '' : '<div class="zb-shot-hud" data-shothud></div>') +
        '<div data-recent></div>' +
        '<div data-standings></div>' +
        '</div>' +
        '<div class="zb-board-col">' +
        '<div class="zb-board-host" data-board></div>' +
        '</div></div>' +
        '<footer class="zb-footer" data-footer></footer></div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (zbPaintStale(container, paintGen)) return;
    const core = window.SRCore;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="zb-title">Zehner-Bingo</span>' +
      '<span class="zb-badge">' + esc(PHASE_LABEL[game.phase] || game.phase || '') + '</span>' +
      (isShooter
        ? '<span class="zb-meta">Stand ' + esc(focusRange) +
          (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>'
        : '') +
      '<span class="zb-status">' + esc(game.statusLine || '') + '</span>';

    await renderScheibe(
      container.querySelector('[data-scheibe]'),
      assetsBase, game, isMaster ? null : focusRange,
      isMaster ? null : rangeDataFor(vm, focusRange), me
    );
    if (zbPaintStale(container, paintGen)) return;

    if (!isMaster) {
      const hud = container.querySelector('[data-shothud]');
      if (hud) {
        hud.innerHTML = shotBlock('Dein letzter Schuss', vm.lastOwn) +
          shotBlock('Letzter Fremdschuss', vm.lastForeign);
      }
    }
    container.querySelector('[data-recent]').innerHTML = recentHtml(game, isMaster ? null : focusRange);
    container.querySelector('[data-standings]').innerHTML =
      '<div class="zb-panel"><h3>Stand</h3>' + standingsHtml(game, isMaster ? null : focusRange) + '</div>';
    container.querySelector('[data-board]').innerHTML =
      (game.startBlockedReason ? '<div class="zb-block">' + esc(game.startBlockedReason) + '</div>' : '') +
      cardsHtml(game, focusRange, isMaster);
    const footer = container.querySelector('[data-footer]');
    if (footer) {
      footer.innerHTML = '<span class="zb-meta">' +
        (game.winMode === 'blackout' ? 'Vollkarte gewinnt' : 'Erste Linie gewinnt') +
        ' · höchste offene Zelle ≤ Schuss · 6–7 zählen nicht</span>';
    }
    if (container.id === 'shared-master-host') {
      container._sharedReady = true;
    }
  }

  window.SRPluginViews[PLUGIN_ID] = render;
  window.SRPlugins[PLUGIN_ID] = { render: render };
})();
