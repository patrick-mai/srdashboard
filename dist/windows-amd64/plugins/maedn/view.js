window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const PLUGIN_ID = 'maedn';
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
      const rn = ev.data && Number(ev.data.rangeNum);
      if (ev.type === 'walk' || ev.type === 'enter') {
        if (!focusRange || rn === focusRange) beep(880, 0.12, 'sine', 0.09);
        else beep(620, 0.08, 'sine', 0.05);
      } else if (ev.type === 'capture') {
        beep(220, 0.22, 'triangle', 0.1);
      } else if (ev.type === 'miss') {
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
          svg.classList.add('md-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="md-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#md-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'md-shots');
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
      const col = s.color || '#1e4d7b';
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
    if (s.result === 'enter') return 'Einsatz';
    if (s.result === 'walk') return s.note || 'Zug';
    return s.note || 'Fehl';
  }

  function shotBlock(title, shot) {
    if (!shot) {
      return '<div class="md-panel"><h3>' + esc(title) + '</h3><div class="md-muted">—</div></div>';
    }
    return '<div class="md-panel"><h3>' + esc(title) + '</h3>' +
      '<div class="md-shot-val">' + esc(fmt1(shot.raw)) + '</div>' +
      '<div class="md-muted">' + esc(resultLabel(shot)) +
      (shot.rangeNum != null ? ' · Stand ' + esc(shot.rangeNum) : '') +
      '</div></div>';
  }

  function recentHtml(game, focusRange) {
    const shots = (game && game.recentShots) || [];
    if (!shots.length) {
      return '<div class="md-panel"><h3>Letzte Schüsse</h3><div class="md-muted">Noch keine</div></div>';
    }
    const items = shots.slice().reverse().map(function (s) {
      const mine = Number(s.rangeNum) === Number(focusRange);
      return '<li class="' + (mine ? 'is-me' : '') + '">' +
        '<span class="md-swatch" style="background:' + esc(s.color || '#888') + '"></span>' +
        '<span>S' + esc(s.rangeNum) + ' · ' + esc(resultLabel(s)) + '</span>' +
        '<strong>' + esc(fmt1(s.raw)) + '</strong>' +
        '<span></span></li>';
    }).join('');
    return '<div class="md-panel"><h3>Letzte Schüsse</h3><ul class="md-recent">' + items + '</ul></div>';
  }

  function visiblePlayers(game) {
    const all = (game && game.players) || [];
    const seated = all.filter(function (p) { return p.seated; });
    if (seated.length) return seated;
    return all.filter(function (p) { return p.active; });
  }

  function standingsHtml(game, focusRange) {
    const homeLen = Number(game && game.homeLength) || 4;
    const list = visiblePlayers(game).slice().sort(function (a, b) {
      if (!!a.finished !== !!b.finished) return a.finished ? -1 : 1;
      const as = a.inYard ? -1 : (a.home > 0 ? 1000 + a.home : (a.lapProgress || 0));
      const bs = b.inYard ? -1 : (b.home > 0 ? 1000 + b.home : (b.lapProgress || 0));
      return bs - as;
    });
    return '<ul class="md-standings">' + list.map(function (p) {
      const cls = [].concat(Number(p.rangeNum) === Number(focusRange) ? ['is-me'] : [])
        .concat(p.finished ? ['is-done'] : []).join(' ');
      let pos = 'Hof';
      if (p.finished) pos = 'Ziel';
      else if (p.home > 0) pos = 'Heim ' + p.home + '/' + homeLen;
      else if (!p.inYard) pos = 'Feld ' + (p.ringCell != null ? p.ringCell : '—');
      return '<li class="' + cls + '">' +
        '<span class="md-swatch" style="background:' + esc(p.color || '#888') + '"></span>' +
        '<span>' + esc(p.label) + '</span>' +
        '<strong>' + esc(pos) + '</strong>' +
        '<span>' + esc(p.hint || '') + '</span></li>';
    }).join('') + '</ul>';
  }

  function polar(cx, cy, r, a) {
    return { x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) };
  }

  function f1(n) {
    return (Math.round(Number(n) * 10) / 10).toFixed(1);
  }

  function hexMix(hex, amt) {
    let h = String(hex || '').replace('#', '');
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    const n = parseInt(h, 16);
    let r = 180, g = 50, b = 40;
    if (h.length === 6 && isFinite(n)) {
      r = (n >> 16) & 255;
      g = (n >> 8) & 255;
      b = n & 255;
    }
    const f = function (c) {
      if (amt >= 0) return Math.round(c + (255 - c) * amt);
      return Math.round(c * (1 + amt));
    };
    const to = function (c) {
      return f(c).toString(16).padStart(2, '0');
    };
    return '#' + to(r) + to(g) + to(b);
  }

  function clipName(s) {
    s = String(s || '');
    if (s.length <= 16) return s;
    return s.slice(0, 15) + '…';
  }

  function hatMarkup(x, y, color, label, mine, done) {
    const s = mine ? 1.28 : (done ? 1.18 : 1.12);
    const mid = color || '#8b2e1f';
    const dark = hexMix(mid, -0.38);
    const light = hexMix(mid, 0.28);
    return '<g class="md-hat' + (mine ? ' is-me' : '') + (done ? ' is-done' : '') +
      '" transform="translate(' + f1(x) + ',' + f1(y) + ') scale(' + s + ')">' +
      '<ellipse cx="0" cy="7.2" rx="9.2" ry="3.1" fill="rgba(40,18,8,0.28)"/>' +
      '<path d="M0,-12 L8,3.2 Q0,6.4 -8,3.2 Z" fill="' + dark + '"/>' +
      '<path d="M0,-12 L6.4,2.4 Q0,5.2 -3.6,3.1 Z" fill="' + mid + '"/>' +
      '<ellipse cx="0" cy="4.1" rx="8.4" ry="3.15" fill="' + mid + '" stroke="#fffaf0" stroke-width="1.15"/>' +
      '<ellipse cx="0" cy="3.3" rx="6.1" ry="2" fill="' + light + '" opacity="0.5"/>' +
      '<ellipse cx="-1.8" cy="-6.4" rx="2.3" ry="2.7" fill="#fff" opacity="0.32"/>' +
      '<text x="0" y="6.4" text-anchor="middle" font-size="6.6" font-weight="700" fill="#fffaf0">' +
      esc(label) + '</text></g>';
  }

  function gxy(cx, cy, gap, gx, gy) {
    return { x: cx + gx * gap, y: cy + gy * gap };
  }

  function classicRingPts(cpp, gap, cx, cy) {
    const full = [
      [1, 5], [0, 5], [-1, 5], [-1, 4], [-1, 3], [-1, 2], [-1, 1], [-2, 1], [-3, 1], [-4, 1],
      [-5, 1], [-5, 0], [-5, -1], [-4, -1], [-3, -1], [-2, -1], [-1, -1], [-1, -2], [-1, -3], [-1, -4],
      [-1, -5], [0, -5], [1, -5], [1, -4], [1, -3], [1, -2], [1, -1], [2, -1], [3, -1], [4, -1],
      [5, -1], [5, 0], [5, 1], [4, 1], [3, 1], [2, 1], [1, 1], [1, 2], [1, 3], [1, 4]
    ];
    const pts = [];
    let idxs = [];
    if (cpp >= 10) {
      for (let i = 0; i < 10; i++) idxs.push(i);
    } else {
      idxs = [0, 1, 2];
      const remain = Math.max(1, cpp - 3);
      for (let i = 1; i <= remain; i++) {
        const idx = Math.min(9, 2 + Math.round(i * 7 / remain));
        if (idxs.indexOf(idx) < 0) idxs.push(idx);
      }
      idxs.sort(function (a, b) { return a - b; });
      for (let k = 0; k < 10 && idxs.length < cpp; k++) {
        if (idxs.indexOf(k) < 0) idxs.push(k);
      }
      idxs.sort(function (a, b) { return a - b; });
      idxs = idxs.slice(0, cpp);
    }
    for (let arm = 0; arm < 4; arm++) {
      for (let i = 0; i < cpp; i++) {
        if (cpp > 10) {
          const t = (i / cpp) * 10;
          const i0 = Math.min(9, Math.floor(t));
          const f = t - i0;
          const a = full[arm * 10 + i0];
          const b = full[(arm * 10 + i0 + 1) % 40];
          pts.push(gxy(cx, cy, gap, a[0] + (b[0] - a[0]) * f, a[1] + (b[1] - a[1]) * f));
        } else {
          const a = full[arm * 10 + idxs[i]];
          pts.push(gxy(cx, cy, gap, a[0], a[1]));
        }
      }
    }
    return pts;
  }

  function starRingPts(n, cpp, gap, cx, cy, homeLen) {
    const a0 = Math.PI / 2;
    const rTip = (homeLen + 1.15) * gap;
    const rCrotch = n <= 2 ? rTip * 0.62 : (n === 3 ? gap * 2.15 : gap * (0.9 + n * 0.32));
    const tipSpread = Math.asin(Math.min(0.92, gap / Math.max(rTip, 1)));
    const pts = [];
    for (let arm = 0; arm < n; arm++) {
      const a = a0 + arm * (2 * Math.PI / n);
      const aNext = a0 + ((arm + 1) % n) * (2 * Math.PI / n);
      const cap0 = polar(cx, cy, rTip, a - tipSpread);
      const cap1 = polar(cx, cy, rTip, a);
      const cap2 = polar(cx, cy, rTip, a + tipSpread);
      const crotch = polar(cx, cy, rCrotch, a + Math.PI / n);
      const nextStart = polar(cx, cy, rTip, aNext - tipSpread);
      const remain = Math.max(0, cpp - 3);
      const inner = remain >= 3 ? 1 : 0;
      const rest = remain - inner;
      const outN = Math.ceil(rest / 2);
      const inN = Math.floor(rest / 2);
      const armPts = [cap0, cap1, cap2];
      for (let k = 1; k <= outN; k++) {
        const t = k / (outN + inner + 0.0001);
        armPts.push({
          x: cap2.x + (crotch.x - cap2.x) * t,
          y: cap2.y + (crotch.y - cap2.y) * t
        });
      }
      if (inner) armPts.push(crotch);
      const from = inner ? crotch : cap2;
      for (let k = 1; k <= inN; k++) {
        const t = k / (inN + 1);
        armPts.push({
          x: from.x + (nextStart.x - from.x) * t,
          y: from.y + (nextStart.y - from.y) * t
        });
      }
      while (armPts.length < cpp) {
        const last = armPts[armPts.length - 1];
        armPts.push({
          x: last.x + (nextStart.x - last.x) * 0.35,
          y: last.y + (nextStart.y - last.y) * 0.35
        });
      }
      for (let i = 0; i < cpp; i++) pts.push(armPts[i]);
    }
    return { pts: pts, rTip: rTip, a0: a0, tipSpread: tipSpread, rCrotch: rCrotch };
  }

  function boardSvg(game, focusRange) {
    const players = visiblePlayers(game);
    const n = Math.max(2, players.length);
    const per = Math.max(4, Number(game && game.cellsPerPlayer) || 8);
    const homeLen = Math.max(1, Number(game && game.homeLength) || 4);
    let ringSize = Number(game && game.ringSize) || 0;
    if (ringSize < n * 4) ringSize = per * n;
    const cpp = Math.max(4, Math.round(ringSize / n));
    const VB = 500;
    const cx = 250;
    const cy = 250;
    const gap = n >= 6 ? 22 : (n === 5 ? 24 : 30);
    const cellR = n >= 6 ? 9.4 : 11.4;
    const classic = n === 4;
    const FALLBACK = ['#c62828', '#1565c0', '#2e7d32', '#f9a825', '#6a1b9a', '#00838f', '#ef6c00', '#37474f'];

    const entries = players.map(function (p) { return Number(p.entry) || 0; });
    const uniq = {};
    entries.forEach(function (e) { uniq[e] = true; });
    const useIndex = players.length > 1 && Object.keys(uniq).length <= 1;

    function armOf(pl, i) {
      if (useIndex) return i % n;
      return ((Math.floor((Number(pl.entry) || 0) / cpp) % n) + n) % n;
    }

    const byArm = new Array(n);
    players.forEach(function (pl, i) { byArm[armOf(pl, i)] = pl; });

    function armColor(arm) {
      const pl = byArm[arm];
      return (pl && pl.color) || FALLBACK[arm % FALLBACK.length];
    }

    function armAngle(arm) {
      return Math.PI / 2 + arm * (2 * Math.PI / n);
    }

    let ringPts;
    let starMeta = null;
    if (classic) {
      ringPts = classicRingPts(cpp, gap, cx, cy);
    } else {
      starMeta = starRingPts(n, cpp, gap, cx, cy, homeLen);
      ringPts = starMeta.pts;
    }

    function homePos(arm, h) {
      if (classic) {
        const u = homeLen <= 1 ? 2.6 : (4.1 - (h - 1) * (2.7 / (homeLen - 1)));
        const g = [[0, u], [-u, 0], [0, -u], [u, 0]][arm];
        return gxy(cx, cy, gap, g[0], g[1]);
      }
      const rTip = starMeta.rTip;
      const r = rTip - h * ((rTip - 1.6 * gap) / homeLen);
      return polar(cx, cy, r, armAngle(arm));
    }

    function yardPos(arm, slot) {
      if (classic) {
        const c = [[1, 1], [-1, 1], [-1, -1], [1, -1]][arm];
        const col = slot % 2;
        const row = (slot / 2) | 0;
        return gxy(cx, cy, gap, c[0] * (3.15 + col * 1.05), c[1] * (3.15 + row * 1.05));
      }
      const a = armAngle(arm);
      const aCorner = a - Math.PI / n;
      const base = polar(cx, cy, starMeta.rTip + 1.7 * gap, a * 0.38 + aCorner * 0.62);
      const ux = Math.cos(a);
      const uy = Math.sin(a);
      const vx = Math.cos(a + Math.PI / 2);
      const vy = Math.sin(a + Math.PI / 2);
      const col = slot % 2;
      const row = (slot / 2) | 0;
      return {
        x: base.x + (col - 0.5) * gap * 0.95 * vx + (row - 0.5) * gap * 0.95 * ux,
        y: base.y + (col - 0.5) * gap * 0.95 * vy + (row - 0.5) * gap * 0.95 * uy
      };
    }

    function yardLabelPos(arm) {
      if (classic) {
        const c = [[1, 1], [-1, 1], [-1, -1], [1, -1]][arm];
        return gxy(cx, cy, gap, c[0] * 4.55, c[1] * 4.55);
      }
      const a = armAngle(arm);
      const aCorner = a - Math.PI / n;
      return polar(cx, cy, starMeta.rTip + 2.15 * gap, a * 0.32 + aCorner * 0.68);
    }

    const t = 1.55 * gap;
    const L = 5.55 * gap;
    const plusD = 'M' + f1(cx - t) + ',' + f1(cy - L) +
      ' H' + f1(cx + t) + ' V' + f1(cy - t) +
      ' H' + f1(cx + L) + ' V' + f1(cy + t) +
      ' H' + f1(cx + t) + ' V' + f1(cy + L) +
      ' H' + f1(cx - t) + ' V' + f1(cy + t) +
      ' H' + f1(cx - L) + ' V' + f1(cy - t) +
      ' H' + f1(cx - t) + ' Z';

    let starFillD = '';
    if (!classic) {
      for (let arm = 0; arm < n; arm++) {
        const a = armAngle(arm);
        const p0 = polar(cx, cy, starMeta.rTip + 0.55 * gap, a - starMeta.tipSpread);
        const p1 = polar(cx, cy, starMeta.rTip + 0.55 * gap, a + starMeta.tipSpread);
        const p2 = polar(cx, cy, starMeta.rCrotch, a + Math.PI / n);
        starFillD += (arm ? 'L' : 'M') + f1(p0.x) + ',' + f1(p0.y) +
          ' L' + f1(p1.x) + ',' + f1(p1.y) + ' L' + f1(p2.x) + ',' + f1(p2.y) + ' ';
      }
      starFillD += 'Z';
    }

    let ribbonD = '';
    ringPts.forEach(function (p, i) {
      ribbonD += (i ? 'L' : 'M') + f1(p.x) + ' ' + f1(p.y) + ' ';
    });
    ribbonD += 'Z';

    const defs =
      '<defs>' +
      '<linearGradient id="md-wood" x1="0" y1="0" x2="1" y2="1">' +
      '<stop offset="0%" stop-color="#9a6844"/><stop offset="42%" stop-color="#734830"/>' +
      '<stop offset="100%" stop-color="#4a2c1a"/></linearGradient>' +
      '<pattern id="md-grain" width="18" height="18" patternUnits="userSpaceOnUse">' +
      '<path d="M0 4h18 M0 11h18 M0 16h18" stroke="#5a3520" stroke-width="0.7" opacity="0.28"/></pattern>' +
      '<radialGradient id="md-felt" cx="50%" cy="42%" r="68%">' +
      '<stop offset="0%" stop-color="#f7ecd4"/><stop offset="100%" stop-color="#e2cc9e"/></radialGradient>' +
      '<filter id="md-board-shadow" x="-8%" y="-8%" width="116%" height="120%">' +
      '<feDropShadow dx="0" dy="5" stdDeviation="4.5" flood-color="#000" flood-opacity="0.32"/></filter>' +
      '</defs>';

    const wood =
      '<g filter="url(#md-board-shadow)">' +
      '<rect x="12" y="12" width="476" height="476" rx="40" fill="url(#md-wood)"/>' +
      '<rect x="12" y="12" width="476" height="476" rx="40" fill="url(#md-grain)" opacity="0.45"/>' +
      '</g>' +
      '<rect x="46" y="46" width="408" height="408" rx="22" fill="url(#md-felt)" stroke="#c4a56a" stroke-width="2.4"/>' +
      '<rect x="54" y="54" width="392" height="392" rx="16" fill="none" stroke="#8a5a32" stroke-width="1.15" opacity="0.4"/>';

    const figure = classic
      ? '<path d="' + plusD + '" fill="#efe0c0" stroke="#c4a56a" stroke-width="2.2" stroke-linejoin="round"/>'
      : '<path d="' + starFillD + '" fill="#efe0c0" stroke="#c4a56a" stroke-width="2.2" stroke-linejoin="round"/>';

    let pads = '';
    for (let arm = 0; arm < n; arm++) {
      const col = armColor(arm);
      if (classic) {
        const c = [[1, 1], [-1, 1], [-1, -1], [1, -1]][arm];
        const x0 = cx + (c[0] > 0 ? 1.85 : -5.4) * gap;
        const y0 = cy + (c[1] > 0 ? 1.85 : -5.4) * gap;
        pads += '<rect x="' + f1(x0) + '" y="' + f1(y0) + '" width="' + f1(3.55 * gap) +
          '" height="' + f1(3.55 * gap) + '" rx="16" fill="' + esc(col) + '" fill-opacity="0.42"/>';
      } else {
        const p = yardPos(arm, 0);
        const q = yardPos(arm, 3);
        const mx = (p.x + q.x) / 2;
        const my = (p.y + q.y) / 2;
        pads += '<circle cx="' + f1(mx) + '" cy="' + f1(my) + '" r="' + f1(gap * 1.45) +
          '" fill="' + esc(col) + '" fill-opacity="0.38"/>';
      }
    }

    let lanes = '';
    for (let arm = 0; arm < n; arm++) {
      const a = homePos(arm, 1);
      const b = homePos(arm, homeLen);
      lanes += '<line x1="' + f1(a.x) + '" y1="' + f1(a.y) + '" x2="' + f1(b.x) + '" y2="' + f1(b.y) +
        '" stroke="' + esc(armColor(arm)) + '" stroke-opacity="0.4" stroke-width="' +
        f1(cellR * 1.85) + '" stroke-linecap="round"/>';
    }

    const ribbon = '<path d="' + ribbonD + '" fill="none" stroke="#d7c196" stroke-width="' +
      f1(cellR * 1.7) + '" stroke-linejoin="round" stroke-linecap="round"/>';

    let yards = '';
    let homes = '';
    let labels = '';
    for (let arm = 0; arm < n; arm++) {
      const col = armColor(arm);
      const light = hexMix(col, 0.55);
      for (let s = 0; s < 4; s++) {
        const p = yardPos(arm, s);
        yards += '<circle cx="' + f1(p.x) + '" cy="' + f1(p.y) + '" r="' + f1(cellR * 0.92) +
          '" fill="' + light + '" stroke="' + esc(col) + '" stroke-width="2"/>';
      }
      for (let h = 1; h <= homeLen; h++) {
        const p = homePos(arm, h);
        homes += '<circle cx="' + f1(p.x) + '" cy="' + f1(p.y) + '" r="' + f1(cellR * 0.88) +
          '" fill="' + esc(col) + '" stroke="#fffaf0" stroke-width="1.6"/>';
        homes += '<text x="' + f1(p.x) + '" y="' + f1(p.y + 3.2) +
          '" text-anchor="middle" font-size="8" font-weight="700" fill="#fffaf0">' + h + '</text>';
      }
      const pl = byArm[arm];
      const lp = yardLabelPos(arm);
      labels += '<text x="' + f1(lp.x) + '" y="' + f1(lp.y) +
        '" text-anchor="middle" font-size="9" font-weight="700" fill="' + esc(hexMix(col, -0.35)) + '">' +
        esc(clipName(pl && pl.label ? pl.label : '')) + '</text>';
    }

    let track = '';
    for (let i = 0; i < ringPts.length; i++) {
      const p = ringPts[i];
      const arm = Math.floor(i / cpp) % n;
      const isStart = (i % cpp) === 0;
      const col = armColor(arm);
      if (isStart) {
        track += '<circle cx="' + f1(p.x) + '" cy="' + f1(p.y) + '" r="' + f1(cellR + 1.2) +
          '" fill="' + esc(col) + '" stroke="#fffaf0" stroke-width="2.2"/>';
        track += '<text x="' + f1(p.x) + '" y="' + f1(p.y + 3.4) +
          '" text-anchor="middle" font-size="8" font-weight="700" fill="#fffaf0">A</text>';
      } else {
        track += '<circle cx="' + f1(p.x) + '" cy="' + f1(p.y) + '" r="' + f1(cellR) +
          '" fill="#fffaf0" stroke="#6a4530" stroke-width="1.45"/>';
      }
    }

    const spikes = classic ? 4 : n;
    let starPts = '';
    for (let i = 0; i < spikes * 2; i++) {
      const r = i % 2 === 0 ? 10 : 4.4;
      const a = -Math.PI / 2 + i * Math.PI / spikes;
      starPts += f1(cx + r * Math.cos(a)) + ',' + f1(cy + r * Math.sin(a)) + ' ';
    }
    const center =
      '<circle cx="' + cx + '" cy="' + cy + '" r="17" fill="#efe0c0" stroke="#c4a56a" stroke-width="1.8"/>' +
      '<circle cx="' + cx + '" cy="' + cy + '" r="12.5" fill="none" stroke="#8b2e1f" stroke-width="1.2" opacity="0.55"/>' +
      '<polygon points="' + starPts + '" fill="#e8b923" stroke="#8a5a18" stroke-width="0.9"/>';

    const yardGroups = {};
    players.forEach(function (pl, i) {
      if (!pl.inYard) return;
      const k = String(armOf(pl, i));
      if (!yardGroups[k]) yardGroups[k] = [];
      yardGroups[k].push(pl);
    });

    let pawns = '';
    players.forEach(function (pl, i) {
      const arm = armOf(pl, i);
      let pos;
      if (pl.finished) {
        pos = homePos(arm, homeLen);
      } else if (pl.inYard) {
        const group = yardGroups[String(arm)] || [pl];
        pos = yardPos(arm, Math.max(0, group.indexOf(pl)));
      } else if (pl.home > 0) {
        pos = homePos(arm, pl.home);
      } else {
        const cell = ((Number(pl.ringCell) || 0) % ringPts.length + ringPts.length) % ringPts.length;
        pos = ringPts[cell];
      }
      const mine = Number(pl.rangeNum) === Number(focusRange);
      pawns += hatMarkup(pos.x, pos.y, pl.color || armColor(arm), String(pl.rangeNum || ''), mine, !!pl.finished);
    });

    return '<svg class="md-board-svg" viewBox="0 0 ' + VB + ' ' + VB +
      '" preserveAspectRatio="xMidYMid meet" aria-label="Mensch ärgere dich nicht" ' +
      'font-family="Segoe UI, system-ui, sans-serif">' +
      defs + wood + figure + pads + lanes + ribbon + yards + homes + track + center + labels + pawns +
      '</svg>';
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

  /** Keep #f1-race-master-host identity; never wipe host className to only md-view. */
  function setMdSurfaceClasses(container, mode) {
    if (!container) return;
    container.classList.add('range-plugin-view', 'md-view');
    if (container.id === 'f1-race-master-host') {
      container.classList.add('f1-race-master-host');
    }
    if (mode === 'master') {
      container.classList.add('md-race-master');
      container.classList.remove('md-race-shooter');
    } else {
      container.classList.add('md-race-shooter');
      container.classList.remove('md-race-master');
    }
  }

  function beginPaint(container) {
    const gen = (container._mdPaintGen || 0) + 1;
    container._mdPaintGen = gen;
    return gen;
  }

  function mdPaintStale(container, gen) {
    return !container || container._mdPaintGen !== gen;
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
    setMdSurfaceClasses(container, isMaster ? 'master' : 'shooter');

    const layoutCls = isMaster ? 'md-master-layout' : 'md-shooter-layout';
    const otherCls = isMaster ? 'md-shooter-layout' : 'md-master-layout';
    if (container.querySelector('.' + otherCls) || !container.querySelector('.' + layoutCls)) {
      container.innerHTML =
        '<div class="' + layoutCls + '">' +
        '<header class="md-header" data-header></header>' +
        '<div class="md-main">' +
        '<div class="md-target-col">' +
        '<div class="md-scheibe-wrap" data-scheibe></div>' +
        (isMaster ? '' : '<div class="md-shot-hud" data-shothud></div>') +
        '<div data-recent></div>' +
        '<div data-standings></div>' +
        '</div>' +
        '<div class="md-board-col">' +
        '<div class="md-board-host" data-board></div>' +
        '</div></div>' +
        '<footer class="md-footer" data-footer></footer></div>';
    }

    await ensureTargetRegistry(assetsBase);
    if (mdPaintStale(container, paintGen)) return;
    const core = window.SRCore;
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    const header = container.querySelector('[data-header]');
    if (!header) return;
    header.innerHTML =
      '<span class="md-title">Mensch ärgere dich nicht</span>' +
      '<span class="md-badge">' + esc(PHASE_LABEL[game.phase] || game.phase || '') + '</span>' +
      (isShooter
        ? '<span class="md-meta">Stand ' + esc(focusRange) +
          (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>'
        : '') +
      '<span class="md-status">' + esc(game.statusLine || '') + '</span>';

    await renderScheibe(
      container.querySelector('[data-scheibe]'),
      assetsBase, game, isMaster ? null : focusRange,
      isMaster ? null : rangeDataFor(vm, focusRange), me
    );
    if (mdPaintStale(container, paintGen)) return;

    if (!isMaster) {
      const hud = container.querySelector('[data-shothud]');
      if (hud) {
        hud.innerHTML = shotBlock('Dein letzter Schuss', vm.lastOwn) +
          shotBlock('Letzter Fremdschuss', vm.lastForeign);
      }
    }
    container.querySelector('[data-recent]').innerHTML = recentHtml(game, isMaster ? null : focusRange);
    container.querySelector('[data-standings]').innerHTML =
      '<div class="md-panel"><h3>Stand</h3>' + standingsHtml(game, isMaster ? null : focusRange) + '</div>';
    container.querySelector('[data-board]').innerHTML =
      (game.startBlockedReason ? '<div class="md-block">' + esc(game.startBlockedReason) + '</div>' : '') +
      boardSvg(game, isMaster ? null : focusRange);
    const footer = container.querySelector('[data-footer]');
    if (footer) {
      footer.innerHTML = '<span class="md-meta">10.0 setzt ein · 9+ ein Feld · 10.5 zwei · landen schickt in den Hof · 6–7 tun nichts</span>';
    }
    if (container.id === 'f1-race-master-host') {
      container._sharedReady = true;
    }
  }

  window.SRPluginViews[PLUGIN_ID] = render;
  window.SRPlugins[PLUGIN_ID] = { render: render };
})();
