"""UI regression: complete game of each plugin at 1920x1080.

The check that was ordered:
  screenshot every hall + compact + shooter, fail leftover-space scrollbars.

    python scripts/ui_regression.py

Requires: pip install playwright && python -m playwright install msedge
Server:   ./srdashboard.exe  (http://127.0.0.1:8080, UDP 30169)
"""
from __future__ import annotations

import json
import math
import os
import socket
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from playwright.sync_api import sync_playwright

BASE = os.environ.get("SRDASH_URL", "http://127.0.0.1:8080")
UDP = ("127.0.0.1", int(os.environ.get("SRDASH_UDP", "30169")))
OUT = Path(os.environ.get("SRDASH_UI_OUT", Path(__file__).resolve().parent.parent / "testdata" / "ui-regression"))
VW, VH = 1920, 1080
RANGES = 6
# UDP is datagram: burst faster than the listener's apply/notify loop and packets vanish.
SHOT_GAP = float(os.environ.get("SRDASH_SHOT_GAP", "0.08"))
PLAYS = max(1, int(os.environ.get("SRDASH_PLAYS", "4")))

OVERFLOW_JS = """() => {
  const skip = new Set(['HTML', 'BODY']);
  const hits = [];
  const walk = (el) => {
    if (!el || el.nodeType !== 1) return;
    if (skip.has(el.tagName)) { [...el.children].forEach(walk); return; }
    const s = getComputedStyle(el);
    const yScroll = (s.overflowY === 'auto' || s.overflowY === 'scroll') && el.scrollHeight > el.clientHeight + 2;
    const xScroll = (s.overflowX === 'auto' || s.overflowX === 'scroll') && el.scrollWidth > el.clientWidth + 2;
    if (yScroll || xScroll) {
      const r = el.getBoundingClientRect();
      let leftover = 0;
      let p = el.parentElement;
      while (p && leftover < 24) {
        const pr = p.getBoundingClientRect();
        leftover = Math.max(leftover, pr.bottom - r.bottom, pr.right - r.right);
        p = p.parentElement;
        if (p && (p.id === 'stage' || p.id === 'shooter-plugin-view' || p.classList.contains('shooter-shell'))) break;
      }
      hits.push({
        id: el.id || '',
        cls: String(el.className || '').slice(0, 120),
        yScroll, xScroll,
        leftover: Math.round(leftover)
      });
    }
    [...el.children].forEach(walk);
  };
  walk(document.body);
  const pageY = document.documentElement.scrollHeight > window.innerHeight + 2;
  const pageX = document.documentElement.scrollWidth > window.innerWidth + 2;
  const shared = document.querySelector('#shared-master-host:not([hidden])');
  const shooter = document.querySelector('#shooter-plugin-view');
  const panel = document.querySelector('.range-panel:not([hidden])');
  const host = shared || shooter || (panel && (panel.querySelector('.range-plugin-view') || panel));
  const text = host ? (host.innerText || '').replace(/\\s+/g, ' ').trim() : '';
  const hasSvg = !!(host && host.querySelector('svg'));
  return {
    hits, pageY, pageX,
    painted: !!(host && host.children.length && (text.length > 10 || hasSvg)),
    textLen: text.length
  };
}"""

