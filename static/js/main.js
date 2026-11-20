// ─── UTILITIES ───
function esc(str) {
  return String(str == null ? '' : str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ─── GLOBAL STATE ───
var nocState = {
  simMode: 'live',       // 'live' | 'warn' | 'crit'
  layoutMode: 'tablet',  // 'tablet' | 'desktop'
  activeTab: 'noc',      // 'noc' | 'windows' | 'wud' | 'blueprint'
  liveConnected: false,
  pollEverySec: 5,
  countdown: 5,
  loadedBeszelURL: '',
  loadedKumaURL: ''
};

// ─── 1. LIVE NOC CLOCK & REFRESH COUNTDOWN ───
(function () {
  var clockEl = document.getElementById('noc-clock');
  var pulseEl = document.getElementById('noc-refresh-label');

  function updateClock() {
    var now = new Date();
    if (clockEl) {
      clockEl.textContent = now.toLocaleTimeString('de-DE', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      });
    }
    if (nocState.simMode === 'live') {
      nocState.countdown = nocState.countdown <= 1 ? nocState.pollEverySec : nocState.countdown - 1;
      if (pulseEl) {
        pulseEl.textContent = (nocState.liveConnected ? 'LIVE ' : 'DEMO ') + nocState.countdown + 'S';
      }
    } else if (pulseEl) {
      pulseEl.textContent = 'TEST';
    }
  }

  updateClock();
  setInterval(updateClock, 1000);
})();

// ─── 2. TABLET vs. DESKTOP LAYOUT SWITCHER & TABS ───
var setTabView;

(function () {
  var layoutBtns = [].slice.call(document.querySelectorAll('[data-layout]'));
  var tabBtns    = [].slice.call(document.querySelectorAll('[data-tab]'));
  var embedBtns  = [].slice.call(document.querySelectorAll('[data-embed-layout]'));
  var embedGrid  = document.getElementById('embed-grid');

  function applyLayoutMode(mode, persist) {
    nocState.layoutMode = mode === 'desktop' ? 'desktop' : 'tablet';
    document.body.classList.toggle('mode-tablet', nocState.layoutMode === 'tablet');
    document.body.classList.toggle('mode-desktop', nocState.layoutMode === 'desktop');
    layoutBtns.forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.getAttribute('data-layout') === nocState.layoutMode));
    });
    if (persist) {
      try { localStorage.setItem('noc_layout_mode', nocState.layoutMode); } catch (e) {}
    }
    // Load iframes lazily once Desktop mode or Windows tab makes them visible
    if (nocState.layoutMode === 'desktop') {
      ensureEmbedsLoaded();
    }
  }

  setTabView = function (tab) {
    nocState.activeTab = tab;
    ['tab-noc', 'tab-windows', 'tab-wud', 'tab-blueprint'].forEach(function (cls) {
      document.body.classList.toggle(cls, cls === 'tab-' + tab);
    });
    tabBtns.forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.getAttribute('data-tab') === tab));
    });

    if (tab === 'windows' || nocState.layoutMode === 'desktop') {
      ensureEmbedsLoaded();
    }

    if (nocState.layoutMode === 'desktop' && tab !== 'blueprint') {
      var targetId = tab === 'windows' ? 'live-windows-section' : (tab === 'wud' ? 'wud-section' : 'noc-monitor-view');
      var el = document.getElementById(targetId);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } else {
      window.scrollTo({ top: 0, behavior: 'instant' });
    }
  };

  // Restore saved Tablet/Desktop preference or auto-detect based on device
  var savedMode = null;
  try { savedMode = localStorage.getItem('noc_layout_mode'); } catch (e) {}
  if (savedMode === 'tablet' || savedMode === 'desktop') {
    applyLayoutMode(savedMode, false);
  } else {
    var isDesktop = window.innerWidth >= 1280 && !window.matchMedia('(pointer: coarse)').matches;
    applyLayoutMode(isDesktop ? 'desktop' : 'tablet', false);
  }

  layoutBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyLayoutMode(btn.getAttribute('data-layout'), true);
    });
  });

  tabBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      setTabView(btn.getAttribute('data-tab'));
    });
  });

  // Row 4 WUD stat card jumps directly to WUD updates
  var wudStatCard = document.getElementById('cnt-wud');
  if (wudStatCard) {
    wudStatCard.addEventListener('click', function () {
      setTabView('wud');
    });
  }

  // Beszel / Kuma Split vs. Single window layout buttons
  embedBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      var l = btn.getAttribute('data-embed-layout');
      embedBtns.forEach(function (b) {
        b.setAttribute('aria-pressed', String(b === btn));
      });
      if (embedGrid) {
        embedGrid.className = 'embed-grid layout-' + l;
      }
    });
  });

  // Reload iframe buttons
  var reloadBeszel = document.getElementById('reload-beszel');
  var reloadKuma   = document.getElementById('reload-kuma');
  if (reloadBeszel) {
    reloadBeszel.addEventListener('click', function () {
      var fr = document.getElementById('iframe-beszel');
      if (fr && nocState.loadedBeszelURL) fr.src = nocState.loadedBeszelURL;
    });
  }
  if (reloadKuma) {
    reloadKuma.addEventListener('click', function () {
      var fr = document.getElementById('iframe-kuma');
      if (fr && nocState.loadedKumaURL) fr.src = nocState.loadedKumaURL;
    });
  }
})();

