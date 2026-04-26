#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-$HOME/apps/lojf}"
SERVICE_NAME="${SERVICE_NAME:-lojf-nextgen}"
BRANCH="${BRANCH:-main}"

cd "$APP_DIR"

/opt/homebrew/bin/git pull origin "$BRANCH" 2>/dev/null || git pull origin "$BRANCH"
/usr/bin/make build

if command -v sudo >/dev/null 2>&1; then
  sudo systemctl restart "$SERVICE_NAME"
else
  systemctl restart "$SERVICE_NAME"
fi

echo "Deployed at $(date)"
