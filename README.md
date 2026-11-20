# Homelab Live NOC Server Dashboard (Pure Go Single-Binary)

Ultraschlankes, produktionsreifes **Live Network Operations Center (NOC) Dashboard** für dein Homelab (`192.168.178.90`) im Editorial-Design von `info.moritzhammer.org` (inkl. **Tag- & Nachtmodus**, **3 interaktiver CMY-Kreise**, **ausklappbarer Infokarten**, **Tablet/Desktop-Umschalter**, **Beszel & Uptime Kuma Live-Fenstern** und **WUD Update-Tracker**).

## Warum Pure Go Single-Binary statt Hugo?

- **~15 MB Docker-Image**, **~10 MB RAM im Dauerbetrieb**, **0 % Idle-CPU** und **0 Disk-I/O** — HTML, CSS, JavaScript und lokal gehostete `.woff2`-Schriften werden per `//go:embed` direkt in das statische Go-Binary kompiliert (keine externen Abhängigkeiten, kein Node.js, kein Nginx, keine Volume-Mount-Fehler).
- **Eingebauter Same-Origin Reverse Proxy** für **Beszel** (`:8099/system/n9qil9mgcw9fhwm`) und **Uptime Kuma** (`:3001/status/overview`): entfernt automatisch `X-Frame-Options` & `Content-Security-Policy`, sodass beide Live-Fenster sofort ohne Browser-Blockade und ohne Änderung an deinen anderen Containern funktionieren.

---

## Features im Überblick

1. **Tag- & Nachtmodus + CMY-Kreise**
   - **Tagmodus:** Warmes Papier-/Stone-Design exakt wie `info.moritzhammer.org`.
   - **Nachtmodus:** Kontrastreiches, AMOLED-schonendes NOC-Dark-Theme.
   - Animiertes CMY-Markenzeichen in der Topbar + interaktive, ziehbare CMY-Kreise im Desktop-Header.
2. **Tablet- vs. Desktop-Modus (sofort umschaltbar & gespeichert)**
   - **Tablet (`100dvh` Kiosk):** Kompakter 1180px-Kiosk-Frame ohne Scrollen, optimiert für 8–10 Zoll Android-Tablets (inkl. Vollbild & Screen Wake-Lock).
   - **Desktop:** Großzügige 1680px-Ansicht mit CMY-Editorial-Header, ausklappbaren Detail-Karten und Schnellzugriffsleiste für alle aktiven Dienste.
3. **Jede Infokarte ausklappbar**
   - Executive Summary, kritische Dienste (inkl. Container-Status, HTTP-Code & Direktlink), Infrastruktur-Gauges (Load 1m/5m/15m, RAM-Cache, Disk-Frei, RX/TX gesamt), Container-/WUD-Kacheln, Incidents, Docker Explorer und SSL-Inspector lassen sich per Klick aufklappen — der offene Zustand bleibt über Live-Updates hinweg erhalten.
4. **4 echte Live-Reiter (ohne statischen Grafana/PromQL-Ballast)**
   - **NOC:** 5-Zeilen-Statusmonitor („Muss ich eingreifen oder kann ich entspannt weitermachen?“).
   - **Beszel & Kuma:** Eigener Reiter mit Live-Fenstern für `http://192.168.178.90:8099/system/n9qil9mgcw9fhwm` und `http://192.168.178.90:3001/status/overview` (Nebeneinander / Einzeln / Proxy vs. Direkt / Reload / Extern öffnen) + nativer Kuma-/Beszel-Statusleiste darüber.
   - **WUD:** Live-Updates von What's Up Docker (`/api/containers`) inkl. `MAJOR`/`MINOR`/`PATCH`-Badges und **DB-Backup-Warnung** für datenbankkritische Apps (`Immich`, `Paperless-ngx`, `Vaultwarden`, `Authelia`, `Nginx Proxy Manager`).
   - **Live-Telemetrie:** Rollende Echtzeit-SVG-Graphen (CPU %, RAM %, Netzwerk RX/TX mit 2-Sekunden-Sampling aus `/host/proc`), filterbarer **Docker Container Explorer** (Alle / Unhealthy / Mit Restarts) und **Live SSL-Inspector**.
5. **Nur deine echten Homelab-Dienste (`192.168.178.90`)**
   - `Vaultwarden (:8081)`, `Immich (:2283)`, `Paperless-ngx (:8000)`, `Nginx Proxy Manager (:81)`, `Uptime Kuma (:3001)`, `Website (:8080)` + Schnellzugriff auf `Beszel (:8099)`, `Dozzle (:8888)` und `Filebrowser (:8082)`.

---

## Deployment in Portainer (Port `3000`)

1. **Stacks → Add stack → Repository** (oder beim bestehenden Stack **Pull and redeploy**):
   - **Repository URL:** `https://github.com/MoritzHammer/server-dashboard`
   - **Repository reference:** `refs/heads/main`
   - **Compose path:** `docker-compose.yml`
2. Optionale Umgebungsvariablen:

| Variable | Standard | Beschreibung |
| --- | --- | --- |
| `PORT` | `3000` | Host-Port in Portainer |
| `UPTIME_KUMA_URL` | `http://192.168.178.90:3001` | Basis-URL deiner Uptime-Kuma-Instanz |
| `UPTIME_KUMA_STATUS_SLUG` | `overview` | Öffentliche Statusseite (versorgt automatisch Zeile 1 & Kuma-Statusleiste ohne API-Key) |
| `UPTIME_KUMA_EMBED_URL` | `http://192.168.178.90:3001/status/overview` | Ziel-URL für das Kuma-Live-Fenster |
| `BESZEL_URL` | `http://192.168.178.90:8099` | Basis-URL deines Beszel Hubs |
| `BESZEL_EMBED_URL` | `http://192.168.178.90:8099/system/n9qil9mgcw9fhwm` | Ziel-URL für das Beszel-Live-Fenster |
| `BESZEL_EMAIL` & `BESZEL_PASSWORD` | *(optional)* | Read-Only-Account für zusätzliche Multi-Host-KPIs in Zeile 1 |
| `ENABLE_EMBED_PROXY` | `true` | Integrierter Same-Origin-Proxy (entfernt `X-Frame-Options` automatisch; auf `false` setzen, falls das Dashboard öffentlich ins Internet gestellt wird) |
| `WUD_URL` | *(auto via Docker Socket)* | Manuelle WUD-URL, z. B. `http://192.168.178.90:3005` |
| `SSL_DOMAINS` | `info.moritzhammer.org` | Komma-separierte Domains für Live-TLS-Prüfung (oder `none`) |