# Catch the class of bugs leftover-space scrollbars miss: clipped button
# labels (Regeln in the 2.85rem rail), overlapping chrome, plugin paint
# failures, duplicate view.js, standings cells sitting on each other
# (Fox "0.0" next to "gestellt"), and footer legend contrast (Ludo
# "10.0 setzt ein" on glass over the wood scene).
READABILITY_JS = """() => {
  const vw = innerWidth, vh = innerHeight;
  const vis = (el) => {
    if (!el || el.nodeType !== 1 || el.hidden) return false;
    const s = getComputedStyle(el);
    if (s.display === 'none' || s.visibility === 'hidden' || Number(s.opacity) === 0) return false;
    const r = el.getBoundingClientRect();
    return r.width >= 1 && r.height >= 1 && r.bottom > 0 && r.right > 0 && r.top < vh && r.left < vw;
  };
  const wrapHits = [];
  document.querySelectorAll('ul').forEach((ul) => {
    const pcls = String(ul.className || '');
    if (!/standings|fahrer|recent/i.test(pcls)) return;
    [...ul.querySelectorAll('span, strong')].forEach((el) => {
      if (!vis(el)) return;
      const s = getComputedStyle(el);
      if (s.textOverflow === 'ellipsis') return;
      const text = (el.innerText || '').replace(/\\s+/g, ' ').trim();
      if (text.length < 8) return;
      if (el.getClientRects().length > 1) {
        wrapHits.push({
          cls: String(ul.className || '').slice(0, 80),
          text: text.slice(0, 48),
          lines: el.getClientRects().length
        });
      }
    });
  });
  const clipHits = [];
  document.querySelectorAll('button, [role="button"], a.btn, .status-chip').forEach((el) => {
    if (!vis(el)) return;
    const s = getComputedStyle(el);
    if (s.textOverflow === 'ellipsis') return;
    const text = (el.innerText || '').replace(/\\s+/g, ' ').trim();
    if (text.length < 2) return;
    if (el.scrollWidth > el.clientWidth + 1) {
      clipHits.push({
        id: el.id || '',
        cls: String(el.className || '').slice(0, 80),
        text: text.slice(0, 48),
        sw: el.scrollWidth,
        cw: el.clientWidth
      });
    }
  });
  const rail = document.getElementById('top-bar');
  const menuOpen = !!(rail && rail.classList.contains('is-menu-open'));
  const leak = (sel) => {
    const el = document.querySelector(sel);
    if (!el || menuOpen) return false;
    const s = getComputedStyle(el);
    if (s.display === 'none') return false;
    const text = (el.innerText || '').replace(/\\s+/g, ' ').trim();
    return text.length > 0 && vis(el);
  };
  const slimLabelLeak = leak('.rulebook-btn-label') || leak('.menu-toggle-label');
  const pluginErrors = [...document.querySelectorAll('.plugin-error')].filter(vis).map((el) =>
    (el.innerText || '').replace(/\\s+/g, ' ').trim().slice(0, 80)
  );
  const chromeOverlaps = [];
  const slim = document.querySelector('.side-rail-slim');
  if (slim) {
    const btns = [...slim.querySelectorAll('button')].filter(vis);
    for (let i = 0; i < btns.length; i++) {
      const a = btns[i].getBoundingClientRect();
      for (let j = i + 1; j < btns.length; j++) {
        const b = btns[j].getBoundingClientRect();
        const ox = Math.min(a.right, b.right) - Math.max(a.left, b.left);
        const oy = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (ox > 1 && oy > 1) {
          chromeOverlaps.push({ a: btns[i].id || btns[i].className, b: btns[j].id || btns[j].className });
        }
      }
    }
  }
  const viewCounts = {};
  [...document.scripts].forEach((s) => {
    if (!s.src || s.src.indexOf('view.js') < 0) return;
    const k = s.src.split('?')[0];
    viewCounts[k] = (viewCounts[k] || 0) + 1;
  });
  const dupViews = Object.entries(viewCounts).filter(([, n]) => n > 1).map(([src, n]) => ({ src, n }));
  const cellOverlaps = [];
  document.querySelectorAll('ul').forEach((ul) => {
    const pcls = String(ul.className || '');
    if (!/standings|fahrer|recent/i.test(pcls)) return;
    [...ul.children].forEach((li) => {
      const spans = [...li.children].filter(vis);
      for (let i = 0; i < spans.length - 1; i++) {
        const a = spans[i].getBoundingClientRect();
        const b = spans[i + 1].getBoundingClientRect();
        const ox = Math.min(a.right, b.right) - Math.max(a.left, b.left);
        const oy = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (ox > 2 && oy > 2) {
          cellOverlaps.push({
            row: (li.innerText || '').replace(/\\s+/g, ' ').trim().slice(0, 48),
            ox: Math.round(ox),
            oy: Math.round(oy)
          });
        }
      }
    });
  });
  const parseRGBA = (str) => {
    if (!str || str === 'transparent') return { r: 0, g: 0, b: 0, a: 0 };
    const open = str.indexOf('(');
    const close = str.lastIndexOf(')');
    if (open < 0 || close < 0) return null;
    const parts = str.slice(open + 1, close).replace('/', ' ').split(/[\\s,]+/).filter(Boolean);
    if (parts.length < 3) return null;
    const ch = (v) => String(v).endsWith('%') ? parseFloat(v) * 2.55 : parseFloat(v);
    const a = parts[3] == null ? 1 : (String(parts[3]).endsWith('%') ? parseFloat(parts[3]) / 100 : parseFloat(parts[3]));
    if (![ch(parts[0]), ch(parts[1]), ch(parts[2])].every(isFinite)) return null;
    return { r: ch(parts[0]), g: ch(parts[1]), b: ch(parts[2]), a: isFinite(a) ? a : 1 };
  };
  const lin = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
  const lum = (c) => 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b);
  const srcOver = (src, dest) => {
    const a = src.a + dest.a * (1 - src.a);
    if (a < 1e-6) return { r: 0, g: 0, b: 0, a: 0 };
    const w = (s, d) => (s * src.a + d * dest.a * (1 - src.a)) / a;
    return { r: w(src.r, dest.r), g: w(src.g, dest.g), b: w(src.b, dest.b), a };
  };
  const effectiveBg = (el) => {
    let acc = { r: 0, g: 0, b: 0, a: 0 };
    let n = el;
    while (n && n.nodeType === 1 && acc.a < 0.99) {
      const c = parseRGBA(getComputedStyle(n).backgroundColor);
      if (c && c.a > 0.01) acc = acc.a < 0.01 ? c : srcOver(acc, c);
      n = n.parentElement;
    }
    if (acc.a < 0.99) acc = srcOver(acc, { r: 255, g: 255, b: 255, a: 1 });
    return acc;
  };
  const contrastHits = [];
  const hosts = [
    document.querySelector('#shared-master-host:not([hidden])'),
    document.querySelector('#shooter-plugin-view'),
    document.querySelector('.range-plugin-view')
  ].filter(Boolean);
  const footers = [];
  hosts.forEach((h) => {
    h.querySelectorAll('footer, [data-footer], .range-footer').forEach((el) => {
      if (footers.indexOf(el) < 0) footers.push(el);
    });
  });
  footers.forEach((root) => {
    if (!vis(root)) return;
    const nodes = [root, ...root.querySelectorAll('*')];
    nodes.forEach((el) => {
      if (!vis(el) || el.closest('svg')) return;
      const own = [...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.textContent || '').join('').replace(/\\s+/g, ' ').trim();
      if (own.length < 2) return;
      const fgRaw = parseRGBA(getComputedStyle(el).color);
      if (!fgRaw) return;
      const bg = effectiveBg(el);
      const fg = srcOver({ r: fgRaw.r, g: fgRaw.g, b: fgRaw.b, a: fgRaw.a }, { r: bg.r, g: bg.g, b: bg.b, a: 1 });
      const hi = Math.max(lum(fg), lum(bg));
      const lo = Math.min(lum(fg), lum(bg));
      const ratio = (hi + 0.05) / (lo + 0.05);
      const fs = parseFloat(getComputedStyle(el).fontSize) || 0;
      const fw = getComputedStyle(el).fontWeight;
      const large = fs >= 18.5 || (fs >= 14 && (fw === 'bold' || parseInt(fw, 10) >= 700));
      const need = large ? 3 : 4.5;
      if (ratio + 1e-6 < need) {
        contrastHits.push({
          cls: String(el.className || root.className || '').slice(0, 80),
          text: own.slice(0, 48),
          ratio: Math.round(ratio * 100) / 100,
          need
        });
      }
    });
  });
  const failed = performance.getEntriesByType('resource')
    .filter((e) => e.responseStatus >= 400 && /\\.(js|css|svg|png|webp|woff2?)(\\?|$)/i.test(e.name))
    .map((e) => ({ name: e.name, status: e.responseStatus }));
  return { clipHits, wrapHits, slimLabelLeak, pluginErrors, chromeOverlaps, dupViews, cellOverlaps, contrastHits, failed };
}"""

