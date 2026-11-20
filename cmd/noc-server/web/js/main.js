// ─── UTILITIES ───
function esc(str) {
  return String(str == null ? '' : str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ─── GLOBAL STATE ───
var state = {
  simMode: 'live',          // 'live' | 'warn' | 'crit'
  layoutMode: 'desktop',    // 'tablet' | 'desktop'
  theme: 'night',           // 'day' | 'night'
  activeTab: 'noc',         // 'noc' | 'kuma-beszel' | 'wud' | 'telemetry'
  containerFilter: 'all',   // 'all' | 'unhealthy' | 'restarts'
  embedSourceMode: 'proxy', // 'proxy' | 'direct'
  embedsMounted: false,
  liveConnected: false,
  pollEverySec: 5,
  countdown: 5,
  expandedCards: new Set(),
  lastPayload: null,
  beszelDirectURL: 'http://192.168.178.90:8099/system/n9qil9mgcw9fhwm',
  beszelFrameURL:  '/system/n9qil9mgcw9fhwm',
  kumaDirectURL:   'http://192.168.178.90:3001/status/overview',
  kumaFrameURL:    '/status/overview',
  proxyAvailable:  false,
  activeBeszelSrc: '',
  activeKumaSrc:   ''
};

// ─── 1. CLOCK & SYNC COUNTDOWN ───
(function () {
  var clockEl = document.getElementById('noc-clock');
  var pulseEl = document.getElementById('noc-refresh-label');

  function tick() {
    var now = new Date();
    if (clockEl) {
      clockEl.textContent = now.toLocaleTimeString('de-DE', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      });
    }
    if (state.simMode === 'live') {
      state.countdown = state.countdown <= 1 ? state.pollEverySec : state.countdown - 1;
      if (pulseEl) {
        pulseEl.textContent = (state.liveConnected ? 'LIVE ' : 'DEMO ') + state.countdown + 'S';
      }
    } else if (pulseEl) {
      pulseEl.textContent = 'TEST';
    }
  }
  tick();
  setInterval(tick, 1000);
})();

// ─── 2. TAG / NACHT, TABLET / DESKTOP & 4-TAB SWITCHER ───
function applyTheme(theme, persist) {
  state.theme = theme === 'day' ? 'day' : 'night';
  document.body.classList.toggle('is-day', state.theme === 'day');
  document.body.classList.toggle('is-night', state.theme === 'night');
  document.querySelectorAll('[data-theme]').forEach(function (b) {
    b.setAttribute('aria-pressed', String(b.getAttribute('data-theme') === state.theme));
  });
  if (persist) {
    try { localStorage.setItem('noc_theme', state.theme); } catch (e) {}
  }
}

function applyLayout(mode, persist) {
  state.layoutMode = mode === 'tablet' ? 'tablet' : 'desktop';
  document.body.classList.toggle('mode-tablet', state.layoutMode === 'tablet');
  document.body.classList.toggle('mode-desktop', state.layoutMode === 'desktop');
  document.querySelectorAll('[data-layout]').forEach(function (b) {
    b.setAttribute('aria-pressed', String(b.getAttribute('data-layout') === state.layoutMode));
  });
  if (persist) {
    try { localStorage.setItem('noc_layout', state.layoutMode); } catch (e) {}
  }
  syncExpandedBodyClass();
}

function computeDesiredEmbedURLs() {
  var useProxy = state.embedSourceMode === 'proxy' && state.liveConnected && state.proxyAvailable;
  return {
    useProxy: useProxy,
    beszel: useProxy ? state.beszelFrameURL : state.beszelDirectURL,
    kuma:   useProxy ? state.kumaFrameURL   : state.kumaDirectURL
  };
}

function ensureEmbedFramesLoaded(forceReload) {
  var desired = computeDesiredEmbedURLs();
  var frB = document.getElementById('iframe-beszel');
  var frK = document.getElementById('iframe-kuma');
  var hB  = document.getElementById('hint-beszel');
  var hK  = document.getElementById('hint-kuma');

  if (hB) {
    hB.innerHTML = desired.useProxy
      ? ('Same-Origin-Proxy aktiv (<code>' + esc(state.beszelFrameURL) + '</code> → <code>' + esc(state.beszelDirectURL) + '</code>) — entfernt <code>X-Frame-Options</code> automatisch.')
      : ('Direktmodus aktiv (<code>' + esc(state.beszelDirectURL) + '</code>). Falls der Browser blockt, nutze <strong>Proxy</strong> oder setze <code>CSP</code> im Beszel-Container.');
  }
  if (hK) {
    hK.innerHTML = desired.useProxy
      ? ('Same-Origin-Proxy aktiv (<code>' + esc(state.kumaFrameURL) + '</code> → <code>' + esc(state.kumaDirectURL) + '</code>) — entfernt <code>X-Frame-Options</code> automatisch.')
      : ('Direktmodus aktiv (<code>' + esc(state.kumaDirectURL) + '</code>). Falls der Browser blockt, nutze <strong>Proxy</strong> oder setze <code>UPTIME_KUMA_DISABLE_FRAME_SAMEORIGIN=true</code>.');
  }

  if (state.activeTab !== 'kuma-beszel' && !state.embedsMounted) {
    return;
  }
  state.embedsMounted = true;

  if (frB && (forceReload || state.activeBeszelSrc !== desired.beszel)) {
    state.activeBeszelSrc = desired.beszel;
    frB.src = desired.beszel;
  }
  if (frK && (forceReload || state.activeKumaSrc !== desired.kuma)) {
    state.activeKumaSrc = desired.kuma;
    frK.src = desired.kuma;
  }
}

function applyTab(tab) {
  state.activeTab = tab;
  ['noc', 'kuma-beszel', 'wud', 'telemetry'].forEach(function (t) {
    document.body.classList.toggle('tab-' + t, t === tab);
  });
  document.querySelectorAll('[data-tab]').forEach(function (b) {
    b.setAttribute('aria-pressed', String(b.getAttribute('data-tab') === tab));
  });
  window.scrollTo({ top: 0, behavior: 'instant' });
  syncExpandedBodyClass();
  if (tab === 'kuma-beszel') {
    ensureEmbedFramesLoaded(false);
  }
}

function setContainerFilter(filter) {
  state.containerFilter = filter || 'all';
  document.querySelectorAll('[data-cfilter]').forEach(function (b) {
    b.setAttribute('aria-pressed', String(b.getAttribute('data-cfilter') === state.containerFilter));
  });
  if (state.lastPayload) {
    renderContainerExplorer(state.lastPayload.containers);
  }
}

function syncExpandedBodyClass() {
  var anyOpenOnNoc = false;
  if (state.activeTab === 'noc') {
    document.querySelectorAll('#pane-noc .is-expandable.is-open').forEach(function () {
      anyOpenOnNoc = true;
    });
  }
  document.body.classList.toggle('has-expanded', anyOpenOnNoc);
}

(function () {
  var savedTheme = null;
  var savedLayout = null;
  var savedEmbedMode = null;
  try {
    savedTheme = localStorage.getItem('noc_theme');
    savedLayout = localStorage.getItem('noc_layout');
    savedEmbedMode = localStorage.getItem('noc_embed_mode');
  } catch (e) {}

  applyTheme(savedTheme === 'day' ? 'day' : 'night', false);

  if (savedLayout === 'tablet' || savedLayout === 'desktop') {
    applyLayout(savedLayout, false);
  } else {
    var autoDesktop = window.innerWidth >= 900 && !window.matchMedia('(pointer: coarse)').matches;
    applyLayout(autoDesktop ? 'desktop' : 'tablet', false);
  }

  if (savedEmbedMode === 'direct' || savedEmbedMode === 'proxy') {
    state.embedSourceMode = savedEmbedMode;
    document.querySelectorAll('[data-embed-mode]').forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.getAttribute('data-embed-mode') === savedEmbedMode));
    });
  }

  document.querySelectorAll('[data-theme]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyTheme(btn.getAttribute('data-theme'), true);
    });
  });

  document.querySelectorAll('[data-layout]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyLayout(btn.getAttribute('data-layout'), true);
    });
  });

  document.querySelectorAll('[data-tab]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyTab(btn.getAttribute('data-tab'));
    });
  });

  var embedGrid = document.getElementById('embed-grid');
  document.querySelectorAll('[data-embed-layout]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var l = btn.getAttribute('data-embed-layout');
      document.querySelectorAll('[data-embed-layout]').forEach(function (b) {
        b.setAttribute('aria-pressed', String(b === btn));
      });
      if (embedGrid) embedGrid.className = 'embed-grid layout-' + l;
    });
  });

  document.querySelectorAll('[data-embed-mode]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      state.embedSourceMode = btn.getAttribute('data-embed-mode') === 'direct' ? 'direct' : 'proxy';
      try { localStorage.setItem('noc_embed_mode', state.embedSourceMode); } catch (e) {}
      document.querySelectorAll('[data-embed-mode]').forEach(function (b) {
        b.setAttribute('aria-pressed', String(b.getAttribute('data-embed-mode') === state.embedSourceMode));
      });
      ensureEmbedFramesLoaded(true);
    });
  });

  var relB = document.getElementById('reload-beszel');
  var relK = document.getElementById('reload-kuma');
  if (relB) {
    relB.addEventListener('click', function () {
      var fr = document.getElementById('iframe-beszel');
      var desired = computeDesiredEmbedURLs();
      if (fr) {
        state.activeBeszelSrc = desired.beszel;
        fr.src = desired.beszel;
      }
    });
  }
  if (relK) {
    relK.addEventListener('click', function () {
      var fr = document.getElementById('iframe-kuma');
      var desired = computeDesiredEmbedURLs();
      if (fr) {
        state.activeKumaSrc = desired.kuma;
        fr.src = desired.kuma;
      }
    });
  }

  document.querySelectorAll('[data-cfilter]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      setContainerFilter(btn.getAttribute('data-cfilter'));
    });
  });
})();

