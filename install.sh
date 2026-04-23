#!/usr/bin/env bash
set -euo pipefail

LABEL="com.teams-luxafor-sync"
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
PLIST_TARGET="$LAUNCH_AGENTS_DIR/$LABEL.plist"
LOG_OUT="$HOME/Library/Logs/teams-luxafor-sync.log"
LOG_ERR="$HOME/Library/Logs/teams-luxafor-sync-error.log"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="$SCRIPT_DIR/teams-luxafor-sync.js"
TEMPLATE_PATH="$SCRIPT_DIR/com.teams-luxafor-sync.plist.template"
NODE_BIN="$(command -v node || true)"

if [[ -z "$NODE_BIN" ]]; then
  echo "Error: node was not found in PATH. Install Node.js 18+ first."
  exit 1
fi

if [[ ! -f "$SCRIPT_PATH" ]]; then
  echo "Error: missing $SCRIPT_PATH"
  exit 1
fi

if [[ ! -f "$TEMPLATE_PATH" ]]; then
  echo "Error: missing $TEMPLATE_PATH"
  exit 1
fi

mkdir -p "$LAUNCH_AGENTS_DIR"
mkdir -p "$HOME/Library/Logs"

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

ESC_NODE_BIN="$(escape_sed_replacement "$NODE_BIN")"
ESC_SCRIPT_PATH="$(escape_sed_replacement "$SCRIPT_PATH")"
ESC_LOG_OUT="$(escape_sed_replacement "$LOG_OUT")"
ESC_LOG_ERR="$(escape_sed_replacement "$LOG_ERR")"

sed \
  -e "s/__NODE_BIN__/$ESC_NODE_BIN/g" \
  -e "s/__SCRIPT_PATH__/$ESC_SCRIPT_PATH/g" \
  -e "s/__LOG_OUT__/$ESC_LOG_OUT/g" \
  -e "s/__LOG_ERR__/$ESC_LOG_ERR/g" \
  "$TEMPLATE_PATH" > "$PLIST_TARGET"

# Install dependencies locally
cd "$SCRIPT_DIR"
if [[ -f package-lock.json ]]; then
  npm ci --omit=dev
else
  npm install --omit=dev
fi

# Restart agent cleanly
launchctl bootout "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
if launchctl bootstrap "gui/$(id -u)" "$PLIST_TARGET" >/dev/null 2>&1; then
  launchctl enable "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
  launchctl kickstart -k "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
else
  # Fallback for shells/environments where bootstrap returns I/O errors.
  launchctl unload "$PLIST_TARGET" >/dev/null 2>&1 || true
  launchctl load "$PLIST_TARGET"
fi

echo "Installed and started: $LABEL"
echo "Plist: $PLIST_TARGET"
echo "Logs:  $LOG_OUT"