FOX_DISC_JS = """() => {
  const svg = document.querySelector('.fox-scheibe-frame svg, .fox-scheibe-wrap svg');
  if (!svg) return { missing: true };
  const circle = svg.querySelector('circle');
  if (!circle) return { missing: true };
  const r = circle.getBoundingClientRect();
  const wrap = document.querySelector('.fox-scheibe-wrap');
  const ws = wrap ? getComputedStyle(wrap) : null;
  const body = document.body.innerText || '';
  const stretch = Math.abs(r.width - r.height) / Math.max(r.width, r.height, 1);
  return {
    missing: false,
    par: svg.getAttribute('preserveAspectRatio') || '',
    circleW: r.width, circleH: r.height, stretch,
    wrapBg: ws ? ws.backgroundColor : '',
    hasS0: body.indexOf('Fuchs S0') >= 0 || body.indexOf('dran S0') >= 0,
    hasVorsprung: body.indexOf('Vorsprung') >= 0
  };
}"""


def api(method, path, body=None, timeout=15):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        BASE + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"} if body is not None else {},
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read()
        return json.loads(raw.decode()) if raw else None


def activate(plugin_id):
    api("POST", "/api/plugins/activate", {"id": plugin_id})


def control(action, params=None):
    api("POST", "/api/plugins/control", {"action": action, "params": params or {}})