// ─── 3. EXPANDABLE INFO CARDS (DELEGATED CLICK — PERSISTS ACROSS LIVE POLLS!) ───
document.addEventListener('click', function (e) {
  if (e.target.closest('a, button')) return;
  var card = e.target.closest('.is-expandable');
  if (!card) return;
  var id = card.getAttribute('data-card-id');
  if (!id) return;

  var isOpen = card.classList.toggle('is-open');
  if (isOpen) {
    state.expandedCards.add(id);
  } else {
    state.expandedCards.delete(id);
  }
  syncExpandedBodyClass();
});

// ─── 4. 3-CIRCLE CMY PERIODIC ORBIT + DRAGGABLE MAGENTA SPHERE ───
(function () {
  var svg = document.querySelector('[data-spin] svg');
  if (svg && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    var ease = 'cubic-bezier(0.65, 0, 0.35, 1)';
    function spinMarks() {
      if (document.hidden) return;
      var cY = svg.querySelector('.c-yellow');
      var cC = svg.querySelector('.c-cyan');
      var cM = svg.querySelector('.c-magenta');
      if (!cY || !cC || !cM) return;
      cY.animate([
        { transform: 'translate3d(0, 0, 0)', easing: ease },
        { transform: 'translate3d(-25px, 43px, 0)', offset: 0.35, easing: ease },
        { transform: 'translate3d(25px, 43px, 0)', offset: 0.7, easing: ease },
        { transform: 'translate3d(0, 0, 0)' }
      ], { duration: 1600 });
      cC.animate([
        { transform: 'translate3d(0, 0, 0)', easing: ease },
        { transform: 'translate3d(50px, 0px, 0)', offset: 0.35, easing: ease },
        { transform: 'translate3d(25px, -43px, 0)', offset: 0.7, easing: ease },
        { transform: 'translate3d(0, 0, 0)' }
      ], { duration: 1600 });
      cM.animate([
        { transform: 'translate3d(0, 0, 0)', easing: ease },
        { transform: 'translate3d(-25px, -43px, 0)', offset: 0.35, easing: ease },
        { transform: 'translate3d(-50px, 0px, 0)', offset: 0.7, easing: ease },
        { transform: 'translate3d(0, 0, 0)' }
      ], { duration: 1600 });
    }
    setInterval(spinMarks, 15000);
  }

  var mag = document.getElementById('sphere-magenta');
  if (!mag) return;
  var dragging = false, startX = 0, startY = 0, curX = 0, curY = 0, toggled = false;

  mag.addEventListener('pointerdown', function (e) {
    dragging = true;
    toggled = false;
    startX = e.clientX - curX;
    startY = e.clientY - curY;
    mag.setPointerCapture(e.pointerId);
    mag.style.transition = 'none';
  });

  mag.addEventListener('pointermove', function (e) {
    if (!dragging) return;
    curX = Math.max(-70, Math.min(50, e.clientX - startX));
    curY = Math.max(-55, Math.min(45, e.clientY - startY));
    mag.style.transform = 'translate3d(' + curX + 'px, ' + curY + 'px, 0)';
    if (!toggled && Math.hypot(curX + 22, curY + 14) < 16) {
      toggled = true;
      applyTheme(state.theme === 'night' ? 'day' : 'night', true);
    }
  });

  window.addEventListener('pointerup', function () {
    if (!dragging) return;
    dragging = false;
    curX = 0; curY = 0;
    mag.style.transition = 'transform 0.45s cubic-bezier(0.22, 1, 0.36, 1)';
    mag.style.transform = 'translate3d(0, 0, 0)';
  });
})();

// ─── 5. FULLSCREEN & SCREEN WAKE LOCK ───
(function () {
  var fsBtn = document.getElementById('btn-kiosk-fs');
  async function requestWakeLock() {
    try { if ('wakeLock' in navigator) await navigator.wakeLock.request('screen'); } catch (e) {}
  }
  if (fsBtn) {
    fsBtn.addEventListener('click', function () {
      if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen().catch(function () {});
        requestWakeLock();
        fsBtn.setAttribute('aria-pressed', 'true');
      } else {
        document.exitFullscreen().catch(function () {});
        fsBtn.setAttribute('aria-pressed', 'false');
      }
    });
  }
  requestWakeLock();
})();

