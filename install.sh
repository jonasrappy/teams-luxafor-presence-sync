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

check_teams_log_access() {
  "$NODE_BIN" <<'NODE'
const fs = require('fs');
const path = require('path');

const home = process.env.HOME;
const groupContainersRoot = path.join(home, 'Library/Group Containers');
const candidates = [
  path.join(
    home,
    'Library/Group Containers/UBF8T346G9.com.microsoft.teams/Library/Application Support/Logs'
  ),
  path.join(home, 'Library/Application Support/Microsoft/Teams/logs'),
];

let permissionDenied = false;
let readable = false;

function denied(err) {
  return err && (err.code === 'EACCES' || err.code === 'EPERM');
}

try {
  const groupDirs = fs.readdirSync(groupContainersRoot);
  for (const entry of groupDirs) {
    if (!entry.toLowerCase().includes('microsoft.teams')) continue;
    candidates.push(path.join(groupContainersRoot, entry, 'Library/Application Support/Logs'));
  }
} catch (err) {
  if (denied(err)) permissionDenied = true;
}

for (const dirPath of [...new Set(candidates)]) {
  try {
    const names = fs.readdirSync(dirPath);
    const logs = names.filter((name) => name.startsWith('MSTeams_') && name.endsWith('.log'));
    for (const name of logs.slice(0, 3)) {
      const filePath = path.join(dirPath, name);
      try {
        const fd = fs.openSync(filePath, 'r');
        fs.closeSync(fd);
        readable = true;
        break;
      } catch (err) {
        if (denied(err)) permissionDenied = true;
      }
    }
    if (readable) break;
  } catch (err) {
    if (denied(err)) permissionDenied = true;
  }
}

if (readable) process.exit(0);
if (permissionDenied) process.exit(10);
process.exit(11);
NODE
}

preflight_permissions() {
  set +e
  check_teams_log_access
  local rc=$?
  set -e

  if [[ $rc -eq 0 ]]; then
    return 0
  fi

  if [[ $rc -eq 11 ]]; then
    echo "Warning: Teams log files were not found yet. Start Teams and sign in before expecting sync."
    return 0
  fi

  echo
  echo "Permission needed: macOS is blocking Node from reading Teams logs."
  echo "Grant Full Disk Access to:"
  echo "  $NODE_BIN"
  echo
  echo "Opening Privacy settings now..."
  open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" >/dev/null 2>&1 || true
  echo
  read -r -p "After enabling access, press Enter to re-check..."

  set +e
  check_teams_log_access
  rc=$?
  set -e

  if [[ $rc -ne 0 && $rc -ne 11 ]]; then
    echo "Error: still no permission to read Teams logs for Node."
    echo "Please enable Full Disk Access for the Node binary above, then run ./install.sh again."
    exit 1
  fi
}

preflight_permissions

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