def live_field(warmup=False, shots=10):
    return {
        str(r): {
            "shooterName": "Test B%d" % r,
            "discipline": "LG 40 Schuss",
            "isWarmup": warmup,
            "totalShotsToFire": shots,
        }
        for r in range(1, RANGES + 1)
    }


def reset_live():
    for r in range(1, RANGES + 1):
        try:
            api("POST", "/api/live/reset?range=%d" % r, {})
        except Exception:
            pass


def shot_xy(dec, n=0, range_num=1, band=25.0):
    """Place the pellet on the ISSF teiler band for this DecValue.

    LG is 25 DSG per 0.1 ring (100 DSG/mm). LP/KK use 80 DSG per 0.1 ring.
    X/Y/Distance must match DecValue or Classic Range draws a 10.3 in the 7-ring.
    """
    if band is None or band <= 0:
        band = 25.0
    dec = max(0.0, min(10.9, float(dec)))
    step = int(round((10.9 - dec) * 10))
    lo = step * band
    hi = lo + band
    t = ((n * 17 + range_num * 13) % 1000) / 1000.0
    dist = (t * band * 0.4) if dec >= 10.9 else lo + t * (hi - lo)
    ang = ((n * 47 + range_num * 31) % 360) * math.pi / 180.0
    x = int(round(math.cos(ang) * dist))
    y = int(round(math.sin(ang) * dist))
    return x, y, round(math.hypot(x, y), 1)


def live_shot_number(range_num):
    try:
        data = api("GET", "/api/live?range=%d" % range_num)
    except Exception:
        return -1
    rows = data.get("ranges") if isinstance(data, dict) else data
    if not rows:
        return -1
    return int(rows[0].get("shotNumber") or 0)


def send_raw_shot(obj):
    rng = int(obj.get("Range") or 1)
    msg = {"MessageType": "Event", "MessageVerb": "Shot", "Ranges": rng, "Objects": [obj]}
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.sendto(json.dumps(msg).encode(), UDP)
    sock.close()
    time.sleep(SHOT_GAP)


