// Client-side holder for the control token.
//
// The server never sends the token back (it is the credential for every
// state-changing endpoint), so each display keeps its own copy in
// localStorage and the operator enters it once per device.
window.SRAuth = (function () {
  const STORAGE_KEY = 'srdashboard-control-token';

  function get() {
    try {
      return localStorage.getItem(STORAGE_KEY) || '';
    } catch (e) {
      return '';
    }
  }

  function set(token) {
    try {
      if (token) localStorage.setItem(STORAGE_KEY, token);
      else localStorage.removeItem(STORAGE_KEY);
    } catch (e) { /* private mode: fall back to in-memory only */ }
  }

  function clear() {
    set('');
  }

  function headers(extra) {
    const h = Object.assign({ 'Content-Type': 'application/json' }, extra || {});
    const token = get();
    if (token) h['X-SR-Control-Token'] = token;
    return h;
  }

  function promptForToken(message) {
    const entered = window.prompt(message || 'Control-Token eingeben:', get());
    if (entered === null) return '';
    const token = entered.trim();
    set(token);
    return token;
  }

  // Sends a request with the stored token and, on 403, asks for the token once
  // and retries. Everything else is passed through untouched.
  async function fetchWithAuth(url, options) {
    const opts = Object.assign({}, options || {});
    opts.headers = headers(opts.headers);
    let res = await fetch(url, opts);
    if (res.status !== 403) return res;
    const token = promptForToken('Control-Token erforderlich für diese Aktion:');
    if (!token) return res;
    opts.headers = headers(options && options.headers);
    res = await fetch(url, opts);
    return res;
  }

  return { get, set, clear, headers, promptForToken, fetchWithAuth };
})();
