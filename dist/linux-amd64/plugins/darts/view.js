window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const PLUGIN_ID = 'darts';

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

  function profileMeta() {
    const reg = window.SRTargetRegistry;
    const profiles = reg && (reg.profiles || reg.PROFILES);
    const p = (profiles && profiles.dartboard) || {};
    const coordMm = p.coordRadiusMm != null ? p.coordRadiusMm : 90;
    const diam = p.targetDiameterMm != null ? p.targetDiameterMm : 170.6;
    const ring8 = p.ring8RadiusMm != null ? p.ring8RadiusMm : 85.3;
    const shotDiam = p.shotDiameterMm != null ? p.shotDiameterMm : 2;
    return {
      coordMm: coordMm, center: 100, svgSize: 200,
      targetDiameterMm: diam, ring8RadiusMm: ring8, shotRadiusSvg: shotDiam / 2
    };
  }

  function dsgToSvg(xDsg, yDsg) {
    const m = profileMeta();
    const dsgPer = 9000 / m.coordMm;
    return { x: m.center + xDsg / dsgPer, y: m.center - yDsg / dsgPer };
  }

  function scheibeViewBox(shots) {
    const m = profileMeta();
    const pad = 1.5;
    let half = m.targetDiameterMm / 2 + pad;
    if (shots && shots.length) {
      let maxDist = 0;
      shots.forEach(function (s) {
        const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0);
        const dist = Math.hypot(pt.x - m.center, pt.y - m.center) + m.shotRadiusSvg;
        if (dist > maxDist) maxDist = dist;
      });
      half = Math.max(half, maxDist + pad * 0.5, m.ring8RadiusMm);
    }
    half = Math.min(half, m.svgSize / 2);
    return { x: m.center - half, y: m.center - half, w: half * 2, h: half * 2 };
  }

  async function renderScheibe(host, assetsBase, game, focusRange) {
    if (!host) return;
    const file = 'dartboard.svg';
    const url = (assetsBase || '').replace(/\/?$/, '/') + file;
    if (host.dataset.profile !== 'dartboard' || !host.querySelector('svg')) {
      host.dataset.profile = 'dartboard';
      try {
        const res = await fetch(url);
        host.innerHTML = await res.text();
        const svg = host.querySelector('svg');
        if (svg) {
          svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
          svg.removeAttribute('width');
          svg.removeAttribute('height');
          svg.classList.add('dt-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="dt-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#dt-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'dt-shots');
      svg.appendChild(g);
    }
    g.innerHTML = '';
    const shots = (game && game.recentShots) || [];
    const vb = scheibeViewBox(shots);
    svg.setAttribute('viewBox', vb.x + ' ' + vb.y + ' ' + vb.w + ' ' + vb.h);
    const meta = profileMeta();
    shots.forEach(function (s) {
      const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0);
      const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      c.setAttribute('cx', String(pt.x));
      c.setAttribute('cy', String(pt.y));
      c.setAttribute('r', String(meta.shotRadiusSvg));
      const mine = focusRange && Number(s.rangeNum) === Number(focusRange);
      const col = s.color || '#e8c547';
      if (!focusRange || mine) {
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

  function visitText(p) {
    const three = (p && p.lastThree) || [];
    if (!three.length) return '';
    return three.join(' · ');
  }

  function standingsHtml(game, focusRange) {
    const players = (game && game.players) || [];
    const rows = players.filter(function (p) { return p && p.active; });
    if (!rows.length) return '<div class="dt-muted">Keine Bahn aktiv</div>';
    return '<ul class="dt-standings">' + rows.map(function (p) {
      const me = focusRange && Number(p.rangeNum) === Number(focusRange);
      const visit = visitText(p);
      return '<li class="' + (me ? 'is-me' : '') + (p.finished ? ' is-done' : '') + '">' +
        '<span class="dt-swatch" style="background:' + esc(p.color || '#888') + '"></span>' +
        '<span class="dt-who">' + esc(p.label || ('Stand ' + p.rangeNum)) + '</span>' +
        '<span class="dt-rem">' + esc(p.remaining) + '</span>' +
        (visit ? '<span class="dt-visit">' + esc(visit) + '</span>' : '<span class="dt-visit"></span>') +
        '</li>';
    }).join('') + '</ul>';
  }

  function shotBlock(title, shot) {
    if (!shot) {
      return '<div class="dt-panel"><h3>' + esc(title) + '</h3><div class="dt-muted">—</div></div>';
    }
    const note = shot.note && shot.note !== shot.label ? shot.note : '';
    return '<div class="dt-panel"><h3>' + esc(title) + '</h3>' +
      '<div class="dt-shot-val">' + esc(shot.label || '0') + '</div>' +
      (note ? '<div class="dt-muted">' + esc(note) + '</div>' : '') +
      '</div>';
  }

  function setDtSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'dt-view');
    if (container.id === 'shared-master-host') {
      container.classList.add('shared-master-host');
    }
    if (mode === 'master') {
      container.classList.add('dt-race-master');
      container.classList.remove('dt-race-shooter');
    } else {
      container.classList.add('dt-race-shooter');
      container.classList.remove('dt-race-master');
    }
  }

  function beginPaint(container) {
    const gen = (container._dtPaintGen || 0) + 1;
    container._dtPaintGen = gen;
    return gen;
  }

  function dtPaintStale(container, gen) {
    return !container || container._dtPaintGen !== gen;
  }

  async function render(container, viewModel, assetsBase) {
    const paintGen = beginPaint(container);
    const vm = viewModel || {};
    const game = vm.game || {};
    const isShooter = document.body.classList.contains('shooter-display') ||
      (window.SRDisplay && window.SRDisplay.display === 'shooter');
    const isCompact = window.SRDisplay && window.SRDisplay.display === 'compact';
    const focusRange = isShooter ? (Number(vm.rangeNum) || 0) : 0;
    const me = vm.me;

    setDtSurfaceClasses(container, isShooter ? 'shooter' : 'master');

    const layoutCls = isShooter ? 'dt-shooter-layout' : (isCompact ? 'dt-compact-layout' : 'dt-master-layout');
    if (!container.querySelector('.' + layoutCls)) {
      container.innerHTML =
        '<div class="' + layoutCls + '">' +
        '<header class="dt-header" data-header></header>' +
        '<div class="dt-main">' +
        '<div class="dt-target-col">' +
        (isCompact ? '' : '<div class="dt-scheibe-wrap" data-scheibe></div>') +
        (isShooter ? '<div class="dt-shot-hud" data-shothud></div>' : '') +
        '<div data-recent></div>' +
        '</div>' +
        '<div class="dt-board-col">' +
        '<div class="dt-board-host" data-board></div>' +
        '</div></div></div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (dtPaintStale(container, paintGen)) return;
    const core = window.SRCore;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const header = container.querySelector('[data-header]');
    if (!header) return;
    const rules = game.rulesLabel || '';
    const status = game.statusLine || '';
    header.innerHTML =
      '<span class="dt-title">Darts</span>' +
      '<span class="dt-badge">' + esc(PHASE_LABEL[game.phase] || game.phase || '') + '</span>' +
      (isShooter
        ? '<span class="dt-meta">Stand ' + esc(focusRange) +
          (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>'
        : '') +
      '<span class="dt-status">' + esc(rules) + '</span>' +
      (status && status !== rules ? '<span class="dt-note">' + esc(status) + '</span>' : '');

    const scheibe = container.querySelector('[data-scheibe]');
    if (scheibe) {
      await renderScheibe(scheibe, assetsBase, game, isShooter ? focusRange : null);
      if (dtPaintStale(container, paintGen)) return;
    }
    if (isShooter) {
      const hud = container.querySelector('[data-shothud]');
      if (hud) {
        hud.innerHTML = shotBlock('Dein letzter Pfeil', vm.lastOwn) +
          shotBlock('Letzter Fremdpfeil', vm.lastForeign);
      }
    }
    const recentHost = container.querySelector('[data-recent]');
    if (recentHost && isShooter && me) {
      recentHost.innerHTML = '<div class="dt-panel"><h3>Rest</h3><div class="dt-shot-val">' +
        esc(me.remaining) + '</div></div>';
    } else if (recentHost) {
      recentHost.innerHTML = '';
    }
    const board = container.querySelector('[data-board]');
    if (board) {
      board.innerHTML =
        (game.startBlockedReason ? '<div class="dt-block">' + esc(game.startBlockedReason) + '</div>' : '') +
        '<div class="dt-panel dt-scoreboard"><h3>Rest</h3>' + standingsHtml(game, isShooter ? focusRange : null) + '</div>';
    }
    if (container.id === 'shared-master-host') {
      container._sharedReady = true;
    }
  }

  window.SRPluginViews[PLUGIN_ID] = render;
  window.SRPlugins[PLUGIN_ID] = { render: render };
})();
