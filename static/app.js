(function () {
  function resolveDisplay() {
    const params = new URLSearchParams(location.search);
    const qDisplay = params.get('display');
    const qRange = params.get('range');
    if (qDisplay === 'shooter' || qDisplay === 'master' || qDisplay === 'config') {
      return {
        display: qDisplay,
        rangeNum: qDisplay === 'shooter' ? (parseInt(qRange, 10) || 1) : null
      };
    }
    if (/^\/config\/?$/.test(location.pathname)) {
      return { display: 'config', rangeNum: null };
    }
    const m = location.pathname.match(/^\/(\d+)\/?$/);
    if (m) {
      return { display: 'shooter', rangeNum: parseInt(m[1], 10) };
    }
    return { display: 'master', rangeNum: null };
  }

  const resolved = resolveDisplay();
  const display = resolved.display;
  window.SRDisplay = resolved;

  const THEME_KEY = 'srdashboard-theme';

  function getTheme() {
    const attr = document.documentElement.getAttribute('data-theme');
    if (attr === 'dark' || attr === 'light') return attr;
    try {
      const stored = localStorage.getItem(THEME_KEY);
      if (stored === 'dark' || stored === 'light') return stored;
    } catch (e) { /* ignore */ }
    return 'light';
  }

  function applyTheme(theme) {
    const next = theme === 'dark' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', next);
    try { localStorage.setItem(THEME_KEY, next); } catch (e) { /* ignore */ }
    document.dispatchEvent(new CustomEvent('srdashboard:themechange', { detail: { theme: next } }));
    return next;
  }

  function toggleTheme() {
    return applyTheme(getTheme() === 'dark' ? 'light' : 'dark');
  }

  window.SRTheme = {
    get: getTheme,
    set: applyTheme,
    toggle: toggleTheme
  };

  const SHOT_SAT_KEY = 'srdashboard-shot-sat';
  const DEFAULT_SHOT_SAT = 48;

  function clampShotSat(n) {
    n = Math.round(Number(n));
    if (!Number.isFinite(n)) return DEFAULT_SHOT_SAT;
    return Math.max(0, Math.min(100, n));
  }

  function getShotSat() {
    const inline = document.documentElement.style.getPropertyValue('--shot-sat').trim();
    if (inline !== '') {
      const n = Number(inline);
      if (Number.isFinite(n)) return clampShotSat(n);
    }
    try {
      const stored = localStorage.getItem(SHOT_SAT_KEY);
      if (stored != null && stored !== '') return clampShotSat(stored);
    } catch (e) { /* ignore */ }
    return DEFAULT_SHOT_SAT;
  }

  function applyShotSat(sat, opts) {
    const next = clampShotSat(sat);
    document.documentElement.style.setProperty('--shot-sat', String(next));
    try { localStorage.setItem(SHOT_SAT_KEY, String(next)); } catch (e) { /* ignore */ }
    if (!opts || opts.emit !== false) {
      document.dispatchEvent(new CustomEvent('srdashboard:shotsatchange', { detail: { sat: next } }));
    }
    return next;
  }

  window.SRShotSat = {
    get: getShotSat,
    set: applyShotSat,
    default: DEFAULT_SHOT_SAT
  };

  document.addEventListener('DOMContentLoaded', function () {
    const chrome = document.getElementById('master-chrome');
    const shooter = document.getElementById('shooter-app');
    const config = document.getElementById('config-app');
    if (display === 'shooter') {
      if (chrome) chrome.hidden = true;
      if (config) config.hidden = true;
      if (shooter) shooter.hidden = false;
    } else if (display === 'config') {
      if (chrome) chrome.hidden = true;
      if (shooter) shooter.hidden = true;
      if (config) config.hidden = false;
      document.body.classList.add('config-display');
    } else {
      if (shooter) shooter.hidden = true;
      if (config) config.hidden = true;
      if (chrome) chrome.hidden = false;
    }
  });
})();
