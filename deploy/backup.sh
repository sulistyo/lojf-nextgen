#!/usr/bin/env bash
# Consistent, rotated SQLite backup for lojf-nextgen (safe for WAL mode).
# Intended to run from cron on the prod server. Writes gzipped snapshots to
# $BACKUP_DIR and keeps the newest $KEEP.
set -euo pipefail

APP_DIR="${APP_DIR:-/root/apps/lojf}"
BACKUP_DIR="${BACKUP_DIR:-$APP_DIR/backups}"
KEEP="${KEEP:-14}"
DB="$APP_DIR/nextgen.db"

mkdir -p "$BACKUP_DIR"
TS="$(date +%Y%m%d-%H%M%S)"
OUT="$BACKUP_DIR/nextgen-$TS.db"

# .backup gives a consistent snapshot even with the app running (WAL).
sqlite3 "$DB" ".backup '$OUT'"

# Verify before keeping; drop a corrupt snapshot instead of masking a good one.
if [ "$(sqlite3 "$OUT" 'PRAGMA integrity_check;')" != "ok" ]; then
  echo "$(date '+%F %T') ERROR: integrity_check failed, removing $OUT" >&2
  rm -f "$OUT"
  exit 1
fi

gzip -f "$OUT"

# Rotate: keep the newest $KEEP gzipped snapshots.
ls -1t "$BACKUP_DIR"/nextgen-*.db.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -f

echo "$(date '+%F %T') backup ok: $OUT.gz ($(du -h "$OUT.gz" | cut -f1)); kept $(ls -1 "$BACKUP_DIR"/nextgen-*.db.gz 2>/dev/null | wc -l | tr -d ' ')"