def probe_broken_shots():
    """Fire packets that must be dropped by udp.ValidateShot. Prints the outcome."""
    print("\nUDP VALIDATION (broken packets — must not enter live state):")
    broken = [
        {
            "why": "10.3 in the 7-ring (X/Y vs DecValue)",
            "X": 800, "Y": -200, "Distance": 0.2, "FullValue": 10, "DecValue": 10.3,
        },
        {
            "why": "Distance 0.2 but hypot(X,Y)=160",
            "X": 0, "Y": 160, "Distance": 0.2, "FullValue": 10, "DecValue": 10.3,
        },
        {
            "why": "FullValue 10 with DecValue 8.2",
            "X": 500, "Y": 200, "Distance": 538.5, "FullValue": 10, "DecValue": 8.2,
        },
        {
            "why": "0.0 miss stacked on the bullseye",
            "X": 0, "Y": 0, "Distance": 0, "FullValue": 0, "DecValue": 0.0,
        },
        {
            "why": "random coords nowhere near the 10.8 teiler band",
            "X": 4120, "Y": -3300, "Distance": 5278.1, "FullValue": 10, "DecValue": 10.8,
        },
    ]
    failed = 0
    for i, spec in enumerate(broken, 1):
        why = spec.pop("why")
        spec.update({
            "Range": 1, "IsWarmup": False, "DiscType": "LG",
            "Shooter": {"Firstname": "Broken", "Lastname": "Probe"},
        })
        before = live_shot_number(1)
        send_raw_shot(spec)
        after = live_shot_number(1)
        if after > before:
            failed += 1
            print("  FAIL  #%d leaked into live (shotNumber %d→%d): %s  X=%s Y=%s D=%s Dec=%.1f Full=%s" % (
                i, before, after, why, spec["X"], spec["Y"], spec["Distance"], spec["DecValue"], spec["FullValue"]))
        else:
            print("  DROP  #%d shotNumber stayed %d — %s" % (i, before, why))
            print("        X=%s Y=%s Distance=%s FullValue=%s DecValue=%s" % (
                spec["X"], spec["Y"], spec["Distance"], spec["FullValue"], spec["DecValue"]))
    if failed:
        print("UDP VALIDATION: %d broken packet(s) were applied — rebuild srdashboard.exe" % failed)
        return False
    print("UDP VALIDATION: all %d broken packets dropped\n" % len(broken))
    return True


def send_shot(range_num, dec, warmup=False, n=0, shooter=None):
    x, y, distance = shot_xy(dec, n, range_num)
    before = live_shot_number(range_num)
    if shooter is None:
        shooter = ("Test", "B%d" % range_num)
    first, last = shooter
    obj = {
        "X": x, "Y": y, "Distance": distance,
        "FullValue": min(10, max(0, int(dec))), "DecValue": dec,
        "Range": range_num, "IsWarmup": warmup,
        "DiscType": "LG",
        "Shooter": {"Firstname": first, "Lastname": last},
        "MenuItem": {"MenuItemName": "Luftgewehr", "MenuPointName": "Luftgewehr"},
    }
    msg = {"MessageType": "Event", "MessageVerb": "Shot", "Ranges": range_num, "Objects": [obj]}
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.sendto(json.dumps(msg).encode(), UDP)
    sock.close()
    deadline = time.time() + 2.0
    while time.time() < deadline:
        if live_shot_number(range_num) > before:
            break
        time.sleep(0.03)
    time.sleep(SHOT_GAP)


def session():
    return api("GET", "/api/plugins/session") or {}


def phase_of(sess):
    snaps = sess.get("sessions") or []
    if not snaps:
        return "", {}
    vm = snaps[0].get("viewModel") or {}
    for key in ("race", "game", "hunt", "match", "tree"):
        block = vm.get(key)
        if isinstance(block, dict) and block.get("phase"):
            return str(block.get("phase")), vm
    return str(vm.get("phase") or snaps[0].get("phase") or ""), vm


