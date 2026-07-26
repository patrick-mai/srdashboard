(function () {
  const params = new URLSearchParams(location.search);
  const display = params.get('display') || 'master';

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

  if (display === 'shooter') {
    document.addEventListener('DOMContentLoaded', function () {
      const chrome = document.getElementById('master-chrome');
      const app = document.getElementById('shooter-app');
      if (chrome) chrome.hidden = true;
      if (app) app.hidden = false;
    });
  } else {
    document.addEventListener('DOMContentLoaded', function () {
      const app = document.getElementById('shooter-app');
      if (app) app.hidden = true;
    });
  }
})();
