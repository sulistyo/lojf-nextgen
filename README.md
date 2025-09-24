# LOJF NextGen Manager (Starter)

A clean, modular Go web app for LOJF Kids Ministry: registrations, class management, and check‑in.

## Stack
- Go (chi)
- GORM (SQLite) – swap to Postgres later
- Tailwind + Flowbite + HTMX templates
- Apache reverse proxy
- systemd service

## Quick Start (local/dev)
```bash
make build
./bin/lojf-nextgen
# open http://127.0.0.1:8080
```

## Deploy on Server (/root/apps/lojf)
1. Upload and unzip this repo:
   ```bash
   mkdir -p /root/apps/lojf && cd /root/apps/lojf
   unzip /path/to/lojf-nextgen-starter.zip -d .
   go mod tidy
   make build
   ./bin/lojf-nextgen &
   ```
2. Apache reverse proxy:
   ```bash
   a2enmod proxy proxy_http headers
   cp deploy/apache-nextgen.conf /etc/apache2/sites-available/nextgen.lojf.id.conf
   a2ensite nextgen.lojf.id.conf
   systemctl reload apache2
   ```
3. Systemd:
   ```bash
   cp deploy/lojf-nextgen.service /etc/systemd/system/
   systemctl daemon-reload
   systemctl enable --now lojf-nextgen
   systemctl status lojf-nextgen
   ```

## Health Check
```bash
curl -s http://127.0.0.1:8080/healthz
```

## Next Steps
- Telegram bot endpoints
- QR code issuance (go-qrcode) for confirmations
- Class capacity & waitlist logic
- Auth for Admin area
