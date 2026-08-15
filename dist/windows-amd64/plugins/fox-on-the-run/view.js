window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  let audioCtx = null;
  let lastEventSig = '';

  const PHASE_LABEL = {
    calibrate: 'Kalibrierung',
    arming: 'Bereit',
    opening: 'Vorwürfe',
    chase: 'Treibjagd',
    round_result: 'Ergebnis',
    finished: 'Beendet'
  };

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
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
      if (ev.type === 'hunt_shot' || ev.type === 'opening_shot' || ev.type === 'cal_shot') {
        if (focusRange && rn && rn === focusRange) beep(980, 0.12, 'sine', 0.09);
        else beep(520, 0.08, 'triangle', 0.04);
      } else if (ev.type === 'wrong_turn') {
        if (!focusRange || rn === focusRange) beep(180, 0.2, 'square', 0.05);
      } else if (ev.type === 'escaped') {
        beep(660, 0.12, 'sine');
        setTimeout(function () { beep(880, 0.18, 'sine'); }, 120);
        setTimeout(function () { beep(1100, 0.22, 'sine'); }, 260);
      } else if (ev.type === 'caught') {
        beep(220, 0.35, 'triangle', 0.1);
      } else if (ev.type === 'turn') {
        if (!focusRange || Number(ev.data && ev.data.rangeNum) === focusRange) {
          beep(740, 0.1, 'sine', 0.06);
        }
      } else if (ev.type === 'chase_start' || ev.type === 'fox_round') {
        beep(500, 0.1); setTimeout(function () { beep(700, 0.15); }, 100);
      } else if (ev.type === 'match_finished') {
        beep(523, 0.15); setTimeout(function () { beep(659, 0.2); }, 150);
      }
    });
  }

  function ensureTargetRegistry(assetsBase) {
    return new Promise(function (resolve) {
      if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === 'fox-on-the-run') {
        resolve();
        return;
      }
      const root = (assetsBase || '/plugins/fox-on-the-run/assets').replace(/\/assets\/?$/, '/');
      const s = document.createElement('script');
      s.src = root + 'target-registry.js?t=' + Date.now();
      s.onload = function () {
        if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = 'fox-on-the-run';
        resolve();
      };
      s.onerror = function () { resolve(); };
      document.head.appendChild(s);
    });
  }

  function resolveProfileId(hunt, rangeData, me) {
    const def = (hunt && hunt.defaultTargetProfile) || 'air_rifle_10m';
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

  function profileFile(profileId) {
    const reg = window.SRTargetRegistry;
    const profiles = reg && (reg.profiles || reg.PROFILES);
    if (profiles && profiles[profileId]) return profiles[profileId].file;
    if (profileId === 'air_pistol_10m') return '10_m_Air_Pistol_target.svg';
    if (profileId.indexOf('smallbore') === 0) return '50_m_Smallbore_target.svg';
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
      coordMm: coordMm,
      center: 100,
      svgSize: 200,
      targetDiameterMm: diam,
      ring8RadiusMm: ring8,
      shotRadiusSvg: shotDiam / 2
    };
  }

  function dsgToSvg(xDsg, yDsg, profileId) {
    const m = profileMeta(profileId);
    const dsgPer = 9000 / m.coordMm;
    return { x: m.center + xDsg / dsgPer, y: m.center - yDsg / dsgPer };
  }

  /** Zoom to scoring disk (LG face is tiny in the 200 mm DISAG frame). */
  function scheibeViewBox(profileId, shots) {
    const m = profileMeta(profileId);
    const pad = 4;
    let half = m.targetDiameterMm / 2 + pad;
    if (shots && shots.length) {
      let maxDist = 0;
      shots.forEach(function (s) {
        const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0, profileId);
        const dist = Math.hypot(pt.x - m.center, pt.y - m.center) + m.shotRadiusSvg;
        if (dist > maxDist) maxDist = dist;
      });
      half = Math.max(half, maxDist + pad * 0.5, m.ring8RadiusMm + pad);
    }
    half = Math.min(half, m.svgSize / 2);
    return {
      x: m.center - half,
      y: m.center - half,
      w: half * 2,
      h: half * 2
    };
  }

  async function renderScheibe(host, assetsBase, hunt, focusRange, rangeData, me) {
    const profileId = resolveProfileId(hunt, rangeData, me);
    const file = profileFile(profileId);
    const url = (assetsBase || '/plugins/fox-on-the-run/assets').replace(/\/?$/, '/') + file;
    if (host.dataset.profile !== profileId || !host.querySelector('.fox-scheibe-frame svg')) {
      host.dataset.profile = profileId;
      try {
        const res = await fetch(url);
        const svgText = await res.text();
        host.innerHTML = '<div class="fox-scheibe-frame">' + svgText + '</div>';
        const svg = host.querySelector('svg');
        if (svg) {
          svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
          svg.setAttribute('width', '200');
          svg.setAttribute('height', '200');
          svg.removeAttribute('shape-rendering');
          svg.classList.add('fox-scheibe-svg');
          svg.querySelectorAll('[shape-rendering]').forEach(function (el) {
            el.setAttribute('shape-rendering', 'auto');
          });
          let g = svg.querySelector('#fox-shots');
          if (!g) {
            g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
            g.setAttribute('id', 'fox-shots');
            svg.appendChild(g);
          }
        }
      } catch (e) {
        host.innerHTML = '<div class="classic-range-loading">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#fox-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'fox-shots');
      svg.appendChild(g);
    }
    g.innerHTML = '';
    const shots = (hunt && hunt.recentShots) || [];
    const vb = scheibeViewBox(profileId, shots);
    svg.setAttribute('viewBox', vb.x + ' ' + vb.y + ' ' + vb.w + ' ' + vb.h);
    svg.setAttribute('width', String(vb.w));
    svg.setAttribute('height', String(vb.h));
    const meta = profileMeta(profileId);
    const shotR = meta.shotRadiusSvg;
    shots.forEach(function (s) {
      const pt = dsgToSvg(Number(s.x) || 0, Number(s.y) || 0, profileId);
      const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      c.setAttribute('cx', String(pt.x));
      c.setAttribute('cy', String(pt.y));
      c.setAttribute('r', String(shotR));
      const col = s.color || '#e85d04';
      const mine = Number(s.rangeNum) === Number(focusRange);
      if (mine) {
        c.setAttribute('fill', col);
        c.setAttribute('stroke', '#fff');
        c.setAttribute('stroke-width', '0.35');
      } else {
        c.setAttribute('fill', 'none');
        c.setAttribute('stroke', col);
        c.setAttribute('stroke-width', '0.7');
      }
      const title = document.createElementNS('http://www.w3.org/2000/svg', 'title');
      title.textContent = 'Stand ' + s.rangeNum + ': ' + Number(s.raw).toFixed(1);
      c.appendChild(title);
      g.appendChild(c);
    });
  }

  async function renderRevier(host, assetsBase, hunt, focusRange) {
    const url = (assetsBase || '/plugins/fox-on-the-run/assets').replace(/\/?$/, '/') + 'revier.svg?v=6';
    if (host.dataset.revierVer !== '6' || !host.querySelector('svg')) {
      host.dataset.revierVer = '6';
      try {
        const res = await fetch(url);
        host.innerHTML = await res.text();
        const img = host.querySelector('image');
        if (img && assetsBase) {
          const mapUrl = assetsBase.replace(/\/?$/, '/') + 'revier-map.png?v=6';
          img.setAttribute('href', mapUrl);
          img.setAttributeNS('http://www.w3.org/1999/xlink', 'href', mapUrl);
        }
      } catch (e) {
        host.innerHTML = '<div>Revier nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    const path = svg && svg.querySelector('#fahrte');
    let layer = svg && svg.querySelector('#tokens');
    if (!svg || !path || !layer) return;
    while (layer.firstChild) layer.removeChild(layer.firstChild);
    const len = path.getTotalLength();
    const players = (hunt && hunt.players) || [];
    const foxProg = hunt && hunt.foxProgress != null ? Number(hunt.foxProgress) : 0;
    const foxRange = hunt && hunt.currentFox;
    const NS = 'http://www.w3.org/2000/svg';

    function pointAt(t) {
      t = Math.max(0.02, Math.min(0.98, t));
      return path.getPointAtLength(len * t);
    }

    function tangentAngle(t) {
      const t0 = Math.max(0.01, t - 0.01);
      const t1 = Math.min(0.99, t + 0.01);
      const a = pointAt(t0);
      const b = pointAt(t1);
      return Math.atan2(b.y - a.y, b.x - a.x) * 180 / Math.PI;
    }

    let packIndex = 0;
    players.forEach(function (p) {
      if (!p || !p.active) return;
      const live = huntIsLive(hunt);
      const isFox = live && Number(p.rangeNum) === Number(foxRange) && Number(foxRange) > 0;
      let t = 0.04 + (packIndex % 6) * 0.016;
      packIndex += 1;
      if (live && isFox) {
        t = 0.12 + foxProg * 0.78;
      } else if (live) {
        t = 0.08 + Math.max(0, foxProg - 0.12 - (p.rangeNum % 5) * 0.02) * 0.7;
      }
      t = Math.max(0.02, Math.min(0.98, t));
      const pt = pointAt(t);
      if (isFox) {
        const g = document.createElementNS(NS, 'g');
        const ang = tangentAngle(t);
        g.setAttribute('transform',
          'translate(' + pt.x + ',' + pt.y + ') rotate(' + ang + ') scale(1.35)');
        g.setAttribute('class', 'fox-token-fox' +
          (Number(p.rangeNum) === Number(focusRange) ? ' is-me' : ''));
        const use = document.createElementNS(NS, 'use');
        // href for modern browsers; xlink for older SVG embeds
        use.setAttribute('href', '#fox-sprite');
        use.setAttributeNS('http://www.w3.org/1999/xlink', 'href', '#fox-sprite');
        g.appendChild(use);
        // lane color ring under fox
        const ring = document.createElementNS(NS, 'circle');
        ring.setAttribute('cx', '0');
        ring.setAttribute('cy', '10');
        ring.setAttribute('r', '11');
        ring.setAttribute('fill', 'none');
        ring.setAttribute('stroke', p.color || '#e85d04');
        ring.setAttribute('stroke-width', '3');
        ring.setAttribute('opacity', '0.9');
        g.insertBefore(ring, use);
        layer.appendChild(g);
      } else {
        const c = document.createElementNS(NS, 'circle');
        c.setAttribute('cx', String(pt.x));
        c.setAttribute('cy', String(pt.y));
        c.setAttribute('r', '11');
        c.setAttribute('fill', p.color || '#264653');
        c.setAttribute('class', 'fox-token' +
          (Number(p.rangeNum) === Number(focusRange) ? ' is-me' : ''));
        const label = document.createElementNS(NS, 'text');
        label.setAttribute('x', String(pt.x));
        label.setAttribute('y', String(pt.y + 4));
        label.setAttribute('text-anchor', 'middle');
        label.setAttribute('font-size', '11');
        label.setAttribute('font-weight', '700');
        label.setAttribute('fill', '#fff');
        label.setAttribute('pointer-events', 'none');
        label.textContent = String(p.rangeNum);
        layer.appendChild(c);
        layer.appendChild(label);
      }
    });
    if (hunt && hunt.terrain && hunt.terrain.label) {
      const tp = pointAt(0.12 + foxProg * 0.78 - 0.05);
      const label = String(hunt.terrain.label);
      const tw = Math.max(96, Math.round(label.length * 9.2) + 28);
      const th = 28;
      const bx = tp.x - tw / 2;
      const by = tp.y - 46;
      const bg = document.createElementNS(NS, 'rect');
      bg.setAttribute('x', String(bx));
      bg.setAttribute('y', String(by));
      bg.setAttribute('width', String(tw));
      bg.setAttribute('height', String(th));
      bg.setAttribute('rx', '10');
      bg.setAttribute('fill', '#fffdf8');
      bg.setAttribute('stroke', 'rgba(28,46,34,0.28)');
      bg.setAttribute('stroke-width', '1.5');
      bg.setAttribute('class', 'fox-terrain-chip-bg');
      const txt = document.createElementNS(NS, 'text');
      txt.setAttribute('x', String(tp.x));
      txt.setAttribute('y', String(by + 19));
      txt.setAttribute('text-anchor', 'middle');
      txt.setAttribute('font-family', '"Segoe UI", system-ui, -apple-system, Roboto, "Noto Sans", sans-serif');
      txt.setAttribute('font-size', '15');
      txt.setAttribute('font-weight', '700');
      txt.setAttribute('fill', '#1c2e22');
      txt.setAttribute('class', 'fox-terrain-chip-text');
      txt.textContent = label;
      layer.appendChild(bg);
      layer.appendChild(txt);
    }
  }

  function fmt1(n) {
    if (n == null || !isFinite(Number(n))) return '—';
    return (Math.round(Number(n) * 10) / 10).toFixed(1);
  }

  function roleLabel(role) {
    return role === 'fuchs' ? 'Fuchs' : 'Jäger';
  }

  function shotBlock(title, shot, signedDelta) {
    if (!shot) {
      return '<div class="fox-panel fox-panel-solid"><h3>' + esc(title) + '</h3><div class="fox-shot-sub">—</div></div>';
    }
    const d = shot.delta;
    let dStr = fmt1(d);
    if (signedDelta && shot.role === 'hunter') dStr = '−' + fmt1(Math.abs(d));
    else if (signedDelta && (shot.role === 'fox' || shot.role === 'opening')) dStr = '+' + fmt1(Math.abs(d));
    return '<div class="fox-panel fox-panel-solid"><h3>' + esc(title) + '</h3>' +
      '<div class="fox-shot-val">' + esc(fmt1(shot.raw)) + '</div>' +
      '<div class="fox-shot-sub">Δ ' + esc(dStr) + ' · Vorsprung ' + esc(fmt1(shot.leadAfter)) +
      (shot.distance != null ? ' · T ' + esc(fmt1(shot.distance)) : '') +
      (shot.rangeNum != null ? ' · Stand ' + esc(shot.rangeNum) : '') +
      '</div></div>';
  }

  function recentList(hunt, focusRange) {
    const shots = (hunt && hunt.recentShots) || [];
    if (!shots.length) {
      return '<div class="fox-panel fox-panel-solid"><h3>Letzte 3 auf der Scheibe</h3>' +
        '<div class="fox-shot-sub">Noch keine Schüsse</div></div>';
    }
    const items = shots.slice().reverse().map(function (s) {
      const mine = Number(s.rangeNum) === Number(focusRange);
      const sw = '<span class="fox-swatch' + (mine ? '' : ' outline') + '" style="' +
        (mine ? 'background:' + esc(s.color) : 'border-color:' + esc(s.color)) + '"></span>';
      const role = s.role === 'fox' || s.role === 'opening' ? 'Fuchs' : (s.role === 'calibrate' ? 'Kal.' : 'Jäger');
      const sign = (s.role === 'hunter') ? '−' : '+';
      return '<li>' + sw +
        '<span>S' + esc(s.rangeNum) + ' · ' + esc(role) + '</span>' +
        '<strong>' + esc(fmt1(s.raw)) + '</strong>' +
        '<span>' + esc(sign + fmt1(Math.abs(s.delta))) + '</span></li>';
    }).join('');
    return '<div class="fox-panel fox-panel-solid"><h3>Letzte 3 auf der Scheibe</h3>' +
      '<ul class="fox-recent">' + items + '</ul></div>';
  }

  function standingsHtml(hunt, focusRange) {
    const players = ((hunt && hunt.players) || []).slice().sort(function (a, b) {
      const sa = a.foxScore || 0;
      const sb = b.foxScore || 0;
      if (sb !== sa) return sb - sa;
      return (a.rangeNum || 0) - (b.rangeNum || 0);
    });
    return '<ul class="fox-standings">' + players.map(function (p) {
      const cls = []
        .concat(Number(p.rangeNum) === Number(focusRange) ? ['is-me'] : [])
        .concat(Number(p.rangeNum) === Number(hunt.currentFox) ? ['is-fox'] : [])
        .concat(Number(p.rangeNum) === Number(hunt.turnRange) ? ['is-turn'] : [])
        .join(' ');
      const name = p.shooterName || ('Stand ' + p.rangeNum);
      const out = p.outcome === 'escaped' ? 'Bau' : (p.outcome === 'caught' ? 'gestellt' : (p.hadFoxTurn ? '—' : 'wartet'));
      return '<li class="' + cls + '">' +
        '<span class="fox-swatch" style="background:' + esc(p.color) + '"></span>' +
        '<span>S' + esc(p.rangeNum) + '</span>' +
        '<span>' + esc(name) + '</span>' +
        '<span>' + esc(fmt1(p.foxScore)) + '</span>' +
        '<span>' + esc(out) + '</span></li>';
    }).join('') + '</ul>';
  }

  function footerMetaHtml(hunt) {
    return '<span class="fox-meta">Schüsse Jagd: ' + esc(hunt && hunt.chaseShots || 0) +
      (hunt && hunt.phase === 'opening'
        ? ' · Vorwurf ' + esc(hunt.openingCount) + '/' + esc(hunt.openingShots)
        : '') +
      '</span>';
  }

  function huntIsLive(hunt) {
    const p = hunt && hunt.phase;
    return p === 'opening' || p === 'chase' || p === 'round_result' || p === 'finished';
  }

  function hudHtml(hunt) {
    const badge = PHASE_LABEL[hunt && hunt.phase] || (hunt && hunt.phase) || '';
    if (!huntIsLive(hunt)) {
      return '<div class="fox-panel fox-hud-row">' +
        '<span class="fox-badge">' + esc(badge) + '</span>' +
        '<span class="fox-meta">' + esc((hunt && hunt.statusLine) || 'Einschießen') + '</span>' +
        '</div>';
    }
    const lead = hunt && hunt.lead;
    const escT = hunt && hunt.escapeTarget;
    const pct = hunt && hunt.foxProgress != null ? Math.round(Number(hunt.foxProgress) * 100) : 0;
    const fox = Number(hunt && hunt.currentFox) || 0;
    const turn = Number(hunt && hunt.turnRange) || 0;
    const who = (fox > 0 ? 'Fuchs S' + fox : 'Fuchs —') +
      ' · dran ' + (turn > 0 ? 'S' + turn : '—');
    return '<div class="fox-panel fox-hud-row">' +
      '<span class="fox-badge">' + esc(badge) + '</span>' +
      '<span class="fox-lead">Vorsprung ' + esc(fmt1(lead)) + ' / ' + esc(fmt1(escT)) + '</span>' +
      '<div class="fox-lead-bar"><div class="fox-lead-fill" style="width:' + pct + '%"></div></div>' +
      '<span class="fox-meta">' + esc(who) + '</span>' +
      '</div>';
  }

  function applyMeadowBackdrop(el, assetsBase) {
    if (!el) return;
    const base = (assetsBase || '/plugins/fox-on-the-run/assets').replace(/\/?$/, '/');
    const url = base + 'meadow-bg.svg?v=4';
    el.style.backgroundColor = '#343a28';
    el.style.backgroundImage =
      'radial-gradient(ellipse 50% 40% at 50% 35%, rgba(151, 152, 136, 0.18), transparent 70%),' +
      'url("' + url + '")';
    el.style.backgroundSize = 'cover';
    el.style.backgroundPosition = 'center';
    el.style.backgroundRepeat = 'no-repeat';
  }

  /** Keep #shared-master-host identity; never wipe host className to only fox-view. */
  function setFoxSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'fox-view');
    if (container.id === 'shared-master-host') {
      container.classList.add('shared-master-host');
    }
    if (mode === 'master') {
      container.classList.add('fox-race-master');
      container.classList.remove('fox-race-shooter');
    } else {
      container.classList.add('fox-race-shooter');
      container.classList.remove('fox-race-master');
    }
  }

  function beginFoxPaint(container) {
    const gen = (container._foxPaintGen || 0) + 1;
    container._foxPaintGen = gen;
    return gen;
  }

  function foxPaintStale(container, gen) {
    return !container || container._foxPaintGen !== gen;
  }

  async function renderShooter(container, viewModel, assetsBase) {
    const paintGen = beginFoxPaint(container);
    const core = window.SRCore;
    const hunt = (viewModel && viewModel.hunt) || {};
    const me = viewModel && viewModel.me;
    const rangeNum = viewModel && viewModel.rangeNum;
    let rangeData = null;
    if (core && core.lastLiveData) {
      rangeData = (core.lastLiveData.ranges || []).find(function (r) {
        return r.rangeNum === rangeNum;
      });
    }
    if (!rangeData && viewModel) rangeData = viewModel.range;

    setFoxSurfaceClasses(container, 'shooter');
    applyMeadowBackdrop(container, assetsBase);

    // Build skeleton before any await so concurrent paints share one DOM tree.
    if (container.querySelector('.fox-master-layout') || !container.querySelector('.fox-shooter-layout')) {
      container.innerHTML =
        '<div class="fox-shooter-layout">' +
        '<header class="fox-header" data-header></header>' +
        '<div class="fox-main">' +
        '<div class="fox-target-col">' +
        '<div class="fox-scheibe-wrap" data-scheibe></div>' +
        '<div class="fox-shot-hud" data-shothud></div>' +
        '<div data-recent></div>' +
        '</div>' +
        '<div class="fox-chase-col">' +
        '<div data-hud></div>' +
        '<div class="fox-revier-wrap" data-revier></div>' +
        '<div data-terrain></div>' +
        '<div data-standings></div>' +
        '</div></div>' +
        '<footer class="fox-footer" data-footer></footer>' +
        '</div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (foxPaintStale(container, paintGen)) return;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const role = viewModel && viewModel.myRole;
    const myTurn = viewModel && viewModel.myTurn;
    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="fox-title">Fuchs auf der Flucht</span>' +
      '<span class="fox-badge ' + esc(role === 'fuchs' ? '' : 'jäger') + '">' + esc(roleLabel(role)) + '</span>' +
      (myTurn ? '<span class="fox-badge turn">Du bist dran</span>' : '') +
      '<span class="fox-meta">Stand ' + esc(rangeNum) +
      (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') +
      (me && me.calibrated ? ' · Skill ' + esc(fmt1(me.skill)) + ' · Eq ' + esc(fmt1(me.equalizer)) : '') +
      '</span>' +
      '<span class="fox-status">' + esc(hunt.statusLine || '') + '</span>';

    await renderScheibe(container.querySelector('[data-scheibe]'), assetsBase, hunt, rangeNum, rangeData, me);
    if (foxPaintStale(container, paintGen)) return;
    container.querySelector('[data-shothud]').innerHTML =
      shotBlock('Dein letzter Schuss', viewModel && viewModel.lastOwn, true) +
      shotBlock('Letzter Fremdschuss', viewModel && viewModel.lastForeign, true);
    container.querySelector('[data-recent]').innerHTML = recentList(hunt, rangeNum);
    container.querySelector('[data-hud]').innerHTML = hudHtml(hunt) +
      (hunt.startBlockedReason ? '<div class="fox-blocked fox-panel">' + esc(hunt.startBlockedReason) + '</div>' : '');
    await renderRevier(container.querySelector('[data-revier]'), assetsBase, hunt, rangeNum);
    if (foxPaintStale(container, paintGen)) return;
    const terr = hunt.terrain;
    container.querySelector('[data-terrain]').innerHTML = terr && terr.label
      ? '<div class="fox-terrain fox-panel">' + esc(terr.label) + '</div>' : '';
    container.querySelector('[data-standings]').innerHTML = standingsHtml(hunt, rangeNum);
    const footer = container.querySelector('[data-footer]');
    if (footer) footer.innerHTML = footerMetaHtml(hunt);

    playEvents(viewModel && viewModel.events, rangeNum);
  }

  async function renderMaster(container, viewModel, assetsBase) {
    const paintGen = beginFoxPaint(container);
    const hunt = (viewModel && viewModel.hunt) || {};
    setFoxSurfaceClasses(container, 'master');
    applyMeadowBackdrop(container, assetsBase);

    if (
      container.querySelector('.fox-shooter-layout') ||
      !container.querySelector('.fox-master-layout') ||
      container.querySelector('.fox-footer')
    ) {
      container.innerHTML =
        '<div class="fox-master-layout">' +
        '<header class="fox-header" data-header></header>' +
        '<div class="fox-main">' +
        '<div class="fox-target-col">' +
        '<div class="fox-scheibe-wrap" data-scheibe></div>' +
        '<div data-recent></div>' +
        '<div data-standings></div>' +
        '</div>' +
        '<div class="fox-chase-col">' +
        '<div data-hud></div>' +
        '<div class="fox-revier-wrap" data-revier></div>' +
        '<div data-terrain></div>' +
        '</div></div>' +
        '</div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (foxPaintStale(container, paintGen)) return;

    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="fox-title">Fuchs auf der Flucht</span>' +
      '<span class="fox-badge">' + esc(PHASE_LABEL[hunt.phase] || hunt.phase || '') + '</span>' +
      '<span class="fox-status">' + esc(hunt.statusLine || '') + '</span>';
    container.querySelector('[data-hud]').innerHTML = hudHtml(hunt);
    await renderRevier(container.querySelector('[data-revier]'), assetsBase, hunt, null);
    if (foxPaintStale(container, paintGen)) return;
    const terr = hunt.terrain;
    container.querySelector('[data-terrain]').innerHTML = terr && terr.label
      ? '<div class="fox-terrain fox-panel">' + esc(terr.label) + '</div>' : '';
    await renderScheibe(container.querySelector('[data-scheibe]'), assetsBase, hunt, hunt.currentFox, null, null);
    if (foxPaintStale(container, paintGen)) return;
    container.querySelector('[data-recent]').innerHTML = recentList(hunt, hunt.currentFox);
    container.querySelector('[data-standings]').innerHTML = standingsHtml(hunt, null);
    // Enable in-place live updates (master used to remount fox every shot).
    container._sharedReady = true;
    playEvents(viewModel && viewModel.events, null);
  }

  function render(container, viewModel, assetsBase) {
    const isShooter = document.body.classList.contains('shooter-display') ||
      (window.SRDisplay && window.SRDisplay.display === 'shooter');
    if (isShooter) return renderShooter(container, viewModel, assetsBase);
    // Master / shared host: Scheibe left (same as Autorennen/classic), chase map right.
    return renderMaster(container, viewModel, assetsBase);
  }

  window.SRPluginViews['fox-on-the-run'] = render;
  window.SRPlugins['fox-on-the-run'] = { render: render };
})();
