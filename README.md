# Homelab NOC Server Dashboard (Hugo + Grafana Blueprint)

Produktionsreifes **Network Operations Center (NOC) Dashboard** für ein Selfhosting-Homelab, optimiert für den 24/7-Dauerbetrieb auf einem **8–10 Zoll Android-Tablet im Kiosk-Modus** sowie als vollständige **Grafana- & Prometheus-Architektur-Referenz**.

Entwickelt mit **[Hugo (`gohugo.io`)](https://gohugo.io)** im gleichen Editorial-Design wie `info.moritzhammer.org` (lokal gehostete `.woff2`-Schriften `Big Shoulders Display`, `Instrument Sans`, `Instrument Serif`, 100 % DSGVO-konform, 0 externe Requests, < 20 KB HTML).

---

## Hauptziel

> **„Muss ich eingreifen oder kann ich entspannt weitermachen?“**

- **Regelbetrieb (Nominal):** Ruhiges, tiefdunkles NOC-Design (`#0B0F17`), überwiegend **Smaragdgrün (`#10B981`)**, aus **3 Metern Entfernung** innerhalb von **2–3 Sekunden** ohne Scrollen (`100dvh`) erfassbar.
- **Störungsfall:** Betroffene Dienste, Hosts, Ressourcen oder Container stechen sofort in **Gelb (`#F59E0B`)** oder **Kritisch-Rot (`#EF4444`)** hervor.

---

## 5-Zeilen-NOC-Aufbau

1. **Zeile 1 – Executive Summary (4 große Stat-Panels)**
   - **Dienste online:** `18 / 18` *(Quelle: Uptime Kuma)*
   - **Hosts online:** `5 / 5` *(Quelle: Beszel)*
   - **Container healthy:** `42 / 42` *(Quelle: Dazzle)*
   - **SSL-Zertifikate:** `OK` *(Quelle: Uptime Kuma)*
2. **Zeile 2 – Kritische Dienste (Binäre Status-Matrix: Grün = Online, Rot = Offline)**
   - `Vaultwarden`, `Immich`, `Home Assistant`, `Gitea`, `Traefik`, `WireGuard`
3. **Zeile 3 – Infrastrukturzustand (Prometheus / Node Exporter / Beszel)**
   - **CPU:** Grün `< 70 %` · Gelb `70–85 %` · Rot `> 85 %`
   - **Arbeitsspeicher:** Grün `< 75 %` · Gelb `75–90 %` · Rot `> 90 %`
   - **Datenträger:** Grün `< 80 %` · Gelb `80–90 %` · Rot `> 90 %`
   - **Netzwerk:** Inbound / Outbound inkl. kompakter Sparkline
4. **Zeile 4 – Container-Gesundheit (Quelle: Dazzle)**
   - `Healthy Container (42)` · `Unhealthy Container (0)` · `Container Restarts letzte 24h (3)`
   - Top-Restarts-Liste (`Jellyfin: 3`, `Immich: 1`)
5. **Zeile 5 – Warnungen / Incidents (Maximal 5 relevante Ereignisse)**
   - Farbcodiert nach Priorität: **Rot = Kritisch**, **Gelb = Warnung**, **Blau = Information**

---

## Deployment in Portainer

In Portainer als Stack über **Git Repository** einbinden:
- **Repository URL:** `https://github.com/MoritzHammer/server-dashboard`
- **Repository reference:** `refs/heads/main`
- **Compose path:** `docker-compose.yml`
- **Port:** `http://192.168.178.90:3000` (Standardmäßig `3000:80`, optional über die Portainer-Umgebungsvariable `PORT` anpassbar)

Das direkt importierbare Grafana-Dashboard liegt zusätzlich unter [`/grafana-noc-dashboard.json`](static/grafana-noc-dashboard.json).