// ─── 6. LIVE TELEMETRY CHARTS, CONTAINER EXPLORER & SSL INSPECTOR ───
function buildPoints(arr, w, h, maxVal, padY) {
  if (!arr || arr.length < 2) return '0,' + (h - padY) + ' ' + w + ',' + (h - padY);
  var n = arr.length;
  var cap = Math.max(maxVal, 0.001);
  return arr.map(function (v, i) {
    var x = Math.round((i / (n - 1)) * w);
    var ratio = Math.min(1, Math.max(0, v / cap));
    var y = Math.round((h - padY) - ratio * (h - padY * 2));
    return x + ',' + y;
  }).join(' ');
}

function formatMbps(mbps) {
  if (mbps < 0.01) return (mbps * 1000).toFixed(1) + ' Kbit/s';
  if (mbps < 1) return Math.round(mbps * 1000) + ' Kbit/s';
  if (mbps >= 100) return Math.round(mbps) + ' Mbit/s';
  return mbps.toFixed(1) + ' Mbit/s';
}

function renderLiveCharts(data) {
  if (!data || !data.infrastructure) return;
  var inf = data.infrastructure;
  var cpuG = (inf.gauges || []).find(function (g) { return g.id === 'cpu'; });
  var ramG = (inf.gauges || []).find(function (g) { return g.id === 'ram'; });

  var cpuValEl = document.getElementById('chart-cpu-val');
  var ramValEl = document.getElementById('chart-ram-val');
  var netValEl = document.getElementById('chart-net-val');
  var netTotEl = document.getElementById('chart-net-totals');
  var netPeakEl = document.getElementById('chart-net-peak');
  var metaEl   = document.getElementById('tele-host-meta');

  if (metaEl && data.hostname) {
    metaEl.textContent = 'Host ' + data.hostname + ' · Uptime ' + (data.uptime_human || 'aktiv') + ' · Live 2s-Sampling';
  }
  if (cpuValEl && cpuG) cpuValEl.textContent = cpuG.value.toFixed(1) + ' %';
  if (ramValEl && ramG) ramValEl.textContent = ramG.value.toFixed(1) + ' %';

  var cpuPts = buildPoints(inf.cpu_history || [0, 0], 360, 110, 100, 6);
  var ramPts = buildPoints(inf.ram_history || [0, 0], 360, 110, 100, 6);

  var cpuLine = document.getElementById('chart-cpu-line');
  var cpuArea = document.getElementById('chart-cpu-area');
  var ramLine = document.getElementById('chart-ram-line');
  var ramArea = document.getElementById('chart-ram-area');
  if (cpuLine) cpuLine.setAttribute('points', cpuPts);
  if (cpuArea) cpuArea.setAttribute('points', '0,110 ' + cpuPts + ' 360,110');
  if (ramLine) ramLine.setAttribute('points', ramPts);
  if (ramArea) ramArea.setAttribute('points', '0,110 ' + ramPts + ' 360,110');

  if (inf.network) {
    var net = inf.network;
    if (netValEl) netValEl.textContent = '↓ ' + net.inbound_str + ' · ↑ ' + net.outbound_str;
    if (netTotEl) netTotEl.textContent = 'NIC ' + (net.interface || 'eth0') + ' · RX ' + (net.rx_total_gb || 0).toFixed(2) + ' GB · TX ' + (net.tx_total_gb || 0).toFixed(2) + ' GB';
    var allNet = (net.in_history || []).concat(net.out_history || []);
    var rawPeak = allNet.reduce(function (m, x) { return Math.max(m, x); }, 0);
    var chartCap = Math.max(rawPeak * 1.2, 0.01);
    if (netPeakEl) netPeakEl.textContent = 'Peak: ' + formatMbps(rawPeak);
    var nIn = document.getElementById('chart-net-in');
    var nOut = document.getElementById('chart-net-out');
    if (nIn) nIn.setAttribute('points', buildPoints(net.in_history || [0, 0], 360, 110, chartCap, 6));
    if (nOut) nOut.setAttribute('points', buildPoints(net.out_history || [0, 0], 360, 110, chartCap, 6));
  }
}

function renderContainerExplorer(containers) {
  var grid = document.getElementById('container-explorer-grid');
  if (!grid) return;
  if (!Array.isArray(containers) || containers.length === 0) {
    grid.innerHTML = '<div class="wud-empty-card is-warn">Keine Docker-Container über /var/run/docker.sock gefunden (im Live-Docker-Stack automatisch befüllt).</div>';
    return;
  }
  var filtered = containers.filter(function (c) {
    if (state.containerFilter === 'unhealthy') return c.health === 'unhealthy';
    if (state.containerFilter === 'restarts') return c.restarts > 0;
    return true;
  });
  if (filtered.length === 0) {
    grid.innerHTML = '<div class="wud-empty-card">✓ Keine Container in dieser Filterkategorie.</div>';
    return;
  }
  grid.innerHTML = filtered.map(function (c) {
    var cid = 'ce-' + c.name;
    var openCls = state.expandedCards.has(cid) ? ' is-open' : '';
    var portsStr = (c.ports && c.ports.length) ? c.ports.join(', ') : 'intern (kein Host-Port)';
    return (
      '<article class="ce-card health-' + esc(c.health) + ' is-expandable' + openCls + '" data-card-id="' + esc(cid) + '">' +
        '<div class="ce-head">' +
          '<span class="ce-name">' + esc(c.name) + '</span>' +
          '<div class="card-head-right">' +
            '<span class="ce-pill">' + esc(c.health) + '</span>' +
            '<span class="chevron">▾</span>' +
          '</div>' +
        '</div>' +
        '<div class="ce-sub">' +
          '<span>Stack: ' + esc(c.stack) + '</span>' +
          '<span>Restarts: ' + esc(c.restarts) + '</span>' +
        '</div>' +
        '<div class="card-drawer">' +
          '<div class="drawer-line"><span>Image</span><code>' + esc(c.image) + '</code></div>' +
          '<div class="drawer-line"><span>Status</span><span>' + esc(c.status_text) + '</span></div>' +
          '<div class="drawer-line"><span>Ports</span><code>' + esc(portsStr) + '</code></div>' +
          (c.oom_killed ? '<div class="drawer-line"><span>OOMKilled</span><strong>JA (Speicherlimit überschritten!)</strong></div>' : '') +
        '</div>' +
      '</article>'
    );
  }).join('');
}