class Runner:
    def __init__(self, hall, shooter, compact):
        self.hall = hall
        self.shooter = shooter
        self.compact = compact
        self.problems = []
        self.play_index = 0
        self.js_errors = []
        self._attach_js_errors("hall", hall)
        self._attach_js_errors("shooter", shooter)
        self._attach_js_errors("compact", compact)

    def _attach_js_errors(self, kind, page):
        def on_pageerror(err):
            self.js_errors.append("%s pageerror: %s" % (kind, err))

        def on_console(msg):
            if msg.type != "error":
                return
            text = msg.text or ""
            low = text.lower()
            if "favicon" in low or "resizeobserver" in low:
                return
            self.js_errors.append("%s console.error: %s" % (kind, text[:200]))

        page.on("pageerror", on_pageerror)
        page.on("console", on_console)

    def wait_paint(self, page):
        page.wait_for_function(
            """() => {
              const shared = document.querySelector('#shared-master-host:not([hidden])');
              const shooter = document.querySelector('#shooter-plugin-view');
              const panel = document.querySelector('.range-panel:not([hidden])');
              const host = shared || shooter || (panel && (panel.querySelector('.range-plugin-view') || panel));
              if (!host) return false;
              const text = (host.innerText || '').replace(/\\s+/g, ' ').trim();
              const hasSvg = !!host.querySelector('svg');
              return host.children.length > 0 && (text.length > 10 || hasSvg);
            }""",
            timeout=15000,
        )

    def reload(self):
        self.hall.goto(BASE + "/", wait_until="domcontentloaded", timeout=30000)
        self.shooter.goto(BASE + "/1/", wait_until="domcontentloaded", timeout=30000)
        self.compact.goto(BASE + "/compact", wait_until="domcontentloaded", timeout=30000)
        self.wait_paint(self.hall)
        self.wait_paint(self.shooter)
        self.wait_paint(self.compact)

    def set_menu(self, open_menu):
        self.hall.evaluate(
            """(open) => {
              const btn = document.getElementById('menu-toggle');
              if (!btn) return;
              const expanded = btn.getAttribute('aria-expanded') === 'true';
              if (expanded !== !!open) btn.click();
            }""",
            open_menu,
        )
        self.hall.wait_for_timeout(250)

    def capture(self, plugin_id, label, menu=False):
        if self.play_index != 0:
            return
        time.sleep(0.2)
        kinds = [("hall", self.hall), ("compact", self.compact), ("shooter", self.shooter)]
        if menu:
            self.set_menu(True)
            kinds = [("hall-menu", self.hall)] + kinds
        try:
            for kind, page in kinds:
                if kind != "hall-menu":
                    if kind == "hall":
                        self.set_menu(False)
                    self.wait_paint(page)
                shot_path = OUT / ("%s-%s-%s.png" % (plugin_id, label, kind))
                page.screenshot(path=str(shot_path), full_page=False)
                info = page.evaluate(OVERFLOW_JS)
                read = page.evaluate(READABILITY_JS)
                wasted = [h for h in info["hits"] if h["leftover"] >= 40]
                print("%s %s %s painted=%s wasted=%d clips=%d wraps=%d contrast=%d pageY=%s" % (
                    plugin_id, label, kind, info["painted"], len(wasted),
                    len(read.get("clipHits") or []), len(read.get("wrapHits") or []),
                    len(read.get("contrastHits") or []),
                    info["pageY"]))
                if not info["painted"]:
                    self.problems.append("%s %s %s: plugin did not paint (%s)" % (plugin_id, label, kind, shot_path))
                if info["pageY"] or info["pageX"]:
                    self.problems.append("%s %s %s: page scrollbar at %dx%d (%s)" % (
                        plugin_id, label, kind, VW, VH, shot_path))
                if wasted:
                    self.problems.append("%s %s %s: leftover-space scrollbar %s (%s)" % (
                        plugin_id, label, kind, wasted, shot_path))
                if read.get("clipHits"):
                    self.problems.append("%s %s %s: button text overflow %s (%s)" % (
                        plugin_id, label, kind, read["clipHits"], shot_path))
                if read.get("wrapHits"):
                    self.problems.append("%s %s %s: standings text wrapped %s (%s)" % (
                        plugin_id, label, kind, read["wrapHits"], shot_path))
                if read.get("slimLabelLeak"):
                    self.problems.append("%s %s %s: Regeln/Menü label visible in collapsed 2.85rem rail (%s)" % (
                        plugin_id, label, kind, shot_path))
                if read.get("pluginErrors"):
                    self.problems.append("%s %s %s: plugin-error %s (%s)" % (
                        plugin_id, label, kind, read["pluginErrors"], shot_path))
                if read.get("chromeOverlaps"):
                    self.problems.append("%s %s %s: slim-rail buttons overlap %s (%s)" % (
                        plugin_id, label, kind, read["chromeOverlaps"], shot_path))
                if read.get("dupViews"):
                    self.problems.append("%s %s %s: view.js loaded more than once %s (%s)" % (
                        plugin_id, label, kind, read["dupViews"], shot_path))
                if read.get("cellOverlaps"):
                    self.problems.append("%s %s %s: standings cells overlap %s (%s)" % (
                        plugin_id, label, kind, read["cellOverlaps"], shot_path))
                if read.get("contrastHits"):
                    self.problems.append("%s %s %s: footer contrast %s (%s)" % (
                        plugin_id, label, kind, read["contrastHits"], shot_path))
                if read.get("failed"):
                    self.problems.append("%s %s %s: failed assets %s (%s)" % (
                        plugin_id, label, kind, read["failed"], shot_path))
                if plugin_id == "fox-on-the-run":
                    disc = page.evaluate(FOX_DISC_JS)
                    if kind == "compact":
                        if not disc.get("missing"):
                            self.problems.append(
                                "fox %s %s: compact hall must not paint a scheibe (%s)" % (
                                    label, kind, shot_path))
                    elif disc.get("missing"):
                        self.problems.append("fox %s %s: no scheibe SVG (%s)" % (label, kind, shot_path))
                    else:
                        if disc["stretch"] > 0.04:
                            self.problems.append(
                                "fox %s %s: disc stretched %.3f %.0fx%.0f (%s)" % (
                                    label, kind, disc["stretch"], disc["circleW"], disc["circleH"], shot_path))
                        if label in ("calibrate", "start") and disc["hasS0"]:
                            self.problems.append("fox %s %s: HUD shows Fuchs S0 / dran S0 (%s)" % (
                                label, kind, shot_path))
                        if label in ("calibrate", "start") and disc["hasVorsprung"]:
                            self.problems.append("fox %s %s: chase Vorsprung HUD during calibration (%s)" % (
                                label, kind, shot_path))
            errs = self.js_errors[:]
            self.js_errors.clear()
            for err in errs:
                self.problems.append("%s %s: JS %s" % (plugin_id, label, err))
        finally:
            if menu:
                self.set_menu(False)

    def play_classic(self, plugin_id="classic-range"):
        activate(plugin_id)
        reset_live()
        self.reload()
        self.capture(plugin_id, "start", menu=True)
        for i in range(12):
            send_shot(1, 10.4 - (i % 5) * 0.3, n=i)
        self.reload()
        self.capture(plugin_id, "end")

    def play_autorennen(self):
        activate("autorennen")
        control("reset")
        control("sync_live", {"live": live_field(warmup=True, shots=10)})
        self.reload()
        self.capture("autorennen", "warmup", menu=True)
        for r in range(1, RANGES + 1):
            send_shot(r, 8.5, warmup=True, n=1)
        control("start", {"live": live_field(warmup=False, shots=10)})
        time.sleep(0.4)
        self.reload()
        self.capture("autorennen", "racing")
        for n in range(10):
            for r in range(1, RANGES + 1):
                send_shot(r, 10.3, n=n + 10)
        self.reload()
        self.capture("autorennen", "end")

    def play_started(self, plugin_id, shots_fn):
        activate(plugin_id)
        control("reset")
        try:
            control("start", {"live": live_field()})
        except Exception as exc:
            print(plugin_id, "start:", exc)
        self.reload()
        self.capture(plugin_id, "start", menu=True)
        shots_fn()
        self.reload()
        self.capture(plugin_id, "end")

    def play_ludo(self):
        names = [
            ("Alex", "Alpha"),
            ("Blake", "Bravo"),
            ("Casey", "Charlie"),
            ("Dana", "Delta"),
            ("Eden", "Echo"),
            ("Finn", "Foxtrot"),
        ]
        def shots():
            for r in range(1, 7):
                send_shot(r, 10.0, n=1, shooter=names[r - 1])
            for r in range(1, 7):
                send_shot(r, 10.5, n=2, shooter=names[r - 1])
            for i in range(6):
                for r in range(1, 7):
                    send_shot(r, 10.5, n=10 + i, shooter=names[r - 1])
        self.play_started("ludo", shots)

    def play_barrikade(self):
        def shots():
            for i in range(40):
                send_shot(1, 10.0 if i % 4 == 3 else 10.5, n=i)
                ph, _ = phase_of(session())
                if "finish" in ph.lower():
                    break
        self.play_started("barrikade", shots)

    def play_bingo(self):
        def shots():
            _, vm = phase_of(session())
            values = ((vm or {}).get("game") or {}).get("cardValues") or []
            for i, v in enumerate(values[:5]):
                send_shot(1, float(v), n=i)
        self.play_started("zehner-bingo", shots)

    def play_tannebaum(self, plugin_id):
        seq = [5, 6, 7, 8.0, 8.5, 9.0, 9.5, 10.0, 8.1, 9.1, 10.1, 10.5, 10.6, 10.7, 10.8, 10.9]

        def shots():
            for i, v in enumerate(seq):
                send_shot(1, v, n=i)
        self.play_started(plugin_id, shots)

    def play_fox(self):
        activate("fox-on-the-run")
        control("reset")
        control("sync_live", {"live": live_field()})
        self.reload()
        self.capture("fox-on-the-run", "calibrate", menu=True)
        for r in range(1, RANGES + 1):
            for i in range(2):
                send_shot(r, 8.0, n=i + r * 5)
        try:
            control("start", {"live": live_field()})
        except Exception as exc:
            print("fox start:", exc)
        time.sleep(0.4)
        self.reload()
        self.capture("fox-on-the-run", "opening")
        for i in range(80):
            ph, vm = phase_of(session())
            hunt = vm.get("hunt") or {}
            turn = hunt.get("turnRange")
            fox = hunt.get("currentFox")
            phase = (hunt.get("phase") or ph or "").lower()
            if "finish" in phase:
                break
            if not turn:
                try:
                    control("start", {"live": live_field()})
                except Exception:
                    pass
                time.sleep(0.05)
                continue
            dec = 1.2 if turn == fox else 10.8
            send_shot(int(turn), dec, n=100 + i)
        self.reload()
        self.capture("fox-on-the-run", "end")

    def play_concept(self, plugin_id):
        def shots():
            for i in range(80):
                for r in range(1, min(3, RANGES + 1)):
                    send_shot(r, 10.9 if r == 1 else 8.8, n=200 + i * 10 + r)
                ph, _ = phase_of(session())
                if "finish" in ph.lower():
                    return
        self.play_started(plugin_id, shots)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    try:
        urllib.request.urlopen(BASE + "/api/plugins", timeout=3)
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        print("dashboard not running at", BASE, "—", exc, file=sys.stderr)
        print("start ./srdashboard.exe then re-run python scripts/ui_regression.py", file=sys.stderr)
        return 2

    if not probe_broken_shots():
        return 1

    with sync_playwright() as p:
        try:
            browser = p.chromium.launch(channel="msedge", headless=True, args=["--disable-gpu"])
        except Exception:
            browser = p.chromium.launch(headless=True)
        hall = browser.new_page(viewport={"width": VW, "height": VH})
        shooter = browser.new_page(viewport={"width": VW, "height": VH})
        compact = browser.new_page(viewport={"width": VW, "height": VH})
        r = Runner(hall, shooter, compact)
        try:
            for play in range(PLAYS):
                r.play_index = play
                print("=== play %d/%d ===" % (play + 1, PLAYS))
                r.play_classic("classic-range")
                r.play_classic("classic-range-condensed")
                r.play_ludo()
                r.play_barrikade()
                r.play_bingo()
                r.play_tannebaum("tannebaum-einzel")
                r.play_tannebaum("tannebaum-team")
                r.play_fox()
                r.play_autorennen()
                for pid in (
                    "tauziehen", "kettenreaktion", "biathlon", "schrumpfender-kreis",
                    "kronen-duell", "bank-oder-risiko", "ko-pokal", "schiessgolf",
                    "turmbau", "ansage-duell",
                ):
                    r.play_concept(pid)
        finally:
            try:
                activate("classic-range")
                reset_live()
            except Exception:
                pass
            browser.close()

    if r.problems:
        print("\nUI REGRESSION FAILURES:")
        for line in r.problems:
            print("-", line)
        return 1
    print("\nOK: %d complete game(s) of each plugin at %dx%d, no leftover-space scrollbars" % (PLAYS, VW, VH))
    return 0


if __name__ == "__main__":
    sys.exit(main())
