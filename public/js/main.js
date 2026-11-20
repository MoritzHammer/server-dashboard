// ─── UTILITIES ───
function esc(str) {
  return String(str == null ? '' : str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ─── 1. LIVE NOC CLOCK & REFRESH COUNTDOWN ───
var nocState = {
  mode: 'live',      // 'live' | 'warn' | 'crit'
  liveConnected: false,
  pollEverySec: 5,
  countdown: 5
};

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
    if (nocState.mode === 'live') {
      nocState.countdown = nocState.countdown <= 1 ? nocState.pollEverySec : nocState.countdown - 1;
      if (pulseEl) {
        pulseEl.textContent = (nocState.liveConnected ? 'LIVE ' : 'DEMO ') + nocState.countdown + 'S';
      }
    } else if (pulseEl) {
      pulseEl.textContent = 'TEST-MODUS';
    }
  }

  updateClock();
  setInterval(updateClock, 1000);
})();

// ─── 2. VIEW SWITCHER: NOC KIOSK (100DVH) vs. GRAFANA BLUEPRINT ───
(function () {
  var btnNoc = document.getElementById('btn-view-noc');
  var btnBp  = document.getElementById('btn-view-bp');
  if (!btnNoc || !btnBp) return;

  function setView(mode) {
    var isBp = mode === 'blueprint';
    document.body.classList.toggle('view-noc', !isBp);
    document.body.classList.toggle('view-blueprint', isBp);
    btnNoc.setAttribute('aria-pressed', String(!isBp));
    btnBp.setAttribute('aria-pressed', String(isBp));
    window.scrollTo({ top: 0, behavior: 'instant' });
  }

  btnNoc.addEventListener('click', function () { setView('noc'); });
  btnBp.addEventListener('click', function () { setView('blueprint'); });
})();

// ─── 3. FULLSCREEN & ANDROID TABLET SCREEN WAKE LOCK ───
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