function renderSSLInspector(certs) {
  var grid = document.getElementById('ssl-inspector-grid');
  if (!grid) return;
  if (!Array.isArray(certs) || certs.length === 0) {
    grid.innerHTML = '<div class="wud-empty-card is-warn">Keine SSL-Domains konfiguriert (ENV <code>SSL_DOMAINS</code>).</div>';
    return;
  }
  grid.innerHTML = certs.map(function (c) {
    var cid = 'ssl-' + c.domain;
    var openCls = state.expandedCards.has(cid) ? ' is-open' : '';
    var cls = !c.valid || c.days_left < 7 ? ' is-crit' : (c.days_left <= 21 ? ' is-warn' : '');
    var badge = !c.valid ? 'FEHLER' : (c.days_left + ' TAGE');
    return (
      '<article class="ssl-card' + cls + ' is-expandable' + openCls + '" data-card-id="' + esc(cid) + '">' +
        '<div class="ce-head">' +
          '<span class="ce-name">' + esc(c.domain) + '</span>' +
          '<div class="card-head-right">' +
            '<span class="ce-pill">' + esc(badge) + '</span>' +
            '<span class="chevron">▾</span>' +
          '</div>' +
        '</div>' +
        '<div class="ce-sub">' +
          '<span>' + esc(c.valid ? ('Gültig bis ' + c.not_after) : (c.error || 'TLS-Fehler')) + '</span>' +
          '<span>Port 443</span>' +
        '</div>' +
        '<div class="card-drawer">' +
          '<div class="drawer-line"><span>Aussteller</span><span>' + esc(c.issuer || 'n/a') + '</span></div>' +
          '<div class="drawer-line"><span>Ablaufdatum</span><span>' + esc(c.not_after || 'n/a') + '</span></div>' +
          '<div class="drawer-actions"><a class="win-btn" href="https://' + esc(c.domain) + '" target="_blank" rel="noopener">HTTPS öffnen ↗</a></div>' +
        '</div>' +
      '</article>'
    );
  }).join('');
}

// ─── 7. MAIN LIVE PAYLOAD RENDERER ───
function renderWUD(wud) {
  if (!wud) return;
  var wudStatCard = document.getElementById('cnt-wud');
  var wudStatNum  = document.getElementById('cnt-wud-num');
  var wudStatSub  = document.getElementById('cnt-wud-sub');
  var wudDrawer   = document.querySelector('[data-drawer-for="cnt-wud"]');
  var wudTabBtn   = document.getElementById('tab-btn-wud');
  var wudGridEl   = document.getElementById('wud-updates-grid');
  var wudSumEl    = document.getElementById('wud-status-summary');
  var openWudEl   = document.getElementById('open-wud');

  if (openWudEl) {
    if (wud.url) { openWudEl.href = wud.url; openWudEl.hidden = false; }
    else { openWudEl.hidden = true; }
  }

  if (wudStatCard && wudStatNum && wudStatSub) {
    var open = state.expandedCards.has('cnt-wud') ? ' is-open' : '';
    if (!wud.available) {
      wudStatCard.className = 'cnt-stat is-expandable' + open;
      wudStatNum.textContent = '–';
      wudStatSub.textContent = wud.error || 'WUD_URL setzen';
    } else if (wud.updates_available > 0) {
      wudStatCard.className = 'cnt-stat is-expandable status-warn' + open;
      wudStatNum.textContent = String(wud.updates_available);
      wudStatSub.textContent = 'von ' + wud.total_watched + ' Containern';
    } else {
      wudStatCard.className = 'cnt-stat is-expandable' + open;
      wudStatNum.textContent = '0';
      wudStatSub.textContent = wud.total_watched + ' aktuell';
    }
  }

  if (wudDrawer) {
    if (!wud.available) {
      wudDrawer.innerHTML = '<div class="drawer-line"><span>Status</span><span>' + esc(wud.error || 'WUD nicht verbunden') + '</span></div>';
    } else if (wud.updates && wud.updates.length > 0) {
      wudDrawer.innerHTML = wud.updates.slice(0, 4).map(function (u) {
        return '<div class="drawer-line"><span>' + esc(u.container) + '</span><span>' + esc(u.current_tag) + ' → ' + esc(u.new_tag) + '</span></div>';
      }).join('') + '<div class="drawer-actions"><button type="button" class="win-btn" onclick="applyTab(\'wud\')">Alle im WUD-Reiter öffnen →</button></div>';
    } else {
      wudDrawer.innerHTML = '<div class="drawer-line"><span>Status</span><span>Alle ' + esc(wud.total_watched) + ' Container aktuell</span></div>';
    }
  }

  if (wudTabBtn) {
    wudTabBtn.textContent = (wud.available && wud.updates_available > 0) ? ('WUD (' + wud.updates_available + ')') : 'WUD';
  }
  if (wudSumEl) {
    wudSumEl.textContent = wud.available
      ? (wud.updates_available + ' Update(s) verfügbar · ' + wud.total_watched + ' überwachte Container')
      : (wud.error || 'WUD nicht verbunden (ENV WUD_URL setzen)');
  }

  if (wudGridEl) {
    if (!wud.available) {
      wudGridEl.innerHTML = '<div class="wud-empty-card is-warn">WUD API aktuell nicht erreichbar (' + esc(wud.error || 'kein WUD-Port erkannt') + '). Setze in Portainer <code>WUD_URL=http://192.168.178.90:&lt;PORT&gt;</code>.</div>';
    } else if (!wud.updates || wud.updates.length === 0) {
      wudGridEl.innerHTML = '<div class="wud-empty-card">✓ Alle ' + esc(wud.total_watched) + ' überwachten Container laufen auf dem neuesten Image-Stand.</div>';
    } else {
      wudGridEl.innerHTML = wud.updates.map(function (u) {
        var isMaj = u.semver_diff === 'major';
        var diffLabel = u.semver_diff ? u.semver_diff.toUpperCase() : 'UPDATE';
        return (
          '<article class="wud-card' + (isMaj ? ' is-major' : '') + '">' +
            '<div class="wud-card-top">' +
              '<span class="wud-name">' + esc(u.container) + '</span>' +
              '<div class="wud-badges">' +
                (u.caution_db ? '<span class="wud-db-pill" title="Datenbank-App: Vor Update Snapshot/Backup prüfen!">DB-Backup</span>' : '') +
                '<span class="wud-diff-pill">' + esc(diffLabel) + '</span>' +
              '</div>' +
            '</div>' +
            '<div class="wud-image">' + esc(u.image) + '</div>' +
            '<div class="wud-version-row">' +
              '<span class="wud-ver-old">' + esc(u.current_tag) + '</span>' +
              '<span>→</span>' +
              '<span class="wud-ver-new">' + esc(u.new_tag) + '</span>' +
              (u.release_link ? ('<a class="win-btn" href="' + esc(u.release_link) + '" target="_blank" rel="noopener">Notes ↗</a>') : '') +
            '</div>' +
          '</article>'
        );
      }).join('');
    }
  }
}

