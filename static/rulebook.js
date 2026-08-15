window.SRRulebook = (function () {
  let cache = {};

  function ensureModal() {
    let modal = document.getElementById('rulebook-modal');
    if (modal) return modal;
    modal = document.createElement('div');
    modal.id = 'rulebook-modal';
    modal.className = 'qr-result-modal rulebook-modal';
    modal.hidden = true;
    modal.innerHTML =
      '<div class="qr-result-backdrop" data-rulebook-close="1"></div>' +
      '<div class="qr-result-dialog rulebook-dialog" role="dialog" aria-modal="true" aria-labelledby="rulebook-title">' +
      '<div class="qr-result-head">' +
      '<h2 id="rulebook-title">Regelbuch</h2>' +
      '<button type="button" class="qr-result-close btn btn-ghost" data-rulebook-close="1" aria-label="Schließen">×</button>' +
      '</div>' +
      '<div class="rulebook-body" id="rulebook-body"></div>' +
      '</div>';
    document.body.appendChild(modal);
    modal.addEventListener('click', function (ev) {
      if (ev.target && ev.target.getAttribute('data-rulebook-close')) {
        modal.hidden = true;
      }
    });
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape' && !modal.hidden) modal.hidden = true;
    });
    return modal;
  }

  function renderSection(sec) {
    const wrap = document.createElement('section');
    wrap.className = 'rulebook-section';
    if (sec.heading) {
      const h = document.createElement('h3');
      h.textContent = sec.heading;
      wrap.appendChild(h);
    }
    (sec.paragraphs || []).forEach(function (p) {
      const el = document.createElement('p');
      el.textContent = p;
      wrap.appendChild(el);
    });
    const items = sec.items || [];
    if (items.length) {
      const ul = document.createElement('ul');
      items.forEach(function (it) {
        const li = document.createElement('li');
        li.textContent = it;
        ul.appendChild(li);
      });
      wrap.appendChild(ul);
    }
    return wrap;
  }

  function paint(modal, data, fallbackLabel) {
    const title = modal.querySelector('#rulebook-title');
    const body = modal.querySelector('#rulebook-body');
    title.textContent = (data && data.title) || fallbackLabel || 'Regelbuch';
    body.replaceChildren();
    if (data && data.summary) {
      const sum = document.createElement('p');
      sum.className = 'rulebook-summary';
      sum.textContent = data.summary;
      body.appendChild(sum);
    }
    (data && data.sections || []).forEach(function (sec) {
      body.appendChild(renderSection(sec));
    });
    if (!body.childNodes.length) {
      const p = document.createElement('p');
      p.className = 'qr-result-error';
      p.textContent = 'Kein Regelbuch für dieses Spiel.';
      body.appendChild(p);
    }
  }

  async function load(url) {
    if (cache[url]) return cache[url];
    const res = await fetch(url);
    if (!res.ok) throw new Error('Regelbuch nicht gefunden');
    const data = await res.json();
    cache[url] = data;
    return data;
  }

  async function open(opts) {
    const modal = ensureModal();
    const url = opts && opts.url;
    const label = (opts && opts.label) || 'Regelbuch';
    modal.hidden = false;
    paint(modal, { title: label, summary: 'Laden…', sections: [] }, label);
    if (!url) {
      paint(modal, null, label);
      return;
    }
    try {
      const data = await load(url);
      paint(modal, data, label);
    } catch (e) {
      const body = modal.querySelector('#rulebook-body');
      body.replaceChildren();
      const err = document.createElement('p');
      err.className = 'qr-result-error';
      err.textContent = 'Regelbuch konnte nicht geladen werden.';
      body.appendChild(err);
    }
  }

  function close() {
    const modal = document.getElementById('rulebook-modal');
    if (modal) modal.hidden = true;
  }

  function syncButton(btn, plugin) {
    if (!btn) return;
    const url = plugin && plugin.rulebookUrl;
    const isGame = plugin && plugin.kind === 'game';
    btn.hidden = !isGame || !url;
    btn.disabled = btn.hidden;
    if (!btn.hidden) {
      btn.dataset.rulebookUrl = url;
      btn.dataset.pluginLabel = plugin.label || plugin.id || 'Regelbuch';
      btn.title = 'Regelbuch — ' + (plugin.label || plugin.id);
      btn.setAttribute('aria-label', 'Regelbuch — ' + (plugin.label || plugin.id));
    }
  }

  function wireButton(btn) {
    if (!btn || btn._rulebookWired) return;
    btn._rulebookWired = true;
    btn.addEventListener('click', function () {
      open({
        url: btn.dataset.rulebookUrl,
        label: btn.dataset.pluginLabel
      });
    });
  }

  return { open, close, syncButton, wireButton, ensureModal };
})();
