// ─── 1. LIVE NOC CLOCK & 10S HEARTBEAT ───
(function () {
  var clockEl = document.getElementById('noc-clock');
  var pulseEl = document.getElementById('noc-refresh-label');
  var sec = 10;

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
    sec = sec <= 1 ? 10 : sec - 1;
    if (pulseEl) {
      pulseEl.textContent = 'SYNC ' + sec + 'S';
    }
  }

  updateClock();
  setInterval(updateClock, 1000);
})();

// ─── 2. VIEW SWITCHER: NOC KIOSK (100VH) vs. GRAFANA BLUEPRINT ───
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
  var wakeLock = null;

  async function requestWakeLock() {
    try {
      if ('wakeLock' in navigator) {
        wakeLock = await navigator.wakeLock.request('screen');
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

// ─── 4. NOC SCENARIO SIMULATOR & INTERACTIVE SERVICE MATRIX ───
(function () {
  var scenarios = {
    ok: {
      verdictClass: '',
      verdictText: 'ALLE SYSTEME NOMINAL · KEIN EINGRIFF ERFORDERLICH',
      exec: {
        'services-online':    { val: '18 / 18', status: 'ok',   sub: '100 % Verfügbarkeit · HTTP/TCP' },
        'hosts-online':       { val: '5 / 5',   status: 'ok',   sub: 'Proxmox Cluster & Docker Nodes' },
        'containers-healthy': { val: '42 / 42', status: 'ok',   sub: 'Docker Healthchecks aktiv' },
        'ssl-status':         { val: 'OK',      status: 'ok',   sub: 'Min. Restlaufzeit > 14 Tage' }
      },
      offlineServices: [],
      gauges: {
        cpu:  { val: 24, status: 'ok',   tag: 'NOMINAL', detail: '8 vCPU · Load 1.12' },
        ram:  { val: 58, status: 'ok',   tag: 'NOMINAL', detail: '18.6 / 32.0 GB belegt' },
        disk: { val: 64, status: 'ok',   tag: 'NOMINAL', detail: '640 GB / 1.0 TB NVMe' }
      },
      net: { inRate: '48.4 Mbit/s', outRate: '14.2 Mbit/s' },
      containers: {
        healthy:   { val: '42', status: 'ok' },
        unhealthy: { val: '0',  status: 'ok' },
        restarts:  { val: '3',  status: 'warn' }
      },
      incidents: [
        { level: 'warn', badge: 'WARNUNG', title: 'Jellyfin restart x3', meta: 'Dazzle · Letzte 24h · Container läuft aktuell stabil', time: 'vor 2 Std.' },
        { level: 'warn', badge: 'WARNUNG', title: 'SSL läuft in 14 Tagen ab', meta: 'Uptime Kuma · Zertifikats-Renewal für Subdomain vorgemerkt', time: '14d Restzeit' },
        { level: 'info', badge: 'INFO',    title: 'Proxmox & Vaultwarden Nightly-Backup erfolgreich', meta: 'Backup-Status-Container · 3-2-1 Snapshot verifiziert (42.8 GB)', time: '03:00 Uhr' },
        { level: 'info', badge: 'INFO',    title: 'WireGuard Handshake & DDNS-Sync aktiv', meta: 'UniFi Gateway · vpn.hammermoritz.de (DNS-only)', time: 'laufend' }
      ]
    },
    warn: {
      verdictClass: 'is-warn',
      verdictText: 'WARNUNG AKTIV · RESSOURCENLAST & SSL-LAUFZEIT PRÜFEN',
      exec: {
        'services-online':    { val: '18 / 18', status: 'ok',   sub: 'Alle Dienste erreichbar' },
        'hosts-online':       { val: '5 / 5',   status: 'ok',   sub: 'Hohe Auslastung auf Host 01' },
        'containers-healthy': { val: '42 / 42', status: 'ok',   sub: 'Erhöhte Restart-Rate (24h)' },
        'ssl-status':         { val: '14 TAGE', status: 'warn', sub: 'Zertifikat läuft in 14 Tagen ab' }
      },
      offlineServices: [],
      gauges: {
        cpu:  { val: 78, status: 'warn', tag: 'WARNUNG (70–85%)', detail: '8 vCPU · Load 5.40' },
        ram:  { val: 83, status: 'warn', tag: 'WARNUNG (75–90%)', detail: '26.5 / 32.0 GB belegt' },
        disk: { val: 85, status: 'warn', tag: 'WARNUNG (80–90%)', detail: '850 GB / 1.0 TB NVMe' }
      },
      net: { inRate: '312.0 Mbit/s', outRate: '94.8 Mbit/s' },
      containers: {
        healthy:   { val: '42', status: 'ok' },
        unhealthy: { val: '0',  status: 'ok' },
        restarts:  { val: '5',  status: 'warn' }
      },
      incidents: [
        { level: 'warn', badge: 'WARNUNG', title: 'SSL läuft in 14 Tagen ab', meta: 'Uptime Kuma · Let\'s Encrypt Renewal prüfen', time: '14d Restzeit' },
        { level: 'warn', badge: 'WARNUNG', title: 'CPU & RAM über Warnschwelle (78 % / 83 %)', meta: 'Beszel · Immich ML-Indexing aktiv auf 192.168.178.90', time: 'seit 12 Min.' },
        { level: 'warn', badge: 'WARNUNG', title: 'Datenträger NVMe bei 85 % Belegung', meta: 'Node Exporter · Warnschwelle 80 % überschritten', time: 'heute' },
        { level: 'warn', badge: 'WARNUNG', title: 'Jellyfin restart x3', meta: 'Dazzle · Letzte 24h · Transcoding Memory Limit', time: 'vor 2 Std.' }
      ]
    },
    crit: {
      verdictClass: 'is-crit',
      verdictText: 'EINGRIFF ERFORDERLICH · KRITISCHER DIENST / HOST AUSGEFALLEN',
      exec: {
        'services-online':    { val: '17 / 18', status: 'crit', sub: '1 kritischer Dienst OFFLINE (Immich)' },
        'hosts-online':       { val: '4 / 5',   status: 'crit', sub: '1 Host OFFLINE (docker-node-02)' },
        'containers-healthy': { val: '40 / 42', status: 'crit', sub: '2 Container UNHEALTHY' },
        'ssl-status':         { val: 'WARN',    status: 'warn', sub: 'SSL läuft in 14 Tagen ab' }
      },
      offlineServices: ['immich'],
      gauges: {
        cpu:  { val: 92, status: 'crit', tag: 'KRITISCH (>85%)', detail: '8 vCPU · Load 9.85' },
        ram:  { val: 94, status: 'crit', tag: 'KRITISCH (>90%)', detail: '30.1 / 32.0 GB belegt' },
        disk: { val: 93, status: 'crit', tag: 'KRITISCH (>90%)', detail: '930 GB / 1.0 TB NVMe' }
      },
      net: { inRate: '842.5 Mbit/s', outRate: '410.2 Mbit/s' },
      containers: {
        healthy:   { val: '40', status: 'warn' },
        unhealthy: { val: '2',  status: 'crit' },
        restarts:  { val: '9',  status: 'crit' }
      },
      incidents: [
        { level: 'crit', badge: 'KRITISCH', title: 'Host offline · docker-node-02 nicht erreichbar', meta: 'Beszel · Heartbeat Timeout seit 90 Sekunden', time: 'AKTIV' },
        { level: 'crit', badge: 'KRITISCH', title: 'Backup fehlgeschlagen · Offsite-Storage Timeout', meta: 'Backup-Status-Container · Exit-Code 2 bei Nightly-Sync', time: 'vor 18 Min.' },
        { level: 'crit', badge: 'KRITISCH', title: 'Immich HTTP-Check fehlgeschlagen (502 Bad Gateway)', meta: 'Uptime Kuma · Container immich_server unhealthy', time: 'AKTIV' },
        { level: 'warn', badge: 'WARNUNG',  title: 'Jellyfin restart x3', meta: 'Dazzle · Container Restarts in den letzten 24h', time: 'vor 2 Std.' },
        { level: 'warn', badge: 'WARNUNG',  title: 'SSL läuft in 14 Tagen ab', meta: 'Uptime Kuma · Zertifikats-Warnung', time: '14d Restzeit' }
      ]
    }
  };

  var verdictEl = document.getElementById('global-verdict');
  var verdictTextEl = document.getElementById('global-verdict-text');
  var incidentsListEl = document.getElementById('incidents-list');
  var simBtns = [].slice.call(document.querySelectorAll('[data-sim]'));
  var svcTiles = [].slice.call(document.querySelectorAll('.svc-tile'));

  function applyScenario(key) {
    var sc = scenarios[key] || scenarios.ok;

    simBtns.forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.getAttribute('data-sim') === key));
    });

    if (verdictEl && verdictTextEl) {
      verdictEl.className = 'global-verdict' + (sc.verdictClass ? ' ' + sc.verdictClass : '');
      verdictTextEl.textContent = sc.verdictText;
    }

    // Zeile 1: Executive Summary
    Object.keys(sc.exec).forEach(function (id) {
      var card = document.querySelector('[data-exec-id="' + id + '"]');
      if (!card) return;
      var info = sc.exec[id];
      card.className = 'stat-card' + (info.status !== 'ok' ? ' status-' + info.status : '');
      var valEl = card.querySelector('.stat-value');
      var subEl = card.querySelector('.stat-sub');
      if (valEl) valEl.textContent = info.val;
      if (subEl) subEl.textContent = info.sub;
    });

    // Zeile 2: Kritische Dienste (Strictly binary Green = Online, Red = Offline)
    svcTiles.forEach(function (tile) {
      var id = tile.getAttribute('data-svc-id');
      var isOff = sc.offlineServices.indexOf(id) !== -1;
      setServiceTileState(tile, !isOff);
    });

    // Zeile 3: Infrastruktur Gauges
    Object.keys(sc.gauges).forEach(function (gid) {
      var gCard = document.querySelector('[data-gauge-id="' + gid + '"]');
      if (!gCard) return;
      var g = sc.gauges[gid];
      gCard.className = 'gauge-card' + (g.status !== 'ok' ? ' status-' + g.status : '');
      var numEl = gCard.querySelector('.gauge-num');
      var tagEl = gCard.querySelector('.gauge-state-tag');
      var detEl = gCard.querySelector('.gauge-detail');
      var fillEl = gCard.querySelector('.gauge-fill');
      if (numEl) numEl.textContent = g.val + ' %';
      if (tagEl) tagEl.textContent = g.tag;
      if (detEl) detEl.textContent = g.detail;
      if (fillEl) fillEl.style.width = g.val + '%';
    });

    var inEl = document.getElementById('net-in-val');
    var outEl = document.getElementById('net-out-val');
    if (inEl) inEl.textContent = sc.net.inRate;
    if (outEl) outEl.textContent = sc.net.outRate;

    // Zeile 4: Container Health
    var cHealthy = document.getElementById('cnt-healthy');
    var cUnhealthy = document.getElementById('cnt-unhealthy');
    var cRestarts = document.getElementById('cnt-restarts');
    if (cHealthy) {
      cHealthy.className = 'cnt-stat' + (sc.containers.healthy.status !== 'ok' ? ' status-' + sc.containers.healthy.status : '');
      cHealthy.querySelector('.cnt-num').textContent = sc.containers.healthy.val;
    }
    if (cUnhealthy) {
      cUnhealthy.className = 'cnt-stat' + (sc.containers.unhealthy.status !== 'ok' ? ' status-' + sc.containers.unhealthy.status : '');
      cUnhealthy.querySelector('.cnt-num').textContent = sc.containers.unhealthy.val;
    }
    if (cRestarts) {
      cRestarts.className = 'cnt-stat' + (sc.containers.restarts.status !== 'ok' ? ' status-' + sc.containers.restarts.status : '');
      cRestarts.querySelector('.cnt-num').textContent = sc.containers.restarts.val;
    }

    // Zeile 5: Incidents (Max 5)
    if (incidentsListEl) {
      incidentsListEl.innerHTML = sc.incidents.slice(0, 5).map(function (inc) {
        return (
          '<li class="incident-card level-' + inc.level + '">' +
            '<div class="inc-left">' +
              '<span class="inc-badge">' + inc.badge + '</span>' +
              '<div class="inc-text">' +
                '<span class="inc-title">' + inc.title + '</span>' +
                '<span class="inc-meta">' + inc.meta + '</span>' +
              '</div>' +
            '</div>' +
            '<span class="inc-time">' + inc.time + '</span>' +
          '</li>'
        );
      }).join('');
    }
  }

  function setServiceTileState(tile, online) {
    tile.classList.toggle('is-offline', !online);
    var badge = tile.querySelector('.svc-badge');
    var lat = tile.querySelector('.svc-latency');
    var defaultLat = tile.getAttribute('data-default-latency') || '5 ms';
    if (badge) {
      badge.textContent = online ? '✅ ONLINE' : '🚨 OFFLINE';
    }
    if (lat) {
      lat.textContent = online ? defaultLat : 'TIMEOUT';
    }
  }

  // Click any service in Zeile 2 to test instant binary Green/Red outage detection
  svcTiles.forEach(function (tile) {
    tile.addEventListener('click', function () {
      var currentlyOffline = tile.classList.contains('is-offline');
      setServiceTileState(tile, currentlyOffline);

      var offCount = document.querySelectorAll('.svc-tile.is-offline').length;
      var onlineCount = 18 - offCount;
      var execSvc = document.querySelector('[data-exec-id="services-online"]');
      if (execSvc) {
        execSvc.className = 'stat-card' + (offCount > 0 ? ' status-crit' : '');
        execSvc.querySelector('.stat-value').textContent = onlineCount + ' / 18';
        execSvc.querySelector('.stat-sub').textContent = offCount > 0
          ? (offCount + ' kritischer Dienst OFFLINE')
          : '100 % Verfügbarkeit · HTTP/TCP';
      }
      if (verdictEl && verdictTextEl) {
        if (offCount > 0) {
          verdictEl.className = 'global-verdict is-crit';
          verdictTextEl.textContent = 'EINGRIFF ERFORDERLICH · ' + offCount + ' KRITISCHER DIENST OFFLINE';
        } else {
          verdictEl.className = 'global-verdict';
          verdictTextEl.textContent = 'ALLE SYSTEME NOMINAL · KEIN EINGRIFF ERFORDERLICH';
        }
      }
    });
  });

  simBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyScenario(btn.getAttribute('data-sim'));
    });
  });
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