function renderEmbeds(embeds, beszel, kuma) {
  if (embeds) {
    state.proxyAvailable = Boolean(embeds.proxy_enabled);
    if (embeds.beszel_url) {
      state.beszelDirectURL = embeds.beszel_url;
      var lblB = document.getElementById('url-label-beszel');
      var opnB = document.getElementById('open-beszel');
      if (lblB) lblB.textContent = embeds.beszel_url;
      if (opnB) opnB.href = embeds.beszel_url;
    }
    if (embeds.beszel_frame_url) {
      state.beszelFrameURL = embeds.beszel_frame_url;
    }
    if (embeds.kuma_url) {
      state.kumaDirectURL = embeds.kuma_url;
      var lblK = document.getElementById('url-label-kuma');
      var opnK = document.getElementById('open-kuma');
      if (lblK) lblK.textContent = embeds.kuma_url;
      if (opnK) opnK.href = embeds.kuma_url;
    }
    if (embeds.kuma_frame_url) {
      state.kumaFrameURL = embeds.kuma_frame_url;
    }
    var dotB = document.getElementById('dot-beszel');
    var dotK = document.getElementById('dot-kuma');
    if (dotB) dotB.className = 'win-dot ' + (embeds.beszel_online ? 'is-up' : 'is-down');
    if (dotK) dotK.className = 'win-dot ' + (embeds.kuma_online ? 'is-up' : 'is-down');
    ensureEmbedFramesLoaded(false);
  }

  var beszelBar = document.getElementById('beszel-hosts-bar');
  if (beszelBar) {
    if (beszel && beszel.available && Array.isArray(beszel.hosts) && beszel.hosts.length > 0) {
      beszelBar.hidden = false;
      beszelBar.innerHTML = beszel.hosts.map(function (h) {
        var down = h.status !== 'up';
        return (
          '<div class="beszel-host-chip' + (down ? ' is-down' : '') + '">' +
            '<strong>Beszel · ' + esc(h.name) + ' (' + esc(h.status.toUpperCase()) + ')</strong>' +
            '<span class="beszel-host-stats">CPU ' + esc(h.cpu) + '% · RAM ' + esc(h.mem) + '% · Disk ' + esc(h.disk) + '%' + (h.temp > 0 ? (' · ' + esc(h.temp) + '°C') : '') + '</span>' +
          '</div>'
        );
      }).join('');
    } else {
      beszelBar.hidden = true;
    }
  }

  var kumaBar = document.getElementById('kuma-monitors-bar');
  if (kumaBar) {
    if (kuma && kuma.available && Array.isArray(kuma.monitors) && kuma.monitors.length > 0) {
      kumaBar.hidden = false;
      kumaBar.innerHTML = kuma.monitors.map(function (m) {
        return (
          '<div class="beszel-host-chip' + (!m.online ? ' is-down' : '') + '">' +
            '<strong>Kuma · ' + esc(m.name) + '</strong>' +
            '<span class="beszel-host-stats">' + (m.online ? ('ONLINE · ' + esc(m.latency_ms) + ' ms') : 'OFFLINE') + '</span>' +
          '</div>'
        );
      }).join('');
    } else {
      kumaBar.hidden = true;
    }
  }
}