// ─── 4. LIVE API RENDERER + OPTIONAL ALARM TEST SCENARIOS ───
(function () {
  var verdictEl      = document.getElementById('global-verdict');
  var verdictTextEl  = document.getElementById('global-verdict-text');
  var hostSubEl      = document.getElementById('topbar-host-sub');
  var row1SrcEl      = document.getElementById('row1-source');
  var row3SrcEl      = document.getElementById('row3-source');
  var row4SrcEl      = document.getElementById('row4-source');
  var matrixEl       = document.getElementById('services-matrix');
  var netIfaceEl     = document.getElementById('net-iface-label');
  var netInEl        = document.getElementById('net-in-val');
  var netOutEl       = document.getElementById('net-out-val');
  var sparkInEl      = document.getElementById('spark-in');
  var sparkOutEl     = document.getElementById('spark-out');
  var restartListEl  = document.getElementById('restart-list');
  var incidentsListEl = document.getElementById('incidents-list');
  var modeLiveBtn    = document.getElementById('btn-mode-live');
  var simBtns        = [].slice.call(document.querySelectorAll('[data-sim]'));

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

  function renderPayload(data) {
    if (!data) return;

    // Topbar Host & Verdict
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

    // Zeile 2: Kritische Dienste (Strict Binary Green = Online, Red = Offline)
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
      if (row3SrcEl && data.infrastructure.source) {
        row3SrcEl.textContent = data.infrastructure.source;
      }
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
        if (sparkInEl && net.in_history) {
          sparkInEl.setAttribute('points', buildSparkPoints(net.in_history, maxRate));
        }
        if (sparkOutEl && net.out_history) {
          sparkOutEl.setAttribute('points', buildSparkPoints(net.out_history, maxRate));
        }
      }
    }

    // Zeile 4: Container Health (Live Docker Engine)
    if (data.container_health) {
      var ch = data.container_health;
      if (row4SrcEl && ch.source) row4SrcEl.textContent = 'Quelle: ' + ch.source;

      var cHealthy   = document.getElementById('cnt-healthy');
      var cUnhealthy = document.getElementById('cnt-unhealthy');
      var cRestarts  = document.getElementById('cnt-restarts');
      var cSub       = document.getElementById('cnt-healthy-sub');

      if (cHealthy) {
        cHealthy.className = 'cnt-stat' + (!ch.available ? ' status-warn' : (ch.unhealthy > 0 ? ' status-warn' : ''));
        cHealthy.querySelector('.cnt-num').textContent = ch.available ? String(ch.healthy) : 'n/a';
        if (cSub) cSub.textContent = ch.available ? ('von ' + ch.total + ' überwachten Containern') : 'Docker Socket fehlt';
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
        if (!ch.available) {
          restartListEl.innerHTML = '<li class="restart-item"><div class="restart-reason">Docker Socket (/var/run/docker.sock) nicht gemountet</div></li>';
        } else if (!ch.top_restarts || ch.top_restarts.length === 0) {
          restartListEl.innerHTML = '<li class="restart-empty"><span>✓ Alle Container ohne Neustarts</span><span>0 Restarts</span></li>';
        } else {
          restartListEl.innerHTML = ch.top_restarts.map(function (r) {
            return (
              '<li class="restart-item">' +
                '<div>' +
                  '<div class="restart-name">' + esc(r.name) + '</div>' +
                  '<div class="restart-reason">' + esc(r.reason) + '</div>' +
                '</div>' +
                '<span class="restart-count">' + esc(r.count) + '</span>' +
              '</li>'
            );
          }).join('');
        }
      }
    }

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

  // Test-Alarm Scenarios (only active when user clicks Test-Warn or Test-Krit)
  var testScenarios = {
    warn: {
      hostname: 'homelab-test',
      uptime_human: 'Simulation',
      sources: ['Alarm-Test (Warnung)'],
      verdict: { level: 'warn', text: 'WARNUNG AKTIV · RESSOURCENLAST & SSL-LAUFZEIT PRÜFEN' },
      executive_summary: [
        { id: 'services-online', value: '18 / 18', source: 'Uptime Kuma', status: 'ok', sub: 'Alle Dienste erreichbar' },
        { id: 'hosts-online', value: '5 / 5', source: 'Beszel', status: 'ok', sub: 'Hohe Auslastung auf Host 01' },
        { id: 'containers-healthy', value: '42 / 42', source: 'Docker Engine', status: 'ok', sub: 'Erhöhte Restart-Rate' },
        { id: 'ssl-status', value: '14 TAGE', source: 'Live TLS', status: 'warn', sub: 'Zertifikat läuft in 14 Tagen ab' }
      ],
      critical_services: [
        { id: 'vaultwarden', name: 'Vaultwarden', category: 'Passwort-Tresor', online: true, latency: '4 ms' },
        { id: 'immich', name: 'Immich', category: 'Foto-Cloud', online: true, latency: '18 ms' },
        { id: 'paperless', name: 'Paperless', category: 'Dokumente', online: true, latency: '9 ms' },
        { id: 'nginx', name: 'Nginx Proxy', category: 'Edge Proxy & TLS', online: true, latency: '3 ms' },
        { id: 'kuma', name: 'Uptime Kuma', category: 'Monitoring', online: true, latency: '5 ms' },
        { id: 'wireguard', name: 'WireGuard', category: 'VPN Tunnel', online: true, latency: '11 ms' }
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
        available: true, source: 'Test-Simulation', healthy: 42, unhealthy: 0, total: 42, restarts: 4,
        top_restarts: [
          { name: 'jellyfin', count: 3, reason: 'Transcoding Memory Limit' },
          { name: 'immich_ml', count: 1, reason: 'Nightly Worker Reload' }
        ]
      },
      incidents: [
        { level: 'warn', badge: 'WARNUNG', title: 'SSL läuft in 14 Tagen ab', meta: 'Live TLS · Let\'s Encrypt Auto-Renewal prüfen', time: '14d Rest' },
        { level: 'warn', badge: 'WARNUNG', title: 'CPU & RAM über Warnschwelle (78 % / 83 %)', meta: 'Host-Telemetrie · Erhöhte Last erkannt', time: 'AKTIV' },
        { level: 'warn', badge: 'WARNUNG', title: 'Datenträger bei 85 % Belegung', meta: 'Root-FS · Warnschwelle 80 % überschritten', time: 'AKTIV' },
        { level: 'warn', badge: 'WARNUNG', title: 'jellyfin restart x3', meta: 'Docker Engine · Transcoding Memory Limit', time: 'Container' }
      ]
    },
    crit: {
      hostname: 'homelab-test',
      uptime_human: 'Simulation',
      sources: ['Alarm-Test (Kritisch)'],
      verdict: { level: 'crit', text: 'EINGRIFF ERFORDERLICH · KRITISCHER DIENST & HOST OFFLINE' },
      executive_summary: [
        { id: 'services-online', value: '17 / 18', source: 'Uptime Kuma', status: 'crit', sub: '1 kritischer Dienst OFFLINE (Immich)' },
        { id: 'hosts-online', value: '4 / 5', source: 'Beszel', status: 'crit', sub: '1 Host OFFLINE (docker-node-02)' },
        { id: 'containers-healthy', value: '40 / 42', source: 'Docker Engine', status: 'crit', sub: '2 Container UNHEALTHY' },
        { id: 'ssl-status', value: '5 TAGE', source: 'Live TLS', status: 'crit', sub: 'Zertifikat läuft in 5 Tagen ab' }
      ],
      critical_services: [
        { id: 'vaultwarden', name: 'Vaultwarden', category: 'Passwort-Tresor', online: true, latency: '4 ms' },
        { id: 'immich', name: 'Immich', category: 'Foto-Cloud', online: false, latency: 'HTTP 502' },
        { id: 'paperless', name: 'Paperless', category: 'Dokumente', online: true, latency: '9 ms' },
        { id: 'nginx', name: 'Nginx Proxy', category: 'Edge Proxy & TLS', online: true, latency: '3 ms' },
        { id: 'kuma', name: 'Uptime Kuma', category: 'Monitoring', online: true, latency: '5 ms' },
        { id: 'wireguard', name: 'WireGuard', category: 'VPN Tunnel', online: false, latency: 'TIMEOUT' }
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
        available: true, source: 'Test-Simulation', healthy: 40, unhealthy: 2, total: 42, restarts: 9,
        top_restarts: [
          { name: 'immich_server', count: 5, reason: 'Healthcheck fehlgeschlagen' },
          { name: 'jellyfin', count: 3, reason: 'OOMKilled' }
        ]
      },
      incidents: [
        { level: 'crit', badge: 'KRITISCH', title: 'Immich OFFLINE (HTTP 502)', meta: 'Ziel: 192.168.178.90:2283 · Container unhealthy', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'Host offline · docker-node-02 nicht erreichbar', meta: 'Beszel · Heartbeat Timeout', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'Container unhealthy: immich_server', meta: 'Docker Engine · Exit-Code 137 (OOMKilled)', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'SSL läuft in 5 Tagen ab', meta: 'Live TLS · Sofortige Erneuerung erforderlich', time: '5d Rest' },
        { level: 'warn', badge: 'WARNUNG', title: 'jellyfin restart x3', meta: 'Docker Engine · Neustarts registriert', time: 'Container' }
      ]
    }
  };

  async function fetchLiveStatus() {
    if (nocState.mode !== 'live') return;
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
      if (modeLiveBtn) modeLiveBtn.textContent = '● Demo (Offline)';
      if (verdictEl && verdictTextEl && verdictTextEl.textContent.indexOf('VERBINDE') !== -1) {
        verdictEl.className = 'global-verdict';
        verdictTextEl.textContent = 'ALLE SYSTEME NOMINAL · VORSCHAU-MODUS (LIVE AKTIV IM DOCKER-STACK)';
      }
    }
  }

  simBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      var mode = btn.getAttribute('data-sim');
      nocState.mode = mode;
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

  // Start immediate live polling every 5 seconds
  fetchLiveStatus();
  setInterval(fetchLiveStatus, nocState.pollEverySec * 1000);
})();

// ─── 5. COPY PROMQL BUTTONS IN BLUEPRINT VIEW ───
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
