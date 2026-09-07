window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  let audioCtx = null;
  let lastEventSig = '';

  const PHASE_LABEL = {
    warmup: 'Einschießen',
    arming: 'Bereit',
    playing: 'Tannebaum',
    finished: 'Beendet'
  };

  const STAGE_COLORS = {
    A: { fill: '#2f6b3a', stroke: '#1e4228', text: '#f7f3ea' },
    B: { fill: '#b33a2b', stroke: '#7a241c', text: '#fff8f0' },
    C: { fill: '#d4a017', stroke: '#8a6a0a', text: '#1e2f24' }
  };

  // Ornament seats on the Tannebaum (drawing coords; SVG viewBox has padding).
  const BUBBLE_SEATS = [
    { stage: 'C', value: 10.9, x: 110, y: 42, r: 16 },
    { stage: 'C', value: 10.8, x: 86, y: 68, r: 14 },
    { stage: 'C', value: 10.7, x: 134, y: 68, r: 14 },
    { stage: 'C', value: 10.6, x: 68, y: 94, r: 13 },
    { stage: 'C', value: 10.5, x: 152, y: 94, r: 13 },
    { stage: 'B', value: 10.0, x: 110, y: 96, r: 14 },
    { stage: 'B', value: 9.5, x: 78, y: 122, r: 13 },
    { stage: 'B', value: 9.0, x: 142, y: 122, r: 13 },
    { stage: 'B', value: 8.5, x: 58, y: 148, r: 13 },
    { stage: 'B', value: 8.0, x: 162, y: 148, r: 13 },
    { stage: 'A', value: 10.0, x: 110, y: 148, r: 13 },
    { stage: 'A', value: 9.0, x: 84, y: 174, r: 13 },
    { stage: 'A', value: 8.0, x: 136, y: 174, r: 13 },
    { stage: 'A', value: 7.0, x: 62, y: 200, r: 13 },
    { stage: 'A', value: 6.0, x: 110, y: 200, r: 13 },
    { stage: 'A', value: 5.0, x: 158, y: 200, r: 13 }
  ];

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function ensureAudio() {
    if (window.SRAudio && typeof window.SRAudio.ensure === 'function') {
      audioCtx = window.SRAudio.ensure();
      return audioCtx;
    }
    if (!audioCtx) {
      try { audioCtx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) { /* */ }
    }
    if (audioCtx && audioCtx.state === 'suspended' && audioCtx.resume) {
      audioCtx.resume().catch(function () {});
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

  function playCue(name, opts, fallback) {
    if (window.SRAudio && typeof window.SRAudio.playOr === 'function') {
      window.SRAudio.playOr(name, opts, fallback);
      return;
    }
    if (fallback) fallback();
  }

  function playEvents(events, focusRange) {
    if (!events || !events.length) return;
    const sig = JSON.stringify(events);
    if (sig === lastEventSig) return;
    lastEventSig = sig;
    let winPlayed = false;
    events.forEach(function (ev) {
      if (!ev || !ev.type) return;
      const rn = ev.data && Number(ev.data.rangeNum);
      if (ev.type === 'strike') {
        if (ev.data && ev.data.gift) beep(420, 0.14, 'triangle', 0.07);
        else if (!focusRange || rn === focusRange) {
          playCue('hit-glass', {}, function () { beep(880, 0.12, 'sine', 0.09); });
        } else {
          playCue('hit-glass-far', { fallbackName: 'hit-glass', fallbackGain: 0.45 }, function () {
            beep(620, 0.08, 'sine', 0.05);
          });
        }
      } else if (ev.type === 'miss') {
        if (!focusRange || rn === focusRange) beep(160, 0.18, 'square', 0.04);
      } else if (ev.type === 'match_finished' || ev.type === 'cleared') {
        if (winPlayed) return;
        winPlayed = true;
        playCue('bingo', {}, function () {
          beep(523, 0.12); setTimeout(function () { beep(659, 0.14); }, 120);
          setTimeout(function () { beep(784, 0.2); }, 260);
        });
      } else if (ev.type === 'match_start') {
        beep(500, 0.1); setTimeout(function () { beep(700, 0.15); }, 100);
      }
    });
  }

  function ensureTargetRegistry(pluginId, assetsBase) {
    return new Promise(function (resolve) {
      if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === pluginId) {
        resolve();
        return;
      }
      const root = (assetsBase || ('/plugins/' + pluginId + '/assets')).replace(/\/assets\/?$/, '/');
      const s = document.createElement('script');
      s.src = root + 'target-registry.js?t=' + Date.now();
      s.onload = function () {
        if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = pluginId;
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

  function resolveProfileId(tree, rangeData, me) {
    const def = (tree && tree.defaultTargetProfile) || 'air_rifle_10m';
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

  async function renderScheibe(host, assetsBase, tree, focusRange, rangeData, me) {
    if (!host) return;
    const profileId = resolveProfileId(tree, rangeData, me);
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
          svg.classList.add('tb-scheibe-svg');
        }
      } catch (e) {
        host.innerHTML = '<div class="tb-muted">Scheibe nicht ladbar</div>';
        return;
      }
    }
    const svg = host.querySelector('svg');
    if (!svg) return;
    let g = svg.querySelector('#tb-shots');
    if (!g) {
      g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('id', 'tb-shots');
      svg.appendChild(g);
    }
    g.innerHTML = '';
    const shots = (tree && tree.recentShots) || [];
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

  function fmt1(n) {
    if (n == null || !isFinite(Number(n))) return '—';
    return (Math.round(Number(n) * 10) / 10).toFixed(1);
  }

  function leafState(contender, stageId, value) {
    const stages = (contender && contender.stages) || {};
    const stage = stages[stageId];
    const leaves = (stage && stage.leaves) || [];
    for (let i = 0; i < leaves.length; i++) {
      if (Math.abs(Number(leaves[i].value) - Number(value)) < 1e-6) {
        return leaves[i];
      }
    }
    return { value: value, label: fmt1(value), remaining: 0, cleared: true };
  }

  function labelFor(value) {
    const v = Number(value);
    if (Math.abs(v - Math.round(v)) < 1e-6) return String(Math.round(v));
    return fmt1(v);
  }

  function treeSvgMarkup(contender, opts) {
    opts = opts || {};
    const mini = !!opts.mini;
    const highlight = opts.highlight || null; // {stageId, value}
    const accent = (contender && contender.color) || '#2f6b3a';
    const finished = contender && contender.finished;
    const bubbles = BUBBLE_SEATS.map(function (seat) {
      const lf = leafState(contender, seat.stage, seat.value);
      const cleared = !!(lf && lf.cleared);
      const col = STAGE_COLORS[seat.stage] || STAGE_COLORS.A;
      const hit = highlight && highlight.stageId === seat.stage &&
        Math.abs(Number(highlight.value) - Number(seat.value)) < 1e-6;
      const cls = ['tb-bubble']
        .concat(cleared ? ['is-cleared'] : ['is-open'])
        .concat(hit ? ['is-hit'] : [])
        .join(' ');
      const fill = cleared ? 'rgba(40,55,45,0.28)' : col.fill;
      const stroke = cleared ? 'rgba(40,55,45,0.35)' : col.stroke;
      const text = cleared ? 'rgba(247,243,234,0.45)' : col.text;
      const lbl = labelFor(seat.value);
      const fontSize = lbl.length > 3 ? Math.max(7, seat.r * 0.7) : Math.max(8, seat.r * 0.85);
      return '<g class="' + cls + '" transform="translate(' + seat.x + ' ' + seat.y + ')">' +
        '<circle class="tb-bubble-orb" r="' + seat.r + '" fill="' + fill + '" stroke="' + stroke + '" stroke-width="2"/>' +
        '<circle class="tb-bubble-shine" cx="' + (-seat.r * 0.28) + '" cy="' + (-seat.r * 0.32) +
        '" r="' + (seat.r * 0.28) + '" fill="rgba(255,255,255,0.35)"/>' +
        '<text class="tb-label" text-anchor="middle" dominant-baseline="central" fill="' + text +
        '" font-size="' + fontSize + '" font-weight="700">' +
        esc(lbl) + '</text></g>';
    }).join('');

    const plateLabel = esc((contender && contender.label) || '') +
      (finished ? ' · geräumt!' : '');
    // Nameplate sits on the trunk/shadow so the viewBox is only the tree.
    const caption =
      '<g class="tb-tree-nameplate">' +
      '<rect x="34" y="252" width="152" height="22" rx="11" fill="rgba(18,28,20,0.78)"/>' +
      '<circle cx="48" cy="263" r="4" fill="' + esc(accent) +
      '" stroke="#fffdf8" stroke-width="1.2"/>' +
      '<text class="tb-label tb-tree-caption" x="122" y="267" text-anchor="middle" dominant-baseline="middle">' +
      plateLabel + '</text></g>';

    return '<svg class="tb-tree-svg' + (mini ? ' is-mini' : '') + (finished ? ' is-finished' : '') +
      '" viewBox="-10 -6 240 282" preserveAspectRatio="xMidYMid meet" aria-label="Tannebaum">' +
      '<defs>' +
      '<linearGradient id="tb-needles-' + esc(contender && contender.id || 'x') + '" x1="0" y1="0" x2="0" y2="1">' +
      '<stop offset="0%" stop-color="#3f8f4a"/><stop offset="55%" stop-color="#246334"/><stop offset="100%" stop-color="#1a4726"/>' +
      '</linearGradient></defs>' +
      '<ellipse cx="110" cy="268" rx="54" ry="10" fill="rgba(20,30,22,0.25)"/>' +
      '<rect x="100" y="232" width="20" height="36" rx="3" fill="#6b3e26"/>' +
      '<path fill="url(#tb-needles-' + esc(contender && contender.id || 'x') + ')" d="' +
      'M110 18 L158 78 L138 78 L178 138 L150 138 L198 210 L22 210 L70 138 L42 138 L82 78 L62 78 Z"/>' +
      '<path fill="none" stroke="rgba(255,255,255,0.18)" stroke-width="2" d="' +
      'M110 28 L148 78 M110 28 L72 78 M95 88 L168 138 M125 88 L52 138"/>' +
      '<polygon points="110,10 118,28 102,28" fill="#f0d060" stroke="#c9a227" stroke-width="1"/>' +
      bubbles +
      caption +
      '</svg>';
  }

  function lastHighlight(tree, contenderId) {
    const shots = (tree && tree.recentShots) || [];
    for (let i = shots.length - 1; i >= 0; i--) {
      const s = shots[i];
      if (!s || (s.result !== 'own' && s.result !== 'gift')) continue;
      if (contenderId && s.targetId !== contenderId && s.contenderId !== contenderId) continue;
      if (s.result === 'gift' && s.targetId !== contenderId) continue;
      if (s.result === 'own' && s.contenderId !== contenderId && s.targetId !== contenderId) continue;
      return { stageId: s.stageId, value: s.mapped };
    }
    return null;
  }

  function shotBlock(title, shot) {
    if (!shot) {
      return '<div class="tb-panel tb-panel-solid"><h3>' + esc(title) + '</h3><div class="tb-muted">—</div></div>';
    }
    const res = shot.result === 'gift' ? 'Geschenk' : (shot.result === 'own' ? 'Treffer' : 'Fehl');
    return '<div class="tb-panel tb-panel-solid"><h3>' + esc(title) + '</h3>' +
      '<div class="tb-shot-val">' + esc(fmt1(shot.raw)) + '</div>' +
      '<div class="tb-muted">' + esc(res) +
      (shot.stageId ? ' · Stufe ' + esc(shot.stageId) + ' → ' + esc(fmt1(shot.mapped)) : '') +
      (shot.rangeNum != null ? ' · Stand ' + esc(shot.rangeNum) : '') +
      '</div></div>';
  }

  function recentHtml(tree, focusRange) {
    const shots = (tree && tree.recentShots) || [];
    if (!shots.length) {
      return '<div class="tb-panel tb-panel-solid"><h3>Letzte Schüsse</h3><div class="tb-muted">Noch keine</div></div>';
    }
    const items = shots.slice().reverse().map(function (s) {
      const mine = Number(s.rangeNum) === Number(focusRange);
      const res = s.result === 'gift' ? 'Geschenk' : (s.result === 'own' ? 'Treffer' : 'Fehl');
      return '<li class="' + (mine ? 'is-me' : '') + '">' +
        '<span class="tb-swatch" style="background:' + esc(s.color || '#888') + '"></span>' +
        '<span>S' + esc(s.rangeNum) + ' · ' + esc(res) + '</span>' +
        '<strong>' + esc(fmt1(s.raw)) + '</strong>' +
        '<span>' + (s.stageId ? (esc(s.stageId) + '→' + esc(fmt1(s.mapped))) : '—') + '</span></li>';
    }).join('');
    return '<div class="tb-panel tb-panel-solid"><h3>Letzte Schüsse</h3><ul class="tb-recent">' + items + '</ul></div>';
  }

  function standingsHtml(tree, focusContenderId) {
    const list = ((tree && tree.contenders) || []).slice().sort(function (a, b) {
      if (!!a.finished !== !!b.finished) return a.finished ? -1 : 1;
      if ((a.finishOrder || 0) !== (b.finishOrder || 0) && a.finished && b.finished) {
        return (a.finishOrder || 0) - (b.finishOrder || 0);
      }
      return (a.remaining || 0) - (b.remaining || 0);
    });
    return '<ul class="tb-standings">' + list.map(function (c) {
      const cls = [].concat(c.id === focusContenderId ? ['is-me'] : [])
        .concat(c.finished ? ['is-done'] : []).join(' ');
      return '<li class="' + cls + '">' +
        '<span class="tb-swatch" style="background:' + esc(c.color) + '"></span>' +
        '<span>' + esc(c.label) + '</span>' +
        '<strong>' + (c.finished ? '✓' : esc(c.remaining)) + '</strong>' +
        '<span>Stufe ' + esc(c.currentStage || '—') + '</span></li>';
    }).join('') + '</ul>';
  }

  function hudHtml(tree) {
    // Header already shows phase + status; footer shows the A/B/C legend.
    return (tree && tree.startBlockedReason)
      ? '<div class="tb-block">' + esc(tree.startBlockedReason) + '</div>'
      : '';
  }

  function legendHtml() {
    return '<span class="tb-legend">' +
      '<i style="background:#2f6b3a"></i>A <i style="background:#b33a2b"></i>B <i style="background:#d4a017"></i>C' +
      '</span>';
  }

  function treeFrameHtml(c, tree, focusContenderId, mini) {
    if (!c) return '';
    const cls = ['tb-tree-frame']
      .concat(mini ? ['is-mini'] : [])
      .concat(c.finished ? ['is-winner'] : [])
      .concat(c.id === focusContenderId ? ['is-me'] : [])
      .join(' ');
    return '<div class="' + cls + '">' +
      treeSvgMarkup(c, { mini: !!mini, highlight: lastHighlight(tree, c.id) }) +
      '</div>';
  }

  function renderTreesHost(host, tree, focusContenderId, mode) {
    if (!host) return;
    const contenders = (tree && tree.contenders) || [];
    if (!contenders.length) {
      host.innerHTML = '<div class="tb-muted">Kein Baum</div>';
      return;
    }

    // Team (or only two trees): always equal side-by-side — never primary+mini.
    if (mode === 'team' || contenders.length <= 2) {
      host.innerHTML = '<div class="tb-trees-row tb-trees-duo">' +
        contenders.map(function (c) {
          return treeFrameHtml(c, tree, focusContenderId, false);
        }).join('') + '</div>';
      return;
    }

    // Hall has no "own" stand — show every tree at the same size. The old
    // primary + mini row left Stands 2–6 tiny and clipped at the bottom-left.
    if (!focusContenderId) {
      const cols = contenders.length <= 4 ? 2 : 3;
      host.innerHTML = '<div class="tb-trees-gallery tb-trees-gallery-' + cols + '">' +
        contenders.map(function (c) {
          return treeFrameHtml(c, tree, null, false);
        }).join('') + '</div>';
      return;
    }

    // Shooter: own tree large, others as a readable row that is not clipped.
    let primary = contenders.find(function (c) { return c.id === focusContenderId; }) || contenders[0];
    const others = contenders.filter(function (c) { return c.id !== primary.id; });
    host.innerHTML =
      '<div class="tb-tree-frame is-primary' +
      (primary.finished ? ' is-winner' : '') +
      (primary.id === focusContenderId ? ' is-me' : '') + '">' +
      treeSvgMarkup(primary, { highlight: lastHighlight(tree, primary.id) }) +
      '</div>' +
      (others.length
        ? '<div class="tb-trees-mini">' + others.map(function (c) {
          return treeFrameHtml(c, tree, focusContenderId, true);
        }).join('') + '</div>'
        : '');
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

  function renderView(pluginId) {
    return async function (container, viewModel, assetsBase) {
      const vm = viewModel || {};
      const tree = vm.tree || {};
      const isShooter = document.body.classList.contains('shooter-display') ||
        (window.SRDisplay && window.SRDisplay.display === 'shooter');
      const isCompact = window.SRDisplay && window.SRDisplay.display === 'compact';
      const focusRange = isShooter ? (Number(vm.rangeNum) || 0) : 0;
      const isMaster = !isShooter && !isCompact;
      const myTree = vm.myTree || null;
      const focusContenderId = myTree && myTree.id;
      const modeLabel = tree.gameMode === 'team' ? 'Team' : 'Einzel';
      const mode = tree.gameMode || 'einzel';

      playEvents(vm.events || [], focusRange || null);
      await ensureTargetRegistry(pluginId, assetsBase);
      const core = window.SRCore;
      if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

      container.classList.add('range-plugin-view', 'tb-view');
      if (container.id === 'shared-master-host') {
        container.classList.add('shared-master-host');
      }
      if (isShooter) {
        container.classList.add('tb-race-shooter');
        container.classList.remove('tb-race-master');
      } else {
        container.classList.add('tb-race-master');
        container.classList.remove('tb-race-shooter');
      }

      if (isMaster || isCompact) {
        const layoutCls = isCompact ? 'tb-compact-layout' : 'tb-master-layout';
        if (!container.querySelector('.' + layoutCls)) {
          container.innerHTML =
            '<div class="' + layoutCls + '">' +
            '<header class="tb-header" data-header></header>' +
            '<div class="tb-main">' +
            '<div class="tb-target-col">' +
            (isCompact ? '' : '<div class="tb-scheibe-wrap" data-scheibe></div>') +
            '<div data-recent></div>' +
            '<div data-standings></div>' +
            '</div>' +
            '<div class="tb-tree-col">' +
            '<div data-hud></div>' +
            '<div class="tb-tree-host" data-trees></div>' +
            '</div></div>' +
            '<footer class="tb-footer" data-footer></footer></div>';
        }
        container.querySelector('[data-header]').innerHTML =
          '<span class="tb-title">Tannebaum · ' + esc(modeLabel) + '</span>' +
          '<span class="tb-badge">' + esc(PHASE_LABEL[tree.phase] || tree.phase || '') + '</span>' +
          '<span class="tb-status">' + esc(tree.statusLine || '') + '</span>';
        container.querySelector('[data-hud]').innerHTML = hudHtml(tree);
        renderTreesHost(container.querySelector('[data-trees]'), tree, null, mode);
        const scheibe = container.querySelector('[data-scheibe]');
        if (scheibe) {
          await renderScheibe(scheibe, assetsBase, tree, null, null, null);
        }
        container.querySelector('[data-recent]').innerHTML = recentHtml(tree, null);
        container.querySelector('[data-standings]').innerHTML =
          '<div class="tb-panel tb-panel-solid"><h3>Stand</h3>' + standingsHtml(tree, null) + '</div>';
        container.querySelector('[data-footer]').innerHTML =
          '<span class="tb-meta">Grün = A · Rot = B · Gold = C · durchgestrichen = geräumt</span>';
        if (container.id === 'shared-master-host') {
          container._sharedReady = true;
        }
        return;
      }

      if (!container.querySelector('.tb-shooter-layout')) {
        container.innerHTML =
          '<div class="tb-shooter-layout">' +
          '<header class="tb-header" data-header></header>' +
          '<div class="tb-main">' +
          '<div class="tb-target-col">' +
          '<div class="tb-scheibe-wrap" data-scheibe></div>' +
          '<div class="tb-shot-hud" data-shothud></div>' +
          '<div data-recent></div>' +
          '<div data-standings></div>' +
          '</div>' +
          '<div class="tb-tree-col">' +
          '<div data-hud></div>' +
          '<div class="tb-tree-host" data-trees></div>' +
          '</div></div>' +
          '<footer class="tb-footer" data-footer></footer></div>';
      }

      const me = vm.me;
      container.querySelector('[data-header]').innerHTML =
        '<span class="tb-title">Tannebaum · ' + esc(modeLabel) + '</span>' +
        '<span class="tb-badge">' + esc(PHASE_LABEL[tree.phase] || tree.phase || '') + '</span>' +
        '<span class="tb-meta">Stand ' + esc(focusRange) +
        (me && me.shooterName ? ' · ' + esc(me.shooterName) : '') + '</span>' +
        '<span class="tb-status">' + esc(tree.statusLine || '') + '</span>';
      container.querySelector('[data-hud]').innerHTML = hudHtml(tree);
      renderTreesHost(container.querySelector('[data-trees]'), tree, focusContenderId, mode);
      await renderScheibe(
        container.querySelector('[data-scheibe]'),
        assetsBase, tree, focusRange, rangeDataFor(vm, focusRange), me
      );
      container.querySelector('[data-shothud]').innerHTML =
        shotBlock('Dein letzter Schuss', vm.lastOwn) +
        shotBlock('Letzter Fremdschuss', vm.lastForeign);
      container.querySelector('[data-recent]').innerHTML = recentHtml(tree, focusRange);
      container.querySelector('[data-standings]').innerHTML =
        '<div class="tb-panel tb-panel-solid"><h3>Stand</h3>' + standingsHtml(tree, focusContenderId) + '</div>';
      container.querySelector('[data-footer]').innerHTML =
        '<span class="tb-meta">Offen: ' + esc(myTree && myTree.remaining != null ? myTree.remaining : '—') +
        ' · Fokus Stufe ' + esc((myTree && myTree.currentStage) || '—') + '</span>' +
        legendHtml();
    };
  }

  window.SRPluginViews['tannebaum-einzel'] = renderView('tannebaum-einzel');
})();