function renderPayload(data) {
  if (!data) return;
  state.lastPayload = data;

  var hostSubEl = document.getElementById('topbar-host-sub');
  var verdictEl = document.getElementById('global-verdict');
  var verdictTx = document.getElementById('global-verdict-text');
  var bannerSrc = document.getElementById('banner-sources');
  var row1Src   = document.getElementById('row1-source');
  var row3Src   = document.getElementById('row3-source');
  var row4Src   = document.getElementById('row4-source');

  if (hostSubEl && data.hostname) {
    hostSubEl.textContent = data.hostname + ' · Uptime ' + (data.uptime_human || 'aktiv');
  }
  if (verdictEl && verdictTx && data.verdict) {
    var vCls = data.verdict.level === 'crit' ? ' is-crit' : (data.verdict.level === 'warn' ? ' is-warn' : '');
    verdictEl.className = 'global-verdict' + vCls;
    verdictTx.textContent = data.verdict.text;
  }
  if (data.sources && data.sources.length) {
    if (row1Src) row1Src.textContent = 'Live-Quellen: ' + data.sources.join(' · ');
    if (bannerSrc) bannerSrc.textContent = 'HOMELAB OPERATIONS CENTER · ' + (data.host_ip || '192.168.178.90') + ' · ' + data.sources.join(' · ');
  }

  // Zeile 1: Executive Summary (Expandable!)
  if (Array.isArray(data.executive_summary)) {
    data.executive_summary.forEach(function (item) {
      var cardId = 'exec-' + item.id;
      var card = document.querySelector('[data-exec-id="' + item.id + '"]');
      if (!card) return;
      var openCls = state.expandedCards.has(cardId) ? ' is-open' : '';
      card.className = 'stat-card is-expandable' + (item.status && item.status !== 'ok' ? ' status-' + item.status : '') + openCls;
      var valEl = card.querySelector('.stat-value');
      var srcEl = card.querySelector('.stat-src-chip');
      var subEl = card.querySelector('.stat-sub');
      var drwEl = card.querySelector('[data-drawer-for="' + cardId + '"]');
      if (valEl) valEl.textContent = item.value;
      if (srcEl) srcEl.textContent = item.source;
      if (subEl) subEl.textContent = item.sub;
      if (drwEl) {
        var dets = Array.isArray(item.details) && item.details.length ? item.details : [item.sub];
        drwEl.innerHTML = dets.slice(0, 6).map(function (line) {
          return '<div class="drawer-line"><span>' + esc(line) + '</span></div>';
        }).join('');
      }
    });
  }

  // Zeile 2: Kritische Dienste (Reine binäre Statusmatrix — nicht anklickbar)
  var matrixEl = document.getElementById('services-matrix');
  if (matrixEl && Array.isArray(data.critical_services)) {
    var cols = Math.min(Math.max(data.critical_services.length, 3), 6);
    matrixEl.style.setProperty('--svc-cols', String(cols));
    matrixEl.innerHTML = data.critical_services.map(function (svc) {
      var offCls = svc.online ? '' : ' is-offline';
      var badge = svc.online ? '✅ ONLINE' : '🚨 OFFLINE';
      return (
        '<article class="svc-tile' + offCls + '">' +
          '<div class="svc-head">' +
            '<h3 class="svc-name">' + esc(svc.name) + '</h3>' +
            '<span class="svc-badge">' + badge + '</span>' +
          '</div>' +
          '<div class="svc-foot">' +
            '<span>' + esc(svc.category) + '</span>' +
            '<span class="svc-latency">' + esc(svc.latency) + '</span>' +
          '</div>' +
        '</article>'
      );
    }).join('');
  }

  // Zeile 3: Infrastruktur Gauges + Live Network Sparkline (Expandable!)
  if (data.infrastructure) {
    if (row3Src && data.infrastructure.source) row3Src.textContent = data.infrastructure.source;
    (data.infrastructure.gauges || []).forEach(function (g) {
      var cid = 'gauge-' + g.id;
      var card = document.querySelector('[data-gauge-id="' + g.id + '"]');
      if (!card) return;
      var openCls = state.expandedCards.has(cid) ? ' is-open' : '';
      card.className = 'gauge-card is-expandable' + (g.status && g.status !== 'ok' ? ' status-' + g.status : '') + openCls;
      var numEl = card.querySelector('.gauge-num');
      var tagEl = card.querySelector('.gauge-state-tag');
      var detEl = card.querySelector('.gauge-detail');
      var fillEl = card.querySelector('.gauge-fill');
      var drwEl = card.querySelector('[data-drawer-for="' + cid + '"]');
      if (numEl) numEl.textContent = Math.round(g.value) + ' ' + g.unit;
      if (tagEl) tagEl.textContent = g.state_tag;
      if (detEl) detEl.textContent = g.detail;
      if (fillEl) fillEl.style.width = Math.min(100, Math.max(0, g.value)) + '%';
      if (drwEl) {
        var extra = Array.isArray(g.extra) && g.extra.length ? g.extra : [g.detail];
        drwEl.innerHTML = extra.map(function (line) {
          return '<div class="drawer-line"><span>' + esc(line) + '</span></div>';
        }).join('') + '<div class="drawer-actions"><button type="button" class="win-btn" onclick="applyTab(\'telemetry\')">Live-Verlauf ansehen →</button></div>';
      }
    });

    if (data.infrastructure.network) {
      var net = data.infrastructure.network;
      var ifEl = document.getElementById('net-iface-label');
      var inEl = document.getElementById('net-in-val');
      var outEl = document.getElementById('net-out-val');
      var spIn = document.getElementById('spark-in');
      var spOut = document.getElementById('spark-out');
      var netDrw = document.querySelector('[data-drawer-for="gauge-net"]');
      if (ifEl) ifEl.textContent = 'NIC ' + (net.interface || 'eth0');
      if (inEl) inEl.textContent = net.inbound_str;
      if (outEl) outEl.textContent = net.outbound_str;
      var allRates = (net.in_history || []).concat(net.out_history || []);
      var maxRate = allRates.reduce(function (m, x) { return Math.max(m, x); }, 0.01);
      if (spIn) spIn.setAttribute('points', buildPoints((net.in_history || []).slice(-18), 240, 26, maxRate * 1.15, 3));
      if (spOut) spOut.setAttribute('points', buildPoints((net.out_history || []).slice(-18), 240, 26, maxRate * 1.15, 3));
      if (netDrw) {
        netDrw.innerHTML =
          '<div class="drawer-line"><span>Interface</span><code>' + esc(net.interface || 'eth0') + '</code></div>' +
          '<div class="drawer-line"><span>Empfangen (RX)</span><span>' + esc((net.rx_total_gb || 0).toFixed(2)) + ' GB</span></div>' +
          '<div class="drawer-line"><span>Gesendet (TX)</span><span>' + esc((net.tx_total_gb || 0).toFixed(2)) + ' GB</span></div>' +
          '<div class="drawer-actions"><button type="button" class="win-btn" onclick="applyTab(\'telemetry\')">Live-Graph öffnen →</button></div>';
      }
    }
  }

  // Zeile 4: Container Health (Expandable!)
  if (data.container_health) {
    var ch = data.container_health;
    if (row4Src) row4Src.textContent = 'Quellen: ' + ch.source + ' & WUD';

    var cH = document.getElementById('cnt-healthy');
    var cU = document.getElementById('cnt-unhealthy');
    var cR = document.getElementById('cnt-restarts');
    var cSub = document.getElementById('cnt-healthy-sub');

    if (cH) {
      cH.className = 'cnt-stat is-expandable' + (!ch.available ? ' status-warn' : (ch.unhealthy > 0 ? ' status-warn' : '')) + (state.expandedCards.has('cnt-healthy') ? ' is-open' : '');
      cH.querySelector('.cnt-num').textContent = ch.available ? String(ch.healthy) : 'n/a';
      if (cSub) cSub.textContent = ch.available ? ('von ' + ch.total + ' Containern') : 'Socket fehlt';
      var dh = cH.querySelector('[data-drawer-for="cnt-healthy"]');
      if (dh) dh.innerHTML = '<div class="drawer-line"><span>Aktiv &amp; gesund</span><span>' + esc(ch.healthy) + ' / ' + esc(ch.total) + '</span></div><div class="drawer-actions"><button type="button" class="win-btn" onclick="applyTab(\'telemetry\')">Container Explorer →</button></div>';
    }
    if (cU) {
      cU.className = 'cnt-stat is-expandable' + (ch.unhealthy > 0 ? ' status-crit' : '') + (state.expandedCards.has('cnt-unhealthy') ? ' is-open' : '');
      cU.querySelector('.cnt-num').textContent = ch.available ? String(ch.unhealthy) : 'n/a';
      var du = cU.querySelector('[data-drawer-for="cnt-unhealthy"]');
      if (du) du.innerHTML = '<div class="drawer-line"><span>Unhealthy / Crashed</span><span>' + esc(ch.unhealthy) + '</span></div><div class="drawer-actions"><button type="button" class="win-btn" onclick="setContainerFilter(\'unhealthy\');applyTab(\'telemetry\')">Filtern →</button></div>';
    }
    if (cR) {
      cR.className = 'cnt-stat is-expandable' + (ch.restarts >= 5 ? ' status-crit' : (ch.restarts > 0 ? ' status-warn' : '')) + (state.expandedCards.has('cnt-restarts') ? ' is-open' : '');
      cR.querySelector('.cnt-num').textContent = ch.available ? String(ch.restarts) : 'n/a';
      var dr = cR.querySelector('[data-drawer-for="cnt-restarts"]');
      if (dr) {
        if (ch.top_restarts && ch.top_restarts.length) {
          dr.innerHTML = ch.top_restarts.map(function (r) {
            return '<div class="drawer-line"><span>' + esc(r.name) + '</span><span>x' + esc(r.count) + '</span></div>';
          }).join('');
        } else {
          dr.innerHTML = '<div class="drawer-line"><span>Neustarts</span><span>0 · Alles stabil</span></div>';
        }
      }
    }

    var rl = document.getElementById('restart-list');
    if (rl) {
      var hasR = ch.available && ch.top_restarts && ch.top_restarts.length > 0;
      var hasW = data.wud && data.wud.available && data.wud.updates && data.wud.updates.length > 0;
      if (!ch.available) {
        rl.innerHTML = '<li class="restart-item"><div class="restart-reason">Docker Socket nicht gemountet</div></li>';
      } else if (hasR) {
        rl.innerHTML = ch.top_restarts.slice(0, 2).map(function (r) {
          return '<li class="restart-item"><div><div class="restart-name">' + esc(r.name) + '</div><div class="restart-reason">' + esc(r.reason) + '</div></div><span class="restart-count">' + esc(r.count) + '</span></li>';
        }).join('');
      } else if (hasW) {
        rl.innerHTML = data.wud.updates.slice(0, 2).map(function (u) {
          return '<li class="restart-item"><div><div class="restart-name">' + esc(u.container) + '</div><div class="restart-reason">' + esc(u.current_tag) + ' → ' + esc(u.new_tag) + '</div></div><span class="restart-count">UPD</span></li>';
        }).join('');
      } else {
        rl.innerHTML = '<li class="restart-empty"><span>✓ Alle Container ohne Neustarts</span><span>0 Restarts</span></li>';
      }
    }
  }

  // Zeile 5: Incidents (Expandable!)
  var incList = document.getElementById('incidents-list');
  if (incList && Array.isArray(data.incidents)) {
    incList.innerHTML = data.incidents.slice(0, 5).map(function (inc, idx) {
      var cid = 'inc-' + idx + '-' + (inc.title || '').slice(0, 16);
      var openCls = state.expandedCards.has(cid) ? ' is-open' : '';
      return (
        '<li class="incident-card level-' + esc(inc.level) + ' is-expandable' + openCls + '" data-card-id="' + esc(cid) + '">' +
          '<div class="inc-row-main">' +
            '<div class="inc-left">' +
              '<span class="inc-badge">' + esc(inc.badge) + '</span>' +
              '<div class="inc-text">' +
                '<span class="inc-title">' + esc(inc.title) + '</span>' +
                '<span class="inc-meta">' + esc(inc.meta) + '</span>' +
              '</div>' +
            '</div>' +
            '<div class="card-head-right">' +
              '<span class="inc-time">' + esc(inc.time) + '</span>' +
              '<span class="chevron">▾</span>' +
            '</div>' +
          '</div>' +
          '<div class="card-drawer">' +
            '<div class="drawer-line"><span>' + esc(inc.detail || inc.meta) + '</span></div>' +
          '</div>' +
        '</li>'
      );
    }).join('');
  }

  renderWUD(data.wud);
  renderEmbeds(data.embeds, data.beszel, data.kuma);
  renderLiveCharts(data);
  renderContainerExplorer(data.containers);
  renderSSLInspector(data.ssl_certs);
}