// ─── 3. EMBEDDED WINDOWS HELPER (NEVER FLICKERS ON POLL) ───
var desiredEmbeds = {
  beszelURL: 'http://192.168.178.90:8090',
  kumaURL:   'http://192.168.178.90:3001'
};

function ensureEmbedsLoaded() {
  var frB = document.getElementById('iframe-beszel');
  var frK = document.getElementById('iframe-kuma');
  if (frB && desiredEmbeds.beszelURL && nocState.loadedBeszelURL !== desiredEmbeds.beszelURL) {
    nocState.loadedBeszelURL = desiredEmbeds.beszelURL;
    frB.src = desiredEmbeds.beszelURL;
  }
  if (frK && desiredEmbeds.kumaURL && nocState.loadedKumaURL !== desiredEmbeds.kumaURL) {
    nocState.loadedKumaURL = desiredEmbeds.kumaURL;
    frK.src = desiredEmbeds.kumaURL;
  }
}

// ─── 4. FULLSCREEN & ANDROID TABLET SCREEN WAKE LOCK ───
(function () {
  var fsBtn = document.getElementById('btn-kiosk-fs');

  async function requestWakeLock() {
    try {
      if ('wakeLock' in navigator) {
        await navigator.wakeLock.request('screen');
      }
    } catch (e) {}
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

// ─── 5. LIVE API RENDERER + ALARM TESTS (ONLY MORITZ' REAL SERVICES) ───
(function () {
  var verdictEl       = document.getElementById('global-verdict');
  var verdictTextEl   = document.getElementById('global-verdict-text');
  var hostSubEl       = document.getElementById('topbar-host-sub');
  var row1SrcEl       = document.getElementById('row1-source');
  var row3SrcEl       = document.getElementById('row3-source');
  var row4SrcEl       = document.getElementById('row4-source');
  var matrixEl        = document.getElementById('services-matrix');
  var netIfaceEl      = document.getElementById('net-iface-label');
  var netInEl         = document.getElementById('net-in-val');
  var netOutEl        = document.getElementById('net-out-val');
  var sparkInEl       = document.getElementById('spark-in');
  var sparkOutEl      = document.getElementById('spark-out');
  var restartListEl   = document.getElementById('restart-list');
  var incidentsListEl = document.getElementById('incidents-list');
  var modeLiveBtn     = document.getElementById('btn-mode-live');
  var simBtns         = [].slice.call(document.querySelectorAll('[data-sim]'));

  var wudStatCard     = document.getElementById('cnt-wud');
  var wudStatNum      = document.getElementById('cnt-wud-num');
  var wudStatSub      = document.getElementById('cnt-wud-sub');
  var wudTabBtn       = document.getElementById('tab-btn-wud');
  var wudGridEl       = document.getElementById('wud-updates-grid');
  var wudSummaryEl    = document.getElementById('wud-status-summary');
  var openWudEl       = document.getElementById('open-wud');
  var beszelHostsBar  = document.getElementById('beszel-hosts-bar');

  function buildSparkPoints(arr, maxVal) {
    if (!arr || arr.length < 2) return '0,22 240,22';
    var n = arr.length;
    var cap = Math.max(maxVal, 1);
    return arr.map(function (v, idx) {
      var x = Math.round((idx / (n - 1)) * 240);
      var y = Math.round(24 - Math.min(1, Math.max(0, v / cap)) * 20);
      return x + ',' + y;
    }).join(' ');
  }

  function renderWUD(wud) {
    if (!wud) return;
    if (openWudEl) {
      if (wud.url) {
        openWudEl.href = wud.url;
        openWudEl.hidden = false;
      } else {
        openWudEl.hidden = true;
      }
    }

    if (wudStatCard && wudStatNum && wudStatSub) {
      if (!wud.available) {
        wudStatCard.className = 'cnt-stat cnt-stat--clickable';
        wudStatNum.textContent = '–';
        wudStatSub.textContent = wud.error || 'WUD_URL setzen';
      } else if (wud.updates_available > 0) {
        wudStatCard.className = 'cnt-stat cnt-stat--clickable status-warn';
        wudStatNum.textContent = String(wud.updates_available);
        wudStatSub.textContent = 'von ' + wud.total_watched + ' Containern';
      } else {
        wudStatCard.className = 'cnt-stat cnt-stat--clickable';
        wudStatNum.textContent = '0';
        wudStatSub.textContent = wud.total_watched + ' Container aktuell';
      }
    }

    if (wudTabBtn) {
      wudTabBtn.textContent = (wud.available && wud.updates_available > 0)
        ? ('WUD (' + wud.updates_available + ')')
        : 'WUD';
    }

    if (wudSummaryEl) {
      if (!wud.available) {
        wudSummaryEl.textContent = wud.error || 'WUD nicht verbunden (ENV WUD_URL in Portainer setzen)';
      } else {
        wudSummaryEl.textContent = wud.updates_available + ' Update(s) verfügbar · ' + wud.total_watched + ' überwachte Container';
      }
    }

    if (wudGridEl) {
      if (!wud.available) {
        wudGridEl.innerHTML =
          '<div class="wud-empty-card is-warn">' +
            '<span>WUD API aktuell nicht erreichbar (' + esc(wud.error || 'kein WUD-Port erkannt') + '). Setze in Portainer einfach <code>WUD_URL=http://192.168.178.90:&lt;WUD-PORT&gt;</code> (ggf. + <code>WUD_USER</code> / <code>WUD_PASS</code>).</span>' +
          '</div>';
      } else if (!wud.updates || wud.updates.length === 0) {
        wudGridEl.innerHTML =
          '<div class="wud-empty-card">' +
            '<span>✓ Alle ' + esc(wud.total_watched) + ' überwachten Container laufen auf dem neuesten Image-Stand.</span>' +
          '</div>';
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

  function renderEmbedsAndBeszel(embeds, beszel) {
    if (embeds) {
      if (embeds.beszel_url) {
        desiredEmbeds.beszelURL = embeds.beszel_url;
        var lblB = document.getElementById('url-label-beszel');
        var opnB = document.getElementById('open-beszel');
        if (lblB) lblB.textContent = embeds.beszel_url;
        if (opnB) opnB.href = embeds.beszel_url;
      }
      if (embeds.kuma_url) {
        desiredEmbeds.kumaURL = embeds.kuma_url;
        var lblK = document.getElementById('url-label-kuma');
        var opnK = document.getElementById('open-kuma');
        if (lblK) lblK.textContent = embeds.kuma_url;
        if (opnK) opnK.href = embeds.kuma_url;
      }
      var dotB = document.getElementById('dot-beszel');
      var dotK = document.getElementById('dot-kuma');
      if (dotB) dotB.className = 'win-dot ' + (embeds.beszel_online ? 'is-up' : 'is-down');
      if (dotK) dotK.className = 'win-dot ' + (embeds.kuma_online ? 'is-up' : 'is-down');

      if (nocState.layoutMode === 'desktop' || nocState.activeTab === 'windows') {
        ensureEmbedsLoaded();
      }
    }

    if (beszelHostsBar && beszel && beszel.available && Array.isArray(beszel.hosts) && beszel.hosts.length > 0) {
      beszelHostsBar.hidden = false;
      beszelHostsBar.innerHTML = beszel.hosts.map(function (h) {
        var down = h.status !== 'up';
        return (
          '<div class="beszel-host-chip' + (down ? ' is-down' : '') + '">' +
            '<strong>' + esc(h.name) + ' (' + esc(h.status.toUpperCase()) + ')</strong>' +
            '<span class="beszel-host-stats">CPU ' + esc(h.cpu) + '% · RAM ' + esc(h.mem) + '% · Disk ' + esc(h.disk) + '%' + (h.temp > 0 ? (' · ' + esc(h.temp) + '°C') : '') + '</span>' +
          '</div>'
        );
      }).join('');
    } else if (beszelHostsBar) {
      beszelHostsBar.hidden = true;
    }
  }

  function renderPayload(data) {
    if (!data) return;

    if (hostSubEl && data.hostname) {
      hostSubEl.textContent = data.hostname + ' · Uptime ' + (data.uptime_human || 'aktiv');
    }
    if (verdictEl && verdictTextEl && data.verdict) {
      var vCls = data.verdict.level === 'crit' ? ' is-crit' : (data.verdict.level === 'warn' ? ' is-warn' : '');
      verdictEl.className = 'global-verdict' + vCls;
      verdictTextEl.textContent = data.verdict.text;
    }
    if (row1SrcEl && data.sources && data.sources.length) {
      row1SrcEl.textContent = 'Live-Quellen: ' + data.sources.join(' · ');
    }

    // Zeile 1: Executive Summary
    if (Array.isArray(data.executive_summary)) {
      data.executive_summary.forEach(function (item) {
        var card = document.querySelector('[data-exec-id="' + item.id + '"]');
        if (!card) return;
        card.className = 'stat-card' + (item.status && item.status !== 'ok' ? ' status-' + item.status : '');
        var valEl = card.querySelector('.stat-value');
        var srcEl = card.querySelector('.stat-src-chip');
        var subEl = card.querySelector('.stat-sub');
        if (valEl) valEl.textContent = item.value;
        if (srcEl) srcEl.textContent = item.source;
        if (subEl) subEl.textContent = item.sub;
      });
    }

    // Zeile 2: Kritische Dienste (Strictly binary Green = Online, Red = Offline)
    if (matrixEl && Array.isArray(data.critical_services) && data.critical_services.length > 0) {
      var cols = Math.min(Math.max(data.critical_services.length, 3), 6);
      matrixEl.style.setProperty('--svc-cols', String(cols));
      matrixEl.innerHTML = data.critical_services.map(function (svc) {
        var offCls = svc.online ? '' : ' is-offline';
        var badge = svc.online ? '✅ ONLINE' : '🚨 OFFLINE';
        var tip = svc.target ? ('Ziel: ' + svc.target + (svc.error ? ' (' + svc.error + ')' : '')) : svc.name;
        return (
          '<article class="svc-tile' + offCls + '" data-svc-id="' + esc(svc.id) + '" title="' + esc(tip) + '">' +
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

    // Zeile 3: Infrastruktur Gauges & Live Network Sparkline
    if (data.infrastructure) {
      if (row3SrcEl && data.infrastructure.source) row3SrcEl.textContent = data.infrastructure.source;
      if (Array.isArray(data.infrastructure.gauges)) {
        data.infrastructure.gauges.forEach(function (g) {
          var gCard = document.querySelector('[data-gauge-id="' + g.id + '"]');
          if (!gCard) return;
          gCard.className = 'gauge-card' + (g.status && g.status !== 'ok' ? ' status-' + g.status : '');
          var numEl  = gCard.querySelector('.gauge-num');
          var tagEl  = gCard.querySelector('.gauge-state-tag');
          var detEl  = gCard.querySelector('.gauge-detail');
          var fillEl = gCard.querySelector('.gauge-fill');
          if (numEl)  numEl.textContent = Math.round(g.value) + ' ' + g.unit;
          if (tagEl)  tagEl.textContent = g.state_tag;
          if (detEl)  detEl.textContent = g.detail;
          if (fillEl) fillEl.style.width = Math.min(100, Math.max(0, g.value)) + '%';
        });
      }
      if (data.infrastructure.network) {
        var net = data.infrastructure.network;
        if (netIfaceEl && net.interface) netIfaceEl.textContent = 'NIC: ' + net.interface;
        if (netInEl)  netInEl.textContent = net.inbound_str;
        if (netOutEl) netOutEl.textContent = net.outbound_str;
        var allRates = (net.in_history || []).concat(net.out_history || []);
        var maxRate = allRates.reduce(function (m, x) { return Math.max(m, x); }, 1);
        if (sparkInEl && net.in_history) sparkInEl.setAttribute('points', buildSparkPoints(net.in_history, maxRate));
        if (sparkOutEl && net.out_history) sparkOutEl.setAttribute('points', buildSparkPoints(net.out_history, maxRate));
      }
    }

    // Zeile 4: Container Health + WUD
    if (data.container_health) {
      var ch = data.container_health;
      if (row4SrcEl && ch.source) row4SrcEl.textContent = 'Quellen: ' + ch.source + ' & WUD';

      var cHealthy   = document.getElementById('cnt-healthy');
      var cUnhealthy = document.getElementById('cnt-unhealthy');
      var cRestarts  = document.getElementById('cnt-restarts');
      var cSub       = document.getElementById('cnt-healthy-sub');

      if (cHealthy) {
        cHealthy.className = 'cnt-stat' + (!ch.available ? ' status-warn' : (ch.unhealthy > 0 ? ' status-warn' : ''));
        cHealthy.querySelector('.cnt-num').textContent = ch.available ? String(ch.healthy) : 'n/a';
        if (cSub) cSub.textContent = ch.available ? ('von ' + ch.total + ' Containern') : 'Docker Socket fehlt';
      }
      if (cUnhealthy) {
        cUnhealthy.className = 'cnt-stat' + (ch.unhealthy > 0 ? ' status-crit' : '');
        cUnhealthy.querySelector('.cnt-num').textContent = ch.available ? String(ch.unhealthy) : 'n/a';
      }
      if (cRestarts) {
        cRestarts.className = 'cnt-stat' + (ch.restarts >= 5 ? ' status-crit' : (ch.restarts > 0 ? ' status-warn' : ''));
        cRestarts.querySelector('.cnt-num').textContent = ch.available ? String(ch.restarts) : 'n/a';
      }

      if (restartListEl) {
        var hasRestarts = ch.available && ch.top_restarts && ch.top_restarts.length > 0;
        var hasWudUpdates = data.wud && data.wud.available && data.wud.updates && data.wud.updates.length > 0;
        if (!ch.available) {
          restartListEl.innerHTML = '<li class="restart-item"><div class="restart-reason">Docker Socket (/var/run/docker.sock) nicht gemountet</div></li>';
        } else if (hasRestarts) {
          restartListEl.innerHTML = ch.top_restarts.slice(0, 2).map(function (r) {
            return (
              '<li class="restart-item">' +
                '<div><div class="restart-name">' + esc(r.name) + '</div><div class="restart-reason">' + esc(r.reason) + '</div></div>' +
                '<span class="restart-count">' + esc(r.count) + '</span>' +
              '</li>'
            );
          }).join('');
        } else if (hasWudUpdates) {
          restartListEl.innerHTML = data.wud.updates.slice(0, 2).map(function (u) {
            return (
              '<li class="restart-item">' +
                '<div><div class="restart-name">' + esc(u.container) + '</div><div class="restart-reason">' + esc(u.current_tag) + ' → ' + esc(u.new_tag) + '</div></div>' +
                '<span class="restart-count">UPD</span>' +
              '</li>'
            );
          }).join('');
        } else {
          restartListEl.innerHTML = '<li class="restart-empty"><span>✓ Alle Container ohne Neustarts</span><span>0 Restarts</span></li>';
        }
      }
    }

    renderWUD(data.wud);
    renderEmbedsAndBeszel(data.embeds, data.beszel);

    // Zeile 5: Incidents (Max 5)
    if (incidentsListEl && Array.isArray(data.incidents)) {
      incidentsListEl.innerHTML = data.incidents.slice(0, 5).map(function (inc) {
        return (
          '<li class="incident-card level-' + esc(inc.level) + '">' +
            '<div class="inc-left">' +
              '<span class="inc-badge">' + esc(inc.badge) + '</span>' +
              '<div class="inc-text">' +
                '<span class="inc-title">' + esc(inc.title) + '</span>' +
                '<span class="inc-meta">' + esc(inc.meta) + '</span>' +
              '</div>' +
            '</div>' +
            '<span class="inc-time">' + esc(inc.time) + '</span>' +
          '</li>'
        );
      }).join('');
    }
  }

  // Test-Alarm Scenarios (Strictly using only Moritz' real services!)
  var testScenarios = {
    warn: {
      hostname: 'docker-host (Test)',
      uptime_human: 'Simulation',
      sources: ['Alarm-Test (Warnung)'],
      verdict: { level: 'warn', text: 'WARNUNG AKTIV · RESSOURCENLAST, WUD UPDATES & SSL PRÜFEN' },
      executive_summary: [
        { id: 'services-online', value: '6 / 6', source: 'Uptime Kuma', status: 'ok', sub: 'Alle Dienste erreichbar' },
        { id: 'hosts-online', value: '1 / 1', source: 'Beszel', status: 'ok', sub: 'Hohe Last auf 192.168.178.90' },
        { id: 'containers-healthy', value: '14 / 14', source: 'Docker Engine', status: 'ok', sub: '2 WUD Updates verfügbar' },
        { id: 'ssl-status', value: '14 TAGE', source: 'Live TLS', status: 'warn', sub: 'info.moritzhammer.org läuft in 14d ab' }
      ],
      critical_services: [
        { id: 'vaultwarden', name: 'Vaultwarden', category: 'Passwort-Manager', online: true, latency: '4 ms' },
        { id: 'immich', name: 'Immich', category: 'Fotoverwaltung', online: true, latency: '18 ms' },
        { id: 'paperless-ngx', name: 'Paperless-ngx', category: 'Dokumente', online: true, latency: '9 ms' },
        { id: 'nginx-proxy-manager', name: 'Nginx Proxy Manager', category: 'Reverse Proxy & SSL', online: true, latency: '3 ms' },
        { id: 'uptime-kuma', name: 'Uptime Kuma', category: 'Monitoring', online: true, latency: '5 ms' },
        { id: 'website', name: 'Website', category: 'info.moritzhammer.org', online: true, latency: '2 ms' }
      ],
      infrastructure: {
        source: 'Test-Simulation · Warnschwellen',
        gauges: [
          { id: 'cpu', value: 78, unit: '%', status: 'warn', state_tag: 'WARNUNG (70–85%)', detail: '8 vCPU · Load 5.40' },
          { id: 'ram', value: 83, unit: '%', status: 'warn', state_tag: 'WARNUNG (75–90%)', detail: '26.5 / 32.0 GB belegt' },
          { id: 'disk', value: 85, unit: '%', status: 'warn', state_tag: 'WARNUNG (80–90%)', detail: '850 GB / 1.0 TB' }
        ],
        network: { interface: 'eth0', inbound_str: '312 Mbit/s', outbound_str: '94.8 Mbit/s', in_history: [120, 180, 240, 290, 312, 285, 310, 312], out_history: [40, 62, 80, 95, 88, 92, 95, 95] }
      },
      container_health: {
        available: true, source: 'Test-Simulation', healthy: 14, unhealthy: 0, total: 14, restarts: 2,
        top_restarts: [
          { name: 'immich_machine_learning', count: 2, reason: 'Speicherlimit bei Foto-Scan' }
        ]
      },
      wud: {
        available: true, total_watched: 14, updates_available: 2,
        updates: [
          { container: 'immich_server', image: 'ghcr.io/immich-app/immich-server', current_tag: 'v1.124.0', new_tag: 'v2.0.0', semver_diff: 'major', caution_db: true },
          { container: 'filebrowser', image: 'filebrowser/filebrowser', current_tag: 'v2.31.1', new_tag: 'v2.31.2', semver_diff: 'patch', caution_db: false }
        ]
      },
      incidents: [
        { level: 'warn', badge: 'WARNUNG', title: 'SSL läuft in 14 Tagen ab · info.moritzhammer.org', meta: 'Live TLS · Zertifikats-Renewal prüfen', time: '14d Rest' },
        { level: 'warn', badge: 'UPDATE',  title: 'WUD: 2 Container-Updates verfügbar (immich_server, filebrowser)', meta: 'Achtung: Major-Update bei Immich (vorher Backup prüfen)', time: 'WUD' },
        { level: 'warn', badge: 'WARNUNG', title: 'CPU & RAM über Warnschwelle (78 % / 83 %)', meta: 'Host 192.168.178.90 · Immich ML-Indexing aktiv', time: 'AKTIV' },
        { level: 'warn', badge: 'WARNUNG', title: 'immich_machine_learning restart x2', meta: 'Docker Engine · Speicherlimit erreicht', time: 'Container' }
      ]
    },
    crit: {
      hostname: 'docker-host (Test)',
      uptime_human: 'Simulation',
      sources: ['Alarm-Test (Kritisch)'],
      verdict: { level: 'crit', text: 'EINGRIFF ERFORDERLICH · IMMICH & PAPERLESS-NGX OFFLINE' },
      executive_summary: [
        { id: 'services-online', value: '4 / 6', source: 'Uptime Kuma', status: 'crit', sub: '2 kritische Dienste OFFLINE' },
        { id: 'hosts-online', value: '1 / 1', source: 'Beszel', status: 'ok', sub: 'Host 192.168.178.90 überlastet' },
        { id: 'containers-healthy', value: '12 / 14', source: 'Docker Engine', status: 'crit', sub: '2 Container UNHEALTHY' },
        { id: 'ssl-status', value: '5 TAGE', source: 'Live TLS', status: 'crit', sub: 'SSL-Zertifikat läuft in 5 Tagen ab' }
      ],
      critical_services: [
        { id: 'vaultwarden', name: 'Vaultwarden', category: 'Passwort-Manager', online: true, latency: '4 ms' },
        { id: 'immich', name: 'Immich', category: 'Fotoverwaltung', online: false, latency: 'HTTP 502' },
        { id: 'paperless-ngx', name: 'Paperless-ngx', category: 'Dokumente', online: false, latency: 'TIMEOUT' },
        { id: 'nginx-proxy-manager', name: 'Nginx Proxy Manager', category: 'Reverse Proxy & SSL', online: true, latency: '3 ms' },
        { id: 'uptime-kuma', name: 'Uptime Kuma', category: 'Monitoring', online: true, latency: '5 ms' },
        { id: 'website', name: 'Website', category: 'info.moritzhammer.org', online: true, latency: '2 ms' }
      ],
      infrastructure: {
        source: 'Test-Simulation · Kritische Schwellen',
        gauges: [
          { id: 'cpu', value: 93, unit: '%', status: 'crit', state_tag: 'KRITISCH (>85%)', detail: '8 vCPU · Load 9.85' },
          { id: 'ram', value: 94, unit: '%', status: 'crit', state_tag: 'KRITISCH (>90%)', detail: '30.1 / 32.0 GB belegt' },
          { id: 'disk', value: 93, unit: '%', status: 'crit', state_tag: 'KRITISCH (>90%)', detail: '930 GB / 1.0 TB' }
        ],
        network: { interface: 'eth0', inbound_str: '842 Mbit/s', outbound_str: '410 Mbit/s', in_history: [400, 520, 690, 780, 842, 810, 842, 842], out_history: [190, 250, 340, 390, 410, 395, 410, 410] }
      },
      container_health: {
        available: true, source: 'Test-Simulation', healthy: 12, unhealthy: 2, total: 14, restarts: 6,
        top_restarts: [
          { name: 'immich_server', count: 4, reason: 'Healthcheck fehlgeschlagen' },
          { name: 'paperless_webserver', count: 2, reason: 'OOMKilled' }
        ]
      },
      wud: {
        available: true, total_watched: 14, updates_available: 1,
        updates: [
          { container: 'paperless_webserver', image: 'ghcr.io/paperless-ngx/paperless-ngx', current_tag: '2.12.1', new_tag: '2.13.5', semver_diff: 'minor', caution_db: true }
        ]
      },
      incidents: [
        { level: 'crit', badge: 'KRITISCH', title: 'Immich OFFLINE (HTTP 502)', meta: 'Ziel: http://192.168.178.90:2283 · Container immich_server unhealthy', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'Paperless-ngx OFFLINE (TIMEOUT)', meta: 'Ziel: http://192.168.178.90:8000 · OOMKilled (Exit 137)', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'Arbeitsspeicher & Datenträger kritisch (94 % / 93 %)', meta: 'Host 192.168.178.90 · Sofort prüfen', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'SSL läuft in 5 Tagen ab · info.moritzhammer.org', meta: 'Live TLS · Sofortige Erneuerung erforderlich', time: '5d Rest' }
      ]
    }
  };

  async function fetchLiveStatus() {
    if (nocState.simMode !== 'live') return;
    try {
      var resp = await fetch('/api/status', { cache: 'no-store' });
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
      var data = await resp.json();
      nocState.liveConnected = true;
      nocState.countdown = nocState.pollEverySec;
      if (modeLiveBtn) modeLiveBtn.textContent = '● Live';
      renderPayload(data);
    } catch (e) {
      nocState.liveConnected = false;
      if (modeLiveBtn) modeLiveBtn.textContent = '● Demo';
      if (verdictEl && verdictTextEl && verdictTextEl.textContent.indexOf('VERBINDE') !== -1) {
        verdictEl.className = 'global-verdict';
        verdictTextEl.textContent = 'ALLE SYSTEME NOMINAL · VORSCHAU (LIVE AKTIV IM DOCKER-STACK)';
      }
      renderWUD({
        available: true,
        total_watched: 14,
        updates_available: 2,
        updates: [
          { container: 'immich_server', image: 'ghcr.io/immich-app/immich-server', current_tag: 'v1.124.0', new_tag: 'v1.125.2', semver_diff: 'minor', caution_db: true },
          { container: 'filebrowser', image: 'filebrowser/filebrowser', current_tag: 'v2.31.1', new_tag: 'v2.31.2', semver_diff: 'patch', caution_db: false }
        ]
      });
    }
  }

  simBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      var mode = btn.getAttribute('data-sim');
      nocState.simMode = mode;
      simBtns.forEach(function (b) {
        b.setAttribute('aria-pressed', String(b.getAttribute('data-sim') === mode));
      });
      if (mode === 'live') {
        fetchLiveStatus();
      } else if (testScenarios[mode]) {
        renderPayload(testScenarios[mode]);
      }
    });
  });

  fetchLiveStatus();
  setInterval(fetchLiveStatus, nocState.pollEverySec * 1000);
})();

// ─── 6. COPY PROMQL BUTTONS IN BLUEPRINT VIEW ───
(function () {
  document.querySelectorAll('[data-copy-target]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var id = btn.getAttribute('data-copy-target');
      var codeEl = document.getElementById(id);
      if (!codeEl || !navigator.clipboard) return;
      navigator.clipboard.writeText(codeEl.textContent.trim()).then(function () {
        var prev = btn.textContent;
        btn.textContent = 'Kopiert ✓';
        setTimeout(function () { btn.textContent = prev; }, 1600);
      });
    });
  });
})();
