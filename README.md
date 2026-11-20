# Homelab Live NOC Server Dashboard (Hugo + Go Live Collector)

Produktionsreifes **Live Network Operations Center (NOC) Dashboard** speziell für dein Homelab (`192.168.178.90`), umschaltbar zwischen **Tablet-Kiosk-Modus** (`100dvh`, kein Scrollen) und **Desktop-Vollansicht** (inkl. eingebetteter Live-Fenster für **Beszel** & **Uptime Kuma**, **WUD Update-Tracker** und Schnellzugriff).

- **Frontend:** Generiert mit **[Hugo (`gohugo.io`)](https://gohugo.io)** im Editorial-Design von `info.moritzhammer.org` (lokal gehostete `.woff2`-Schriften, 100 % DSGVO-konform).
- **Live-Backend:** Eingebetteter, abhängigkeitsfreier **Go-Server** ([`cmd/noc-server/main.go`](cmd/noc-server/main.go)), der unter **`/api/status`** alle 5–10 Sekunden echte Live-Daten bereitstellt.

---

## Features im Überblick

1. **Tablet- vs. Desktop-Modus (oben umschaltbar & gespeichert)**
   - **Tablet (`100dvh` Kiosk):** Kompakte 5-Zeilen-Ansicht ohne Scrollen für 8–10 Zoll Android-Tablets + eigene Vollbild-Tabs für **Beszel & Kuma** und **WUD**.
   - **Desktop (Vollansicht):** Alle 5 NOC-Zeilen plus direkt darunter die eingebetteten Live-Fenster (**Beszel** & **Uptime Kuma** nebeneinander oder einzeln), der **WUD Update-Tracker** und die **Homelab-Schnellzugriffsleiste**.
2. **Nur deine echten Homelab-Dienste (`192.168.178.90`)**
   - `Vaultwarden (:8081)`, `Immich (:2283)`, `Paperless-ngx (:8000)`, `Nginx Proxy Manager (:81)`, `Uptime Kuma (:3001)`, `Website (:8080)` sowie Schnellzugriff auf `Beszel (:8090)`, `Dozzle (:8888)` und `Filebrowser (:8082)`.
3. **WUD (What's Up Docker) Live-Integration**
   - Erkennt laufende WUD-Container automatisch über den Docker Socket (oder per `WUD_URL`), fragt `/api/containers` ab und listet verfügbare Updates inkl. Version-Diff (`MAJOR` / `MINOR` / `PATCH`) und **DB-Backup-Warnung** für datenbankkritische Apps (`Immich`, `Paperless-ngx`, `Vaultwarden`, `Authelia`, `Nginx Proxy Manager`).
4. **Beszel & Uptime Kuma Live-Fenster + API**
   - Zeigt Beszel (`http://192.168.178.90:8090`) und Uptime Kuma (`http://192.168.178.90:3001`) direkt als HTML-Fenster (Split-View / Einzelansicht / Reload / Extern öffnen).
   - Optional über `BESZEL_EMAIL` + `BESZEL_PASSWORD` (PocketBase JWT) und `UPTIME_KUMA_API_KEY` / `UPTIME_KUMA_STATUS_SLUG` zusätzlich direkt in den Executive-Summary-Zahlen auswertbar.

---

## Deployment in Portainer (Port `3000`)

1. **Stacks → Add stack → Repository**
   - **Repository URL:** `https://github.com/MoritzHammer/server-dashboard`
   - **Repository reference:** `refs/heads/main`
   - **Compose path:** `docker-compose.yml`
   - *(Falls das Repo auf **Privat** gestellt wird: in Portainer unter **Authentication** GitHub-Username + GitHub Personal Access Token mit `Contents: Read-only` eintragen.)*
2. Optionale Umgebungsvariablen in Portainer:

| Variable | Standard / Beispiel |
| --- | --- |
| `PORT` | `3000` (Host-Port; falls z. B. `homepage_dashboard` Port `3000` nutzt, z. B. `3002` setzen) |
| `BESZEL_URL` / `BESZEL_EMBED_URL` | `http://192.168.178.90:8090` (oder HTTPS-Domain bei Zugriff über NPM) |
| `BESZEL_EMAIL` & `BESZEL_PASSWORD` | Optionaler Read-Only-Benutzer für native Beszel-Multi-Host-Metriken |
| `UPTIME_KUMA_URL` / `UPTIME_KUMA_EMBED_URL` | `http://192.168.178.90:3001` (für Iframes in Kuma ggf. X-Frame-Options deaktivieren oder `UPTIME_KUMA_STATUS_SLUG` setzen) |
| `UPTIME_KUMA_API_KEY` | Optional für `/metrics`-Auswertung |
| `WUD_URL` | Automatische Port-Erkennung via Docker Socket oder manuell `http://192.168.178.90:<WUD-PORT>` |
| `WUD_USER` & `WUD_PASS` | Optional, falls WUD Basic Auth nutzt |
| `CRITICAL_SERVICES` | Überschreibt Zeile 2 (`Name\|Kategorie\|http(s)/tcp/docker://...`) |
| `SSL_DOMAINS` | `info.moritzhammer.org` (Komma-separiert oder `none`) |