// ─── 8. LIVE POLLING & TEST SCENARIOS (ONLY MORITZ' REAL SERVICES) ───
(function () {
  var demoData = {
    hostname: 'homelab-192.168.178.90',
    host_ip: '192.168.178.90',
    uptime_human: '14d 6h',
    sources: ['Docker Engine', 'Host /proc', 'Uptime Kuma', 'Beszel', 'WUD'],
    verdict: { level: 'ok', text: 'ALLE SYSTEME NOMINAL · VORSCHAU (LIVE AKTIV IM DOCKER-STACK)' },
    executive_summary: [
      { id: 'services-online', value: '6 / 6', source: 'Uptime Kuma', status: 'ok', sub: '100 % Verfügbarkeit', details: ['✓ Vaultwarden (4 ms)', '✓ Immich (8 ms)', '✓ Paperless-ngx (6 ms)', '✓ Nginx Proxy Manager (3 ms)', '✓ Uptime Kuma (5 ms)', '✓ Website (2 ms)'] },
      { id: 'hosts-online', value: '1 / 1', source: 'Beszel', status: 'ok', sub: '192.168.178.90 · Uptime 14d 6h', details: ['Docker-Host (UP): CPU 24% · RAM 52% · Disk 58%'] },
      { id: 'containers-healthy', value: '14 / 14', source: 'Docker Engine', status: 'ok', sub: '14 laufende Container · 0 unhealthy', details: ['Healthy / Laufend: 14', 'Unhealthy: 0', 'Neustarts: 0', 'WUD Updates: 2'] },
      { id: 'ssl-status', value: 'OK', source: 'Live TLS', status: 'ok', sub: 'Min. 68 Tage · info.moritzhammer.org', details: ['✓ info.moritzhammer.org: noch 68 Tage'] }
    ],
    critical_services: [
      { id: 'vaultwarden', name: 'Vaultwarden', category: 'Passwort-Manager', target: 'http://192.168.178.90:8081', online: true, latency: '4 ms', http_code: 200, container_status: 'vaultwarden (Up 14 days)' },
      { id: 'immich', name: 'Immich', category: 'Fotoverwaltung', target: 'http://192.168.178.90:2283', online: true, latency: '8 ms', http_code: 200, container_status: 'immich_server (Up 14 days)' },
      { id: 'paperless-ngx', name: 'Paperless-ngx', category: 'Dokumente', target: 'http://192.168.178.90:8000', online: true, latency: '6 ms', http_code: 302, container_status: 'paperless_webserver (Up 14 days)' },
      { id: 'nginx-proxy-manager', name: 'Nginx Proxy Manager', category: 'Reverse Proxy & SSL', target: 'http://192.168.178.90:81', online: true, latency: '3 ms', http_code: 200, container_status: 'nginx_manager (Up 14 days)' },
      { id: 'uptime-kuma', name: 'Uptime Kuma', category: 'Monitoring', target: 'http://192.168.178.90:3001', online: true, latency: '5 ms', http_code: 200, container_status: 'uptime-kuma (Up 14 days)' },
      { id: 'website', name: 'Website', category: 'info.moritzhammer.org', target: 'http://192.168.178.90:8080', online: true, latency: '2 ms', http_code: 200, container_status: 'website (Up 2 days)' }
    ],
    infrastructure: {
      source: 'Live /proc · 192.168.178.90',
      cpu_history: [18, 22, 20, 26, 24, 21, 28, 25, 23, 24, 22, 24],
      ram_history: [50, 51, 51, 52, 52, 52, 53, 52, 52, 52, 52, 52],
      gauges: [
        { id: 'cpu', value: 24, unit: '%', status: 'ok', state_tag: 'NOMINAL', detail: '8 vCPU · Load 0.84', extra: ['Load 1m / 5m / 15m: 0.84 · 0.92 · 0.88', '8 vCPUs aktiv'] },
        { id: 'ram', value: 52, unit: '%', status: 'ok', state_tag: 'NOMINAL', detail: '16.6 / 32.0 GB belegt', extra: ['Verfügbar: 15.4 GB', 'Buffers & Page-Cache: 5.2 GB'] },
        { id: 'disk', value: 58, unit: '%', status: 'ok', state_tag: 'NOMINAL', detail: '580 GB / 1.0 TB', extra: ['Frei: 420 GB · Mount: /', 'Belegt: 580 GB'] }
      ],
      network: { interface: 'eth0', inbound_str: '18.4 Mbit/s', outbound_str: '6.2 Mbit/s', rx_total_gb: 142.8, tx_total_gb: 64.1, in_history: [10, 14, 18, 15, 22, 19, 17, 18], out_history: [4, 5, 6, 5, 8, 7, 6, 6] }
    },
    container_health: { available: true, source: 'Docker Engine', healthy: 14, unhealthy: 0, total: 14, restarts: 0, top_restarts: [] },
    containers: [
      { name: 'vaultwarden', stack: 'vaultwarden', image: 'vaultwarden/server:latest', state: 'running', status_text: 'Up 14 days (healthy)', health: 'healthy', restarts: 0, ports: [':8081→80/tcp'] },
      { name: 'immich_server', stack: 'immich', image: 'ghcr.io/immich-app/immich-server:release', state: 'running', status_text: 'Up 14 days (healthy)', health: 'healthy', restarts: 0, ports: [':2283→2283/tcp'] },
      { name: 'paperless_webserver', stack: 'paperless', image: 'ghcr.io/paperless-ngx/paperless-ngx:latest', state: 'running', status_text: 'Up 14 days (healthy)', health: 'healthy', restarts: 0, ports: [':8000→8000/tcp'] },
      { name: 'nginx_manager', stack: 'nginx_manager', image: 'jc21/nginx-proxy-manager:latest', state: 'running', status_text: 'Up 14 days', health: 'running', restarts: 0, ports: [':81→81/tcp', ':443→443/tcp'] },
      { name: 'uptime-kuma', stack: 'kuma_monitoring', image: 'louislam/uptime-kuma:1', state: 'running', status_text: 'Up 14 days (healthy)', health: 'healthy', restarts: 0, ports: [':3001→3001/tcp'] },
      { name: 'beszel', stack: 'beszel', image: 'henrygd/beszel:latest', state: 'running', status_text: 'Up 14 days', health: 'running', restarts: 0, ports: [':8099→8090/tcp'] }
    ],
    ssl_certs: [{ domain: 'info.moritzhammer.org', days_left: 68, valid: true, issuer: 'Cloudflare / Let\'s Encrypt', not_after: '27.01.2027' }],
    wud: {
      available: true, total_watched: 14, updates_available: 2,
      updates: [
        { container: 'immich_server', image: 'ghcr.io/immich-app/immich-server', current_tag: 'v1.124.0', new_tag: 'v1.125.2', semver_diff: 'minor', caution_db: true },
        { container: 'filebrowser', image: 'filebrowser/filebrowser', current_tag: 'v2.31.1', new_tag: 'v2.31.2', semver_diff: 'patch', caution_db: false }
      ]
    },
    embeds: {
      beszel_url: 'http://192.168.178.90:8099/system/n9qil9mgcw9fhwm', beszel_online: true,
      kuma_url: 'http://192.168.178.90:3001/status/overview', kuma_online: true
    },
    incidents: [
      { level: 'warn', badge: 'UPDATE', title: 'WUD: 2 Container-Updates verfügbar (immich_server, filebrowser)', meta: 'What\'s Up Docker · Minor & Patch Update', detail: 'Vor dem Immich-Update Proxmox-Snapshot prüfen.', time: 'WUD' },
      { level: 'info', badge: 'INFO', title: 'SSL-Zertifikate gültig (noch 68 Tage)', meta: 'Live TLS · info.moritzhammer.org', detail: 'Zertifikat gültig bis 27.01.2027.', time: 'OK' },
      { level: 'info', badge: 'INFO', title: 'Docker Engine stabil · 14 / 14 Container healthy', meta: 'Host 192.168.178.90 · 0 Neustarts', detail: 'Alle Produktiv-Stacks laufen ohne Fehler.', time: 'LIVE' }
    ]
  };

  var testScenarios = {
    warn: JSON.parse(JSON.stringify(demoData)),
    crit: JSON.parse(JSON.stringify(demoData))
  };
  testScenarios.warn.verdict = { level: 'warn', text: 'WARNUNG AKTIV · RESSOURCENLAST & SSL-LAUFZEIT PRÜFEN' };
  testScenarios.warn.executive_summary[3] = { id: 'ssl-status', value: '14 TAGE', source: 'Live TLS', status: 'warn', sub: 'info.moritzhammer.org läuft in 14d ab', details: ['info.moritzhammer.org: noch 14 Tage'] };
  testScenarios.warn.infrastructure.gauges[0] = { id: 'cpu', value: 78, unit: '%', status: 'warn', state_tag: 'WARNUNG (70–85%)', detail: '8 vCPU · Load 5.40', extra: ['Immich ML-Indexing aktiv'] };
  testScenarios.warn.infrastructure.gauges[1] = { id: 'ram', value: 83, unit: '%', status: 'warn', state_tag: 'WARNUNG (75–90%)', detail: '26.5 / 32.0 GB belegt', extra: ['Verfügbar: 5.5 GB'] };

  testScenarios.crit.verdict = { level: 'crit', text: 'EINGRIFF ERFORDERLICH · IMMICH & PAPERLESS-NGX OFFLINE' };
  testScenarios.crit.executive_summary[0] = { id: 'services-online', value: '4 / 6', source: 'Uptime Kuma', status: 'crit', sub: '2 kritische Dienste OFFLINE', details: ['✕ Immich (HTTP 502)', '✕ Paperless-ngx (TIMEOUT)'] };
  testScenarios.crit.executive_summary[2] = { id: 'containers-healthy', value: '12 / 14', source: 'Docker Engine', status: 'crit', sub: '2 Container UNHEALTHY', details: ['Unhealthy: immich_server, paperless_webserver'] };
  testScenarios.crit.critical_services[1].online = false; testScenarios.crit.critical_services[1].latency = 'HTTP 502';
  testScenarios.crit.critical_services[2].online = false; testScenarios.crit.critical_services[2].latency = 'TIMEOUT';
  testScenarios.crit.infrastructure.gauges[0] = { id: 'cpu', value: 92, unit: '%', status: 'crit', state_tag: 'KRITISCH (>85%)', detail: '8 vCPU · Load 9.85', extra: ['Kritische Auslastung'] };
  testScenarios.crit.container_health = { available: true, source: 'Docker Engine', healthy: 12, unhealthy: 2, total: 14, restarts: 5, top_restarts: [{ name: 'immich_server', count: 3, reason: 'Healthcheck fehlgeschlagen' }, { name: 'paperless_webserver', count: 2, reason: 'OOMKilled' }] };
  testScenarios.crit.incidents = [
    { level: 'crit', badge: 'KRITISCH', title: 'Immich OFFLINE (HTTP 502)', meta: 'http://192.168.178.90:2283', detail: 'Container immich_server unhealthy · Logs in Dozzle prüfen.', time: 'AKTIV' },
    { level: 'crit', badge: 'KRITISCH', title: 'Paperless-ngx OFFLINE (TIMEOUT)', meta: 'http://192.168.178.90:8000', detail: 'Container paperless_webserver OOMKilled.', time: 'AKTIV' },
    { level: 'crit', badge: 'KRITISCH', title: 'CPU-Last kritisch (92 %)', meta: 'Host 192.168.178.90 · Load 9.85', detail: 'Sofort Prozesse in Beszel prüfen.', time: 'AKTIV' }
  ];

  var liveBtn = document.getElementById('btn-mode-live');

  async function fetchLive() {
    if (state.simMode !== 'live') return;
    try {
      var r = await fetch('/api/status', { cache: 'no-store' });
      if (!r.ok) throw new Error('HTTP ' + r.status);
      var data = await r.json();
      state.liveConnected = true;
      state.countdown = state.pollEverySec;
      if (liveBtn) liveBtn.textContent = '● Live';
      renderPayload(data);
    } catch (e) {
      state.liveConnected = false;
      if (liveBtn) liveBtn.textContent = '● Demo';
      if (!state.lastPayload) {
        renderPayload(demoData);
      }
    }
  }

  document.querySelectorAll('[data-sim]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var m = btn.getAttribute('data-sim');
      state.simMode = m;
      document.querySelectorAll('[data-sim]').forEach(function (b) {
        b.setAttribute('aria-pressed', String(b === btn));
      });
      if (m === 'live') {
        fetchLive();
      } else if (testScenarios[m]) {
        renderPayload(testScenarios[m]);
      }
    });
  });

  fetchLive();
  setInterval(fetchLive, state.pollEverySec * 1000);
})();
