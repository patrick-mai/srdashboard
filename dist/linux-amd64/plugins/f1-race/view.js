window.SRPlugins = window.SRPlugins || {};
window.SRPluginViews = window.SRPluginViews || {};

(function () {
  const CIRCUIT_FILES = {
    spa: 'circuits/spa.svg',
    nuerburgring: 'circuits/nuerburgring.svg',
    melbourne: 'circuits/melbourne.svg',
    nordschleife: 'circuits/nordschleife.svg'
  };

  // Last displayed progress keyed by container+range so master and shooter
  // do not fight over the same lerp state.
  const carProgCacheByHost = new WeakMap();
  let audioCtx = null;
  let lastEventSig = '';
  let lastPitCueAt = '';
  let lastCountdownSec = null;

  function progCacheFor(host) {
    let cache = carProgCacheByHost.get(host);
    if (!cache) {
      cache = {};
      carProgCacheByHost.set(host, cache);
    }
    return cache;
  }

  function ensureAudio() {
    if (!audioCtx) {
      try { audioCtx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) { /* ignore */ }
    }
    return audioCtx;
  }

  function beep(freq, dur, type, gain) {
    const ctx = ensureAudio();
    if (!ctx) return;
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = type || 'square';
    o.frequency.value = freq;
    g.gain.value = gain == null ? 0.08 : gain;
    o.connect(g);
    g.connect(ctx.destination);
    o.start();
    g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + dur);
    o.stop(ctx.currentTime + dur);
  }

  function beepSeq(notes) {
    let t = 0;
    notes.forEach(function (n) {
      setTimeout(function () { beep(n.f, n.d, n.type, n.g); }, t);
      t += (n.gap != null ? n.gap : Math.round(n.d * 1000) + 30);
    });
  }

  function playEventSounds(events, focusRange) {
    if (!events || !events.length) return;
    events.forEach(function (ev) {
      if (!ev || !ev.type) return;
      const rn = ev.data && Number(ev.data.rangeNum);
      // Range-scoped cues only play on that shooter (or on master when no focus).
      if (focusRange && rn && rn !== focusRange) return;

      if (ev.type === 'pit_cue' || ev.type === 'field_event') {
        if (ev.type === 'field_event' && focusRange) {
          const targets = (ev.data && ev.data.targets) || [];
          const hit = targets.some(function (t) { return Number(t) === focusRange; });
          // oil_leak targets everyone; puncture only the victim hears the urgent cue
          if (targets.length && !hit) return;
        }
        beepSeq([
          { f: 880, d: 0.18, type: 'sawtooth' },
          { f: 660, d: 0.18, type: 'sawtooth' },
          { f: 990, d: 0.28, type: 'sawtooth' }
        ]);
      } else if (ev.type === 'pit_ok') beep(1200, 0.15, 'sine');
      else if (ev.type === 'pit_slow' || ev.type === 'pit_fail' || ev.type === 'miss') beep(220, 0.35, 'triangle');
      else if (ev.type === 'crash') beep(120, 0.5, 'triangle');
      else if (ev.type === 'hole_in_hole' || ev.type === 'streak_bonus') beep(1400, 0.2, 'sine');
      else if (ev.type === 'drs_active') {
        if (ev.data && ev.data.passed) {
          beepSeq([
            { f: 920, d: 0.08, type: 'sine' },
            { f: 1240, d: 0.1, type: 'sine' },
            { f: 1560, d: 0.18, type: 'sine', g: 0.11 }
          ]);
        } else {
          beepSeq([
            { f: 740, d: 0.1, type: 'triangle' },
            { f: 980, d: 0.16, type: 'triangle' }
          ]);
        }
      } else if (ev.type === 'overtake' && !(ev.data && ev.data.viaDRS)) {
        beep(540, 0.12, 'square');
      } else if (ev.type === 'race_start') {
        beep(600, 0.1);
        setTimeout(function () { beep(900, 0.2); }, 150);
      }
    });
  }

  function pitCountdownSec(race) {
    if (!race || !race.pitCueAt) return null;
    if (race.fieldEvent && race.fieldEvent.pending) {
      // Field events reuse pitCueAt; show their own banner instead.
      return null;
    }
    if (race.fieldEvent && !race.fieldEvent.cleared && race.fieldEvent.affectsMe) {
      return null;
    }
    if (!race.isPitRound && race.stintPitRound !== race.currentRound) return null;
    const start = Date.parse(race.pitCueAt);
    if (!isFinite(start)) return null;
    const windowMs = Number(race.pitWindowMs) > 0 ? Number(race.pitWindowMs) : 5000;
    const left = Math.ceil((start + windowMs - Date.now()) / 1000);
    if (left < 0) return 0;
    return left;
  }

  function pitBannerHtml(race) {
    const sec = pitCountdownSec(race);
    if (sec == null) return '';
    const label = sec > 0 ? ('PIT — ' + sec) : 'PIT — JETZT';
    return '<div class="f1-event-banner f1-pit-banner" data-pit-count="' + sec + '">' +
      esc(label) + '</div>';
  }

  function syncPitCountdownAudio(race) {
    const sec = pitCountdownSec(race);
    if (sec == null) {
      lastCountdownSec = null;
      return;
    }
    if (lastCountdownSec === sec) return;
    const prev = lastCountdownSec;
    lastCountdownSec = sec;
    // Tick only when the shared countdown steps down (same for all ranges).
    if (prev == null) return;
    if (sec >= 1 && sec <= 3) beep(700 + (3 - sec) * 120, 0.1, 'square', 0.1);
    else if (sec === 0) beep(1100, 0.22, 'sawtooth', 0.12);
  }

  function updatePitOverlay(overlayEl, race) {
    if (!overlayEl) return;
    const field = race && race.fieldEvent;
    if (field && !field.cleared) {
      overlayEl.innerHTML = fieldBannerHtml(race, null);
      return;
    }
    overlayEl.innerHTML = pitBannerHtml(race);
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function phaseLabel(p) {
    return ({
      warmup_collect: 'Probe',
      arming: 'Bereitmachen',
      racing: 'Rennen',
      finished: 'Ziel'
    })[p] || p || '—';
  }

  function fieldLabel(t, field) {
    if (t === 'puncture') {
      const targets = (field && field.targets) || [];
      if (targets.length === 1) {
        return 'Reifenplatzer Bahn ' + targets[0] + ' — nächster Schuss PIT';
      }
      return 'Reifenplatzer — nächster Schuss PIT';
    }
    if (t === 'oil_leak') return 'Ölverlust — nächster Schuss PIT';
    return t || '';
  }

  function fieldBannerHtml(race, focusRange) {
    const field = race && race.fieldEvent;
    if (!field) return '';
    const pending = field.pending || (field.affectsMe && !field.cleared);
    if (focusRange != null && focusRange > 0) {
      if (pending) {
        const label = field.type === 'puncture'
          ? 'Reifenplatzer — nächster Schuss PIT'
          : (field.type === 'oil_leak' ? 'Ölverlust — nächster Schuss PIT' : fieldLabel(field.type, field));
        return '<div class="f1-event-banner">' + esc(label) + '</div>';
      }
      // Non-targets keep racing; no wait banner for puncture on another lane.
      return '';
    }
    return '<div class="f1-event-banner">' + esc(fieldLabel(field.type, field)) + '</div>';
  }

  function outcomeHtml(me) {
    if (!me) return '';
    const hint = me.nextHint;
    const note = me.lastNote;
    const place = me.placeReason;
    if (!hint && !note && !place) return '';
    const hintKind = me.nextHintKind || '';
    const kind = me.lastNoteKind || '';
    return '<div class="f1-explain">' +
      (hint ? '<div class="f1-next-hint kind-' + esc(hintKind) + '">' + esc(hint) + '</div>' : '') +
      (note ? '<div class="f1-outcome kind-' + esc(kind) + '">' + esc(note) + '</div>' : '') +
      (place ? '<div class="f1-place-reason">' + esc(place) + '</div>' : '') +
      '</div>';
  }

  // Circuit SVGs are static assets but were refetched on every render, which on
  // the shooter view meant one request per shot.
  const circuitCache = {};

  function loadCircuit(assetsBase, circuitId) {
    const key = (assetsBase || '') + '|' + circuitId;
    if (!circuitCache[key]) {
      circuitCache[key] = fetchCircuit(assetsBase, circuitId).catch(function (e) {
        delete circuitCache[key];
        throw e;
      });
    }
    return circuitCache[key];
  }

  async function fetchCircuit(assetsBase, circuitId) {
    const file = CIRCUIT_FILES[circuitId] || CIRCUIT_FILES.spa;
    const base = (assetsBase || '').replace(/\/?$/, '/');
    const bust = Date.now();
    const res = await fetch(base + file + '?t=' + bust);
    if (!res.ok) throw new Error('circuit load failed');
    let text = await res.text();
    // Rewrite relative image hrefs so backgrounds resolve from assetsBase
    text = text.replace(/href="([^"]+\.(?:png|jpg|webp))"/g, function (_, src) {
      if (/^https?:|^\//.test(src) && src.indexOf('?') !== -1) return 'href="' + src + '"';
      if (/^https?:|^\//.test(src)) return 'href="' + src + (src.indexOf('?') === -1 ? '?t=' + bust : '') + '"';
      const name = src.split('/').pop().split('?')[0];
      return 'href="' + base + 'circuits/' + name + '?t=' + bust + '"';
    });
    return text;
  }

  // Colours come from the server palette, but they land in SVG attributes built
  // by string concatenation, so anything unexpected falls back to the default.
  function safeColor(color) {
    return /^#[0-9a-fA-F]{3,8}$|^[a-zA-Z]{3,20}$/.test(color || '') ? color : '#e10600';
  }

  function carMarkup(color, num) {
    color = safeColor(color);
    return (
      '<g class="f1-car-visual" filter="url(#f1-car-shadow)">' +
      '<ellipse cx="0" cy="5" rx="26" ry="6" fill="#000" opacity="0.4"/>' +
      // rear wing
      '<rect class="f1-paint" x="-28" y="-9" width="5" height="18" rx="1" fill="' + color + '"/>' +
      '<rect class="f1-paint" x="-30" y="-11" width="9" height="2.5" rx="0.4" fill="' + color + '"/>' +
      '<rect class="f1-paint" x="-30" y="8.5" width="9" height="2.5" rx="0.4" fill="' + color + '"/>' +
      // rear tires
      '<rect x="-20" y="-13" width="9" height="4.5" rx="1" fill="#0d0d0f"/>' +
      '<rect x="-20" y="8.5" width="9" height="4.5" rx="1" fill="#0d0d0f"/>' +
      // body / sidepods
      '<path class="f1-paint" d="M-22,-6.5 L7,-7.5 L13,-3.5 L13,3.5 L7,7.5 L-22,6.5 Z" fill="' + color + '"/>' +
      // cockpit
      '<ellipse cx="-2" cy="0" rx="6.5" ry="4" fill="#0a0a10"/>' +
      '<ellipse cx="-1" cy="0" rx="3.5" ry="2.2" fill="#4af" opacity="0.4"/>' +
      // nose
      '<path class="f1-paint" d="M11,-3 L30,0 L11,3 Z" fill="' + color + '"/>' +
      // front wing
      '<rect class="f1-paint" x="26" y="-10" width="4.5" height="20" rx="1" fill="' + color + '"/>' +
      '<rect class="f1-paint" x="28" y="-12" width="7" height="2.2" rx="0.4" fill="' + color + '"/>' +
      '<rect class="f1-paint" x="28" y="9.8" width="7" height="2.2" rx="0.4" fill="' + color + '"/>' +
      // front tires
      '<rect x="16" y="-12" width="8" height="4" rx="1" fill="#0d0d0f"/>' +
      '<rect x="16" y="8" width="8" height="4" rx="1" fill="#0d0d0f"/>' +
      // gloss
      '<path d="M-18,-1.2 L9,-1.2 L11,0 L9,1.2 L-18,1.2 Z" fill="#fff" opacity="0.22"/>' +
      // number disc
      '<circle cx="3" cy="0" r="5.2" fill="#fff"/>' +
      '<text class="f1-car-num" x="3" y="3.2" text-anchor="middle" fill="#111" font-size="7" font-weight="700" font-family="Oswald,sans-serif">' +
      num + '</text>' +
      '</g>' +
      '<circle class="f1-car-crash" cx="0" cy="0" r="18" fill="none" stroke="#ff6b35" stroke-width="2.5" opacity="0"/>'
    );
  }

  function ensureCarShadow(svgRoot) {
    let defs = svgRoot.querySelector('defs');
    if (!defs) {
      defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
      svgRoot.insertBefore(defs, svgRoot.firstChild);
    }
    if (!defs.querySelector('#f1-car-shadow')) {
      const f = document.createElementNS('http://www.w3.org/2000/svg', 'filter');
      f.setAttribute('id', 'f1-car-shadow');
      f.setAttribute('x', '-50%');
      f.setAttribute('y', '-50%');
      f.setAttribute('width', '200%');
      f.setAttribute('height', '200%');
      f.innerHTML = '<feDropShadow dx="0" dy="2" stdDeviation="2.5" flood-color="#000" flood-opacity="0.7"/>';
      defs.appendChild(f);
    }
  }

  function placeCars(svgRoot, cars, smooth) {
    if (!svgRoot) return;
    const path = svgRoot.querySelector('#racing-line');
    if (!path || typeof path.getTotalLength !== 'function') return;
    ensureCarShadow(svgRoot);
    const len = path.getTotalLength();
    if (!(len > 0)) return;
    let layer = svgRoot.querySelector('#f1-cars-layer');
    if (!layer) {
      layer = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      layer.setAttribute('id', 'f1-cars-layer');
      svgRoot.appendChild(layer);
    }
    const carProgCache = progCacheFor(svgRoot);

    // Dedupe by rangeNum — a duplicated cars[] entry would draw two sprites with
    // the same number and look like a stacked pile.
    const byRange = {};
    (cars || []).forEach(function (c) {
      if (!c || c.rangeNum == null) return;
      byRange[c.rangeNum] = c;
    });
    const list = Object.keys(byRange).map(function (k) { return byRange[k]; }).sort(function (a, b) {
      const dp = (Number(b.progress) || 0) - (Number(a.progress) || 0);
      if (dp !== 0) return dp;
      return (a.rangeNum || 0) - (b.rangeNum || 0);
    });

    const scale = smooth ? 0.92 : 0.8;
    // Nose-to-tail of the sprite at this scale (~62 local units). Hairpins fold the
    // track so lap-fraction gaps alone are not enough — we also separate in pixels.
    const carLen = 62 * scale;
    const minFrac = Math.min(0.035, Math.max(0.01, (carLen * 1.2) / len));
    const minPx = carLen * 1.15;

    const placed = [];
    const slots = []; // {car, g, display, crashed}
    const seen = {};

    list.forEach(function (car) {
      const id = 'car-' + car.rangeNum;
      seen[id] = true;
      let g = layer.querySelector('#' + id);
      const color = safeColor(car.color);
      if (!g) {
        g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        g.setAttribute('id', id);
        g.innerHTML = carMarkup(color, String(car.rangeNum));
        layer.appendChild(g);
      } else {
        Array.prototype.forEach.call(g.querySelectorAll('.f1-paint'), function (el) {
          el.setAttribute('fill', color);
        });
        const num = g.querySelector('.f1-car-num');
        if (num) num.textContent = String(car.rangeNum);
      }
      const crash = g.querySelector('.f1-car-crash');
      if (crash) crash.setAttribute('opacity', car.status === 'crashed' ? '1' : '0');

      let target = Number(car.progress);
      if (!isFinite(target)) target = 0;

      const key = car.rangeNum;
      let display = target;
      if (smooth && carProgCache[key] != null && isFinite(carProgCache[key])) {
        display = carProgCache[key] + (target - carProgCache[key]) * 0.45;
      }
      const crashed = car.status === 'crashed';
      if (!crashed && placed.length) {
        const maxAllowed = placed[placed.length - 1] - minFrac;
        if (display > maxAllowed) display = maxAllowed;
      }
      if (!crashed) placed.push(display);
      carProgCache[key] = display;
      slots.push({ car: car, g: g, display: display, crashed: crashed });
    });

    // Push trailers back along the path until centres are far enough in SVG space.
    // Needed on hairpins where a large lap-fraction still maps to nearby pixels.
    function fracOf(prog) {
      let f = prog - Math.floor(prog);
      if (f < 0) f += 1;
      return f;
    }
    function distOf(prog) {
      return fracOf(prog) * len;
    }
    function xyAt(prog) {
      return path.getPointAtLength(distOf(prog));
    }
    for (let pass = 0; pass < 8; pass++) {
      let moved = false;
      for (let i = 1; i < slots.length; i++) {
        const ahead = slots[i - 1];
        const cur = slots[i];
        if (cur.crashed || ahead.crashed) continue;
        const pa = xyAt(ahead.display);
        const pb = xyAt(cur.display);
        const dx = pa.x - pb.x;
        const dy = pa.y - pb.y;
        const gapPx = Math.sqrt(dx * dx + dy * dy);
        if (gapPx >= minPx) continue;
        // Step the trailer backward along the lap until clear (or give up).
        let d = cur.display;
        for (let step = 0; step < 40; step++) {
          d -= minFrac * 0.15;
          const p = xyAt(d);
          const gdx = pa.x - p.x;
          const gdy = pa.y - p.y;
          if (Math.sqrt(gdx * gdx + gdy * gdy) >= minPx) break;
        }
        if (d < cur.display) {
          cur.display = d;
          carProgCache[cur.car.rangeNum] = d;
          moved = true;
        }
        // Keep progress order: each trailer still behind the one ahead.
        const maxAllowed = ahead.display - minFrac * 0.5;
        if (cur.display > maxAllowed) {
          cur.display = maxAllowed;
          carProgCache[cur.car.rangeNum] = cur.display;
          moved = true;
        }
      }
      if (!moved) break;
    }

    slots.forEach(function (slot, idx) {
      const dist = distOf(slot.display);
      const p1 = path.getPointAtLength(dist);
      const p2 = path.getPointAtLength((dist + 8) % len);
      const angle = Math.atan2(p2.y - p1.y, p2.x - p1.x) * 180 / Math.PI;
      slot.g.setAttribute('transform',
        'translate(' + p1.x + ',' + p1.y + ') rotate(' + angle + ') scale(' + scale + ')');
      slot.g.setAttribute('opacity', slot.crashed ? '0.5' : '1');
      slot.g.setAttribute('data-z', String(1000 - idx));
      if (slot.crashed) slot.g.classList.add('is-crashed');
      else slot.g.classList.remove('is-crashed');
    });

    Array.prototype.slice.call(layer.children)
      .sort(function (a, b) {
        return (Number(a.getAttribute('data-z')) || 0) - (Number(b.getAttribute('data-z')) || 0);
      })
      .forEach(function (ch) { layer.appendChild(ch); });
    Array.prototype.slice.call(layer.children).forEach(function (ch) {
      if (!seen[ch.id]) ch.remove();
    });
  }

  function renderTrackHost(host, svgText, race, focusRange, mini) {
    host.className = 'f1-track-host' + (mini ? ' f1-track-mini' : '');
    // Reparse the circuit only when it actually changes; placeCars updates the
    // existing car nodes in place, so a rebuild here would undo that work.
    if (host.__f1TrackSvg !== svgText) {
      host.innerHTML = svgText;
      host.__f1TrackSvg = svgText;
      // Drop eased positions so cars don't animate across circuit changes.
      const svg = host.querySelector('svg');
      if (svg) {
        const cache = progCacheFor(svg);
        Object.keys(cache).forEach(function (k) { delete cache[k]; });
      }
    }
    const svg = host.querySelector('svg');
    if (svg) {
      svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
      if (mini) svg.setAttribute('viewBox', svg.getAttribute('viewBox') || '0 0 1000 667');
      placeCars(svg, race && race.cars, !mini);
      svg.querySelectorAll('.f1-focus-ring').forEach(function (el) { el.remove(); });
      if (focusRange != null) {
        const el = svg.querySelector('#car-' + focusRange);
        if (el) {
          const pulse = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
          pulse.setAttribute('r', '22');
          pulse.setAttribute('fill', 'none');
          pulse.setAttribute('stroke', '#fff');
          pulse.setAttribute('stroke-width', '2');
          pulse.setAttribute('class', 'f1-focus-ring');
          el.insertBefore(pulse, el.firstChild);
        }
      }
    }
    return svg;
  }

  function standingsHtml(race, focusRange) {
    const cars = (race && race.cars ? race.cars.slice() : []).sort(function (a, b) {
      return (a.position || 99) - (b.position || 99);
    });
    return '<ol class="f1-standings f1-standings-slim">' + cars.map(function (c) {
      const cls = 'f1-standings-row' +
        (c.rangeNum === focusRange ? ' is-me' : '') +
        (c.status === 'crashed' ? ' is-crash' : '');
      const name = c.shooterName || ('Bahn ' + c.rangeNum);
      const shot = c.lastShotValue != null && c.lastShotValue > 0
        ? (Math.round(c.lastShotValue * 10) / 10).toFixed(1)
        : (c.lastBoostKind === 'miss' ? '0' : '—');
      const reason = (c.rangeNum === focusRange && c.placeReason)
        ? '<div class="f1-standings-reason">' + esc(c.placeReason) + '</div>'
        : '';
      return '<li class="' + cls + '">' +
        '<span class="swatch" style="background:' + esc(c.color) + '"></span>' +
        '<span class="name">' + esc(name) + '</span>' +
        '<span class="shot">' + esc(shot) + '</span>' +
        reason +
        '</li>';
    }).join('') + '</ol>';
  }

  function hudHtml(race, blocked, focusRange) {
    const rem = race && race.roundRemainingSec != null ? Math.ceil(race.roundRemainingSec) : null;
    const pit = pitBannerHtml(race);
    const fieldHtml = fieldBannerHtml(race, focusRange != null ? focusRange : null);
    return '<div class="f1-hud f1-hud-compact">' +
      '<div class="f1-hud-row">' +
      '<span class="f1-badge">' + esc(phaseLabel(race && race.phase)) + '</span>' +
      '<span class="f1-circuit">' + esc((race && race.circuitId) || '') + '</span>' +
      '<span class="f1-round">R' + esc(race && race.currentRound) +
      (race && race.shotTotal ? '/' + esc(race.shotTotal) : '') +
      (race && race.currentSection
        ? (race.isPitRound ? ' · PIT' : ' · S' + esc(race.currentSection) + '/' + esc((race.powerSections || 9)))
        : (race && race.isPitRound ? ' · PIT' : '')) + '</span>' +
      (rem != null ? '<span class="f1-timer">' + rem + 's</span>' : '') +
      '</div>' +
      (fieldHtml || pit) +
      (blocked ? '<div class="f1-block">' + esc(blocked) + '</div>' : '') +
      '</div>';
  }

  function ensureTargetRegistry(assetsBase) {
    return new Promise(function (resolve) {
      if (window.SRTargetRegistry && window.SRTargetRegistry.ownerPluginId === 'f1-race') {
        resolve();
        return;
      }
      // assetsBase is /plugins/f1-race/assets → registry at /plugins/f1-race/target-registry.js
      const root = (assetsBase || '/plugins/f1-race/assets').replace(/\/assets\/?$/, '/');
      const s = document.createElement('script');
      s.src = root + 'target-registry.js?t=' + Date.now();
      s.onload = function () {
        if (window.SRTargetRegistry) window.SRTargetRegistry.ownerPluginId = 'f1-race';
        resolve();
      };
      s.onerror = function () {
        // target-core falls back to built-in geometry, which can misplace shots
        // for anything but the default profile.
        console.warn('f1-race: target-registry.js failed to load from ' + s.src);
        resolve();
      };
      document.head.appendChild(s);
    });
  }

  async function renderMaster(container, viewModel, assetsBase) {
    const race = (viewModel && viewModel.race) || {};
    const circuitId = race.circuitId || 'spa';
    container.className = 'range-plugin-view f1-race-view f1-race-master';

    const needsShell = !container.querySelector('.f1-master-layout');
    const circuitChanged = container.dataset.circuit !== circuitId;

    if (needsShell) {
      container.innerHTML =
        '<div class="f1-master-layout">' +
        '<div class="f1-master-track" data-track>' +
        '<div class="f1-track-host" data-host></div>' +
        '<div class="f1-track-overlay" data-overlay></div>' +
        '</div>' +
        '<aside class="f1-master-side">' +
        '<div data-hud></div>' +
        '<div class="f1-side-label">Fahrer</div>' +
        '<div data-standings></div>' +
        '</aside></div>';
    }

    const host = container.querySelector('[data-host]');
    const overlayEl = container.querySelector('[data-overlay]');
    const hudEl = container.querySelector('[data-hud]');
    const stEl = container.querySelector('[data-standings]');

    if (needsShell || circuitChanged) {
      container.dataset.circuit = circuitId;
      try {
        const svg = await loadCircuit(assetsBase, circuitId);
        renderTrackHost(host, svg, race, null, false);
      } catch (e) {
        host.innerHTML = '<div class="f1-error">Circuit konnte nicht geladen werden</div>';
      }
      container._f1Anim = false;
    }

    hudEl.innerHTML = hudHtml(race, race.startBlockedReason, null);
    stEl.innerHTML = standingsHtml(race, null);
    updatePitOverlay(overlayEl, race);

    container._f1LastVM = viewModel;
    const svgRoot = host && host.querySelector('svg');
    if (svgRoot) {
      placeCars(svgRoot, race.cars, true);
      if (!container._f1Anim) {
        container._f1Anim = true;
        (function tick() {
          if (!container.isConnected || container.hidden) {
            container._f1Anim = false;
            return;
          }
          const vm = container._f1LastVM || viewModel;
          const raceNow = (vm && vm.race) || {};
          const svg = container.querySelector('[data-host] svg');
          if (svg) placeCars(svg, raceNow.cars, true);
          updatePitOverlay(container.querySelector('[data-overlay]'), raceNow);
          syncPitCountdownAudio(raceNow);
          requestAnimationFrame(tick);
        })();
      }
    }
  }

  async function renderShooter(container, viewModel, assetsBase) {
    const core = window.SRCore;
    const race = (viewModel && viewModel.race) || {};
    const me = viewModel && viewModel.me;
    const rangeNum = viewModel && viewModel.rangeNum;
    let rangeData = null;
    if (core && core.lastLiveData) {
      rangeData = (core.lastLiveData.ranges || []).find(function (r) {
        return r.rangeNum === rangeNum;
      });
    }
    if (!rangeData && viewModel) rangeData = viewModel.range;

    container.className = 'range-plugin-view f1-race-view f1-race-shooter';
    await ensureTargetRegistry(assetsBase);
    if (core && core.setTargetAssetBase && assetsBase) core.setTargetAssetBase(assetsBase);

    // Build the shell once. Wiping it on every live message threw away the
    // target SVG and forced a full re-render per shot.
    if (!container.querySelector('.f1-shooter-layout')) {
      container.innerHTML =
        '<div class="f1-shooter-layout">' +
        '<header class="f1-shooter-header" data-header></header>' +
        '<div class="f1-shooter-main">' +
        '<div class="f1-shooter-target range-plugin-view classic-range-view" data-target></div>' +
        '<div class="f1-shooter-race">' +
        '<div data-hud></div>' +
        '<div data-hint></div>' +
        '<div class="f1-track-mini-wrap" data-track></div>' +
        '<div data-standings></div>' +
        '</div></div>' +
        '<footer class="f1-shooter-footer" data-footer></footer>' +
        '</div>';
    }

    const header = container.querySelector('[data-header]');
    const footer = container.querySelector('[data-footer]');
    const target = container.querySelector('[data-target]');
    const trackEl = container.querySelector('[data-track]');
    const hudEl = container.querySelector('[data-hud]');
    const hintEl = container.querySelector('[data-hint]');
    const stEl = container.querySelector('[data-standings]');

    const field = race.fieldEvent;
    const pitHtml = (!field || field.cleared || !field.pending) ? pitBannerHtml(race) : '';
    header.innerHTML =
      '<div class="f1-sh-top">' +
      '<strong>Bahn ' + esc(rangeNum) + '</strong>' +
      '<span>' + esc((rangeData && rangeData.shooterName) || (me && me.shooterName) || '') + '</span>' +
      '<span class="f1-badge">' + esc(phaseLabel(race.phase)) + '</span>' +
      '</div>' +
      (fieldBannerHtml(race, rangeNum) || pitHtml) +
      '<div class="f1-sh-meta">Runde ' + esc(race.currentRound) +
      (race.isPitRound ? ' · PIT' : '') +
      (me && me.totalShots ? ' · Schuss ' + esc(me.shotsFired) + '/' + esc(me.totalShots) : '') +
      (race.roundRemainingSec != null ? ' · ' + Math.ceil(race.roundRemainingSec) + 's' : '') +
      (me ? ' · Skip ' + esc(me.skippedConsecutive) + '/2' : '') +
      '</div>';

    if (rangeData && core && typeof core.renderClassicRangeView === 'function') {
      core.renderClassicRangeView(target, rangeData);
    } else {
      target.innerHTML = '<div class="classic-range-loading">Warte auf Scheibe…</div>';
    }

    hudEl.innerHTML = hudHtml(race, null, rangeNum);
    if (hintEl) {
      const hint = me && me.nextHint;
      const kind = (me && me.nextHintKind) || '';
      hintEl.innerHTML = hint
        ? '<div class="f1-next-hint kind-' + esc(kind) + '">' + esc(hint) + '</div>'
        : '';
    }
    stEl.innerHTML = standingsHtml(race, rangeNum);
    syncPitCountdownAudio(race);
    container._f1LastRace = race;
    if (!container._f1PitTick) {
      container._f1PitTick = true;
      (function pitTick() {
        if (!container.isConnected || container.hidden) {
          container._f1PitTick = false;
          return;
        }
        const r = container._f1LastRace;
        if (r) {
          syncPitCountdownAudio(r);
          const headerEl = container.querySelector('[data-header]');
          const existing = headerEl && headerEl.querySelector('.f1-pit-banner, .f1-event-banner');
          const fieldNow = r.fieldEvent;
          if (headerEl && (!fieldNow || fieldNow.cleared)) {
            const next = pitBannerHtml(r);
            if (existing && existing.classList.contains('f1-pit-banner')) {
              if (next) {
                const sec = pitCountdownSec(r);
                const label = sec > 0 ? ('PIT — ' + sec) : 'PIT — JETZT';
                if (existing.textContent !== label) existing.textContent = label;
              } else {
                existing.remove();
              }
            } else if (!existing && next) {
              const wrap = document.createElement('div');
              wrap.innerHTML = next;
              const shMeta = headerEl.querySelector('.f1-sh-meta');
              if (shMeta) headerEl.insertBefore(wrap.firstChild, shMeta);
              else headerEl.appendChild(wrap.firstChild);
            }
          }
        }
        requestAnimationFrame(pitTick);
      })();
    }
    try {
      const svg = await loadCircuit(assetsBase, race.circuitId || 'spa');
      renderTrackHost(trackEl, svg, race, rangeNum, true);
    } catch (e) {
      trackEl.innerHTML = '';
    }

    footer.innerHTML =
      '<div class="f1-footer-grid">' +
      '<div><span class="lbl">Position</span><span class="val">P' + esc(me && me.position) + '</span></div>' +
      '<div><span class="lbl">Tempo</span><span class="val">' + esc(me && me.lastSpeed != null ? me.lastSpeed.toFixed(1) : '—') + '</span></div>' +
      '<div><span class="lbl">Streak</span><span class="val">' + esc(me && me.highStreak) + '</span></div>' +
      '<div><span class="lbl">Status</span><span class="val">' + esc(me && me.status) + '</span></div>' +
      '</div>' +
      outcomeHtml(me);
  }

  window.SRPluginViews['f1-race'] = function render(container, viewModel, assetsBase) {
    if (!container) return;
    const isShooter = document.body.classList.contains('shooter-display') ||
      (window.SRDisplay && window.SRDisplay.display === 'shooter');
    const focusRange = isShooter ? Number(viewModel && viewModel.rangeNum) : 0;
    const events = (viewModel && viewModel.events) || [];
    const eventSig = JSON.stringify(events);
    if (events.length && eventSig !== lastEventSig) {
      lastEventSig = eventSig;
      playEventSounds(events, focusRange || null);
    }
    // Shared pit cue: also fire when pitCueAt appears even if events were drained.
    const pitAt = viewModel && viewModel.race && viewModel.race.pitCueAt;
    if (pitAt && pitAt !== lastPitCueAt) {
      lastPitCueAt = pitAt;
      if (!events.some(function (e) { return e && e.type === 'pit_cue'; })) {
        playEventSounds([{ type: 'pit_cue' }], null);
      }
      lastCountdownSec = null;
    } else if (!pitAt) {
      lastPitCueAt = '';
    }

    return isShooter
      ? renderShooter(container, viewModel, assetsBase)
      : renderMaster(container, viewModel, assetsBase);
  };

  window.SRPlugins.render = function (id, container, viewModel, assetsBase) {
    const fn = window.SRPluginViews[id];
    if (typeof fn === 'function') fn(container, viewModel, assetsBase);
  };
})();
