# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary (CGO_ENABLED=1 required for SQLite)
make run     # Run compiled binary
make test    # go test ./... -v
make fmt     # go fmt ./...
make tidy    # go mod tidy
make clean   # Remove bin/
```

Run the server: `./bin/lojf-nextgen` → listens on `http://127.0.0.1:8080`

## Architecture

**Stack:** Go 1.22 · Chi router · GORM + SQLite · html/template · Tailwind CSS + Flowbite · HTMX · Telegram bot

### Directory Layout

```
cmd/server/main.go        # Entry point
internal/
  db/db.go                # GORM connection + auto-migrations
  models/                 # Domain models (Parent, Child, Class, Registration, etc.)
  handlers/               # HTTP handlers (~24 files, one concern per file)
  services/               # Business logic (registration, phone, email)
  events/events.go        # Internal event bus (e.g. OnPromotion triggers Telegram)
  bot/                    # Telegram bot client, dispatcher, reminder scheduler
  web/router.go           # Chi router setup + template loading with FuncMap
templates/
  layouts/base.tmpl       # Master layout (loads Tailwind, Flowbite, HTMX from CDN)
  pages/admin/            # Admin page templates
  pages/parents/          # Parent-facing page templates
  partials/               # navbar, flash, footer
```

### Key Patterns

**Handlers** are factory functions: `func NewFooHandler(db *gorm.DB, tmpl *template.Template) http.HandlerFunc`. Templates are injected at creation time, not looked up per-request.

**Routing** is split in `web/router.go`:
- Public: `/`, `/register`, `/account`, `/my`, `/cancel`, `/qr/{code}.png`
- Admin: `/admin/*` — protected by `RequireAdmin` middleware
- Telegram webhook: `/tg/webhook`
- Check-in: `/checkin`, `/admin/checkin`

**Session management** uses plain HTTPOnly cookies (`parent_phone`, `parent_name`). No JWT or external session store. `RequireParent` and `RequireAdmin` are middleware functions.

**Template helpers** (FuncMap in `web/router.go`): `jdate`, `jisodate`, `jlong`, `fmtDate`, `fmtDateTime` all use `Asia/Jakarta` timezone. `nl2br` and `unescape` handle text formatting.

**POST-Redirect-GET** is the standard form submission pattern. Flash messages are passed via cookies and rendered by `partials/flash.tmpl`.

### Registration State Machine

```
pending → confirmed   (capacity available)
        → waitlisted  (class full)
waitlisted → confirmed  (auto-promoted via recomputeClass when slot opens)
confirmed/waitlisted → canceled
```

`services/registration.go` owns capacity checks, waitlist ordering (FIFO by `created_at, id`), and triggers `events.OnPromotion()` when a waitlisted registration is promoted.

### Data Model Highlights

- **Parent** identified by unique phone number; owns one or more **Child** records.
- **Class** is a schedulable session with capacity. Belongs to a **ClassTemplate** (reusable question/description blueprint).
- **Registration** links a Child to a Class, has a unique `code` (used for QR), a status, and an optional `check_in_at`.
- **ClassQuestion** / **ClassTemplateQuestion** implement a dynamic question system; answers stored in **RegistrationAnswer**.
- `SignupOpensAt` is nullable — null means open immediately.

### Telegram Bot

- Enabled via `TG_BOT_TOKEN` environment variable (bot is no-op if unset).
- Parents link their Telegram account using a `LinkCode`.
- `bot/reminders.go` schedules recurring notifications; `events/events.go` triggers immediate messages (e.g. promotion confirmations).

### Environment Variables

| Variable | Purpose |
|---|---|
| `ADDR` | Listen address (default `:8080`) |
| `TG_BOT_TOKEN` | Telegram bot token (optional) |

Database file `nextgen.db` is created in the working directory on first run.

### Deployment

Single binary deployment behind Apache reverse proxy. `bootstrap.sh` sets up a systemd service. `make build` → copy `bin/lojf-nextgen` to server.
