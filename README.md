# Homelab Live NOC Server Dashboard (Hugo + Go Live Collector)

Produktionsreifes **Live Network Operations Center (NOC) Dashboard** für ein Selfhosting-Homelab, optimiert für den 24/7-Dauerbetrieb auf einem **8–10 Zoll Android-Tablet im Kiosk-Modus**.

- **Frontend:** Generiert mit **[Hugo (`gohugo.io`)](https://gohugo.io)** im Editorial-Design von `info.moritzhammer.org` (lokal gehostete `.woff2`-Schriften, 100 % DSGVO-konform, 0 externe Requests).
- **Live-Backend:** Eingebetteter, abhängigkeitsfreier **Go-Server** (`cmd/noc-server/main.go`), der das Hugo-Frontend ausliefert und unter **`/api/status`** alle 5–10 Sekunden echte Live-Daten liefert.

---

## Echte Live-Datenquellen (Out-of-the-Box aktiv)

1. **Docker Engine Socket (`/var/run/docker.sock:ro`)**
   - Live-Zählung aller laufenden, gesunden (`healthy`) und fehlerhaften (`unhealthy` / `restarting` / `OOMKilled`) Container.
   - Echte `RestartCount`-Werte inkl. Top-Restart-Liste in Zeile 4.
2. **Linux Host-Telemetrie (`/proc:/host/proc:ro` + Disk `statfs`)**
   - Live-CPU-Auslastung & Load Average (Grün `< 70 %`, Gelb `70–85 %`, Rot `> 85 %`).
   - Live-Arbeitsspeicher in % und GB (Grün `< 75 %`, Gelb `75–90 %`, Rot `> 90 %`).
   - Live-Datenträgerbelegung in % und GB (Grün `< 80 %`, Gelb `80–90 %`, Rot `> 90 %`).
   - Live-Netzwerkdurchsatz (Inbound / Outbound Mbit/s) inkl. rollender 16-Punkt-SVG-Sparkline.
3. **Aktive Live-Probes (HTTP / HTTPS / TCP / Docker) & Live-TLS-SSL-Check**
   - Prüft in Zeile 2 standardmäßig deine laufenden Dienste auf `192.168.178.90` (`Vaultwarden`, `Immich`, `Paperless`, `Nginx Proxy`, `Uptime Kuma`, `Website`) mit echter Latenz in Millisekunden.
   - Prüft per echtem TLS-Handshake auf Port `443` die Restlaufzeit deiner SSL-Zertifikate (`info.moritzhammer.org`, `moritzhammer.de`, `hammermoritz.de`).
4. **Optionale Anbindung an Uptime Kuma & Prometheus**
   - Sobald `UPTIME_KUMA_URL` (+ `UPTIME_KUMA_API_KEY` oder `UPTIME_KUMA_STATUS_SLUG`) bzw. `PROMETHEUS_URL` in Portainer gesetzt werden, übernimmt das Dashboard automatisch die Werte direkt aus Uptime Kuma (`/metrics` bzw. Status-Page-API) und Prometheus (`up`).

---

## Deployment in Portainer (Port `3000`)

1. **Stacks → Add stack → Repository**
   - **Repository URL:** `https://github.com/MoritzHammer/server-dashboard`
   - **Repository reference:** `refs/heads/main`
   - **Compose path:** `docker-compose.yml`
2. Optional unter **Environment variables** anpassen:

| Variable | Beispiel / Standardwert |
| --- | --- |
| `PORT` | `3000` (Host-Port) |
| `POLL_INTERVAL` | `10` (Sekunden) |
| `CRITICAL_SERVICES` | `Vaultwarden\|Passwort-Tresor\|http://192.168.178.90:8081,Immich\|Foto-Cloud\|http://192.168.178.90:2283,Home Assistant\|Smart Home\|http://192.168.178.90:8123,WireGuard\|VPN\|docker://wg-easy` |
| `HOSTS` | `Docker-Host\|local,Proxmox\|tcp://192.168.178.10:8006` |
| `SSL_DOMAINS` | `info.moritzhammer.org,moritzhammer.de,hammermoritz.de` |
| `UPTIME_KUMA_URL` | `http://192.168.178.90:3001` |
| `UPTIME_KUMA_API_KEY` | *(Optional: API-Key für `/metrics`)* |
| `UPTIME_KUMA_STATUS_SLUG` | *(Alternativ: Slug einer öffentlichen Uptime-Kuma-Statusseite)* |
| `PROMETHEUS_URL` | `http://192.168.178.90:9090` |

> **Unterstützte Probe-Ziele für `CRITICAL_SERVICES`:** `http://...`, `https://...`, `tcp://ip:port` oder `docker://<container-name>` (prüft den laufenden & gesunden Docker-Container direkt über den Socket).
