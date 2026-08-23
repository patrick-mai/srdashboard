window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const PLUGIN_ID = 'ansage-duell';
  let audioCtx = null;
  let lastEventSig = '';

  const PHASE_LABEL = {
    warmup: 'Einschießen',
    arming: 'Bereit',
    playing: 'Spiel',
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
      if (ev.type === 'hole_in_hole') beep(980, 0.16, 'triangle', 0.1);
      else if (ev.type === 'finished' || ev.type === 'match_finished' || ev.type === 'win') {
        beep(523, 0.12); setTimeout(function () { beep(659, 0.14); }, 120);
        setTimeout(function () { beep(784, 0.2); }, 260);
      } else if (ev.type === 'match_start') {
        beep(500, 0.1); setTimeout(function () { beep(700, 0.15); }, 100);
      }
    });
  }

  function postControl(action, params) {
    const body = JSON.stringify({ action: action, params: params || {} });
    const fn = (window.SRAuth && window.SRAuth.fetchWithAuth) || fetch;
    fn('/api/plugins/control', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body });
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
          svg.classList.add('ad-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="ad-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#ad-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'ad-shots');
      svg.appendChild(g);
    }
    while (g.firstChild) g.removeChild(g.firstChild);
    const shots = ((game && game.recentShots) || []).filter(function (s) {
      if (!focusRange) return true;
      return Number(s.rangeNum) === Number(focusRange);
    });
    const meta = profileMeta(profileId);
    const vb = scheibeViewBox(profileId, shots);
    svg.setAttribute('viewBox', vb.x + ' ' + vb.y + ' ' + vb.w + ' ' + vb.h);
    shots.forEach(function (s) {
      const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0, profileId);
      const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      c.setAttribute('cx', String(pt.x));
      c.setAttribute('cy', String(pt.y));
      c.setAttribute('r', String(meta.shotRadiusSvg));
      c.setAttribute('fill', s.color || '#c45c26');
      c.setAttribute('fill-opacity', '0.72');
      g.appendChild(c);
    });
  }

  function visiblePlayers(game) {
    return ((game && game.players) || []).filter(function (p) { return p && p.active !== false; });
  }

  function recentHtml(game, focusRange) {
    const list = ((game && game.recentShots) || []).slice(-6).reverse();
    if (!list.length) return '';
    return '<div class="ad-panel"><h3>Letzte Schüsse</h3><ul class="ad-recent">' + list.map(function (s) {
      const me = focusRange && Number(s.rangeNum) === Number(focusRange);
      return '<li class="' + (me ? 'is-me' : '') + '"><span class="ad-swatch" style="background:' +
        esc(s.color || '#888') + '"></span><span>S' + esc(s.rangeNum) + '</span><span>' +
        fmt1(s.raw) + '</span><span>' + esc(s.note || s.result || '') + '</span></li>';
    }).join('') + '</ul></div>';
  }

  function standingsHtml(game, focusRange) {
    const list = visiblePlayers(game);
    return '<ul class="ad-standings">' + list.map(function (p) {
      const me = focusRange && Number(p.rangeNum) === Number(focusRange);
      return '<li class="' + (me ? 'is-me' : '') + '"><span class="ad-swatch" style="background:' +
        esc(p.color || '#888') + '"></span><span>' + esc(p.label || ('Stand ' + p.rangeNum)) +
        '</span><span>' + esc(p.hint || '') + '</span><span>' + fmt1(p.lastRaw) + '</span></li>';
    }).join('') + '</ul>';
  }

  function shotBlock(title, shot) {
    if (!shot) return '<div class="ad-panel"><h3>' + esc(title) + '</h3><div class="ad-muted">—</div></div>';
    return '<div class="ad-panel"><h3>' + esc(title) + '</h3><div class="ad-shot-val">' +
      fmt1(shot.raw) + '</div><div class="ad-meta">' + esc(shot.note || '') + '</div></div>';
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

  function boardSvg(game, focusRange) {
    const players = visiblePlayers(game);
    return players.map(function (pl) {
      return '<div class="ad-panel' + (focusRange && pl.rangeNum === focusRange ? ' is-me' : '') + '">' +
        '<h3>' + esc(pl.label) + '</h3>' +
        '<div class="ad-meta">' + esc(pl.contractLabel || 'Keine Ansage') + '</div>' +
        '<div class="ad-shot-val">' + esc(pl.points || 0) + ' Pkt</div>' +
        '<div class="ad-meta">' + esc(pl.progress || '') + '</div></div>';
    }).join('');
  }

  function setAdSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'ad-view');
    if (container.id === 'shared-master-host') {
      container.classList.add('shared-master-host');
    }
    if (mode === 'master') {
      container.classList.add('ad-race-master');
      container.classList.remove('ad-race-shooter');
    } else {
      container.classList.add('ad-race-shooter');
      container.classList.remove('ad-race-master');
    }
  }

  function beginPaint(container) {
    const gen = (container._adPaintGen || 0) + 1;
    container._adPaintGen = gen;
    return gen;
  }

  function adPaintStale(container, gen) {
    return !container || container._adPaintGen !== gen;
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
    setAdSurfaceClasses(container, isMaster ? 'master' : 'shooter');

    const layoutCls = isMaster ? 'ad-master-layout' : 'ad-shooter-layout';
    const otherCls = isMaster ? 'ad-shooter-layout' : 'ad-master-layout';
    if (container.querySelector('.' + otherCls) || !container.querySelector('.' + layoutCls)) {
      container.innerHTML =
        '<div class="' + layoutCls + '">' +
        '<header class="ad-header" data-header></header>' +
        '<div class="ad-main">' +
        '<div class="ad-target-col">' +
        '<div class="ad-scheibe-wrap" data-scheibe></div>' +
        (isMaster ? '' : '<div class="ad-shot-hud" data-shothud></div>') +
        '<div data-recent></div>' +
        '<div data-standings></div>' +
        '<div data-actions></div>' +
        '</div>' +
        '<div class="ad-board-col">' +
        '<div class="ad-board-host" data-board></div>' +
        '</div></div>' +
        '<footer class="ad-footer" data-footer></footer></div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (adPaintStale(container, paintGen)) return;
    const core = window.SRCore;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="ad-title">Ansage-Duell</span>' +
      '<span class="ad-badge">' + esc(PHASE_LABEL[game.phase] || game.phase || '') + '</span>' +
      (isShooter
        ? '<span class="ad-meta">Stand ' + esc(focusRange) +
          (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>'
        : '') +
      '<span class="ad-status">' + esc(game.statusLine || '') + '</span>';

    await renderScheibe(
      container.querySelector('[data-scheibe]'),
      assetsBase, game, isMaster ? null : focusRange,
      isMaster ? null : rangeDataFor(vm, focusRange), me
    );
    if (adPaintStale(container, paintGen)) return;

    if (!isMaster) {
      const hud = container.querySelector('[data-shothud]');
      if (hud) {
        hud.innerHTML = shotBlock('Dein letzter Schuss', vm.lastOwn) +
          shotBlock('Letzter Fremdschuss', vm.lastForeign);
      }
    }
    container.querySelector('[data-recent]').innerHTML = recentHtml(game, isMaster ? null : focusRange);
    container.querySelector('[data-standings]').innerHTML =
      '<div class="ad-panel"><h3>Stand</h3>' + standingsHtml(game, isMaster ? null : focusRange) + '</div>';
    container.querySelector('[data-board]').innerHTML =
      (game.startBlockedReason ? '<div class="ad-block">' + esc(game.startBlockedReason) + '</div>' : '') +
      boardSvg(game, isMaster ? null : focusRange);

    if (!isMaster && me && game.phase === 'playing') {
      const actions = container.querySelector('[data-actions]');
      const contracts = game.contracts || [];
      if (actions) {
        actions.innerHTML = '<div class="ad-actions">' + contracts.map(function (c) {
          return '<button class="ad-btn" type="button" data-bid="' + esc(c.id) + '">' + esc(c.label) + '</button>';
        }).join('') + '</div>';
        actions.querySelectorAll('[data-bid]').forEach(function (btn) {
          btn.onclick = function () { postControl('bid', { rangeNum: focusRange, contractId: btn.getAttribute('data-bid') }); };
        });
      }
    }

    const footer = container.querySelector('[data-footer]');
    if (footer) {
      footer.innerHTML = '<span class="ad-meta">Ansage vor der Runde · nur höherer Wert oder Hole-in-Hole über 8,5 · halten bringt Punkte</span>';
    }
    if (container.id === 'shared-master-host') {
      container._sharedReady = true;
    }
  }

  window.SRPluginViews[PLUGIN_ID] = render;
  window.SRPlugins[PLUGIN_ID] = { render: render };
})();
