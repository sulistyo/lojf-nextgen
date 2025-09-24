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
