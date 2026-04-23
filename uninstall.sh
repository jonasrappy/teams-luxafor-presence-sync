#!/usr/bin/env bash
set -euo pipefail

LABEL="com.teams-luxafor-sync"
PLIST_TARGET="$HOME/Library/LaunchAgents/$LABEL.plist"

launchctl bootout "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
launchctl disable "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
rm -f "$PLIST_TARGET"

echo "Uninstalled: $LABEL"
