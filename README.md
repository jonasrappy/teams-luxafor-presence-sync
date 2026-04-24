# teams-luxafor-sync

Syncs Luxafor color with Microsoft Teams presence on macOS.

- Busy-like statuses -> red
- Away-like statuses (`BeRightBack`, `Away`) -> yellow
- All other statuses -> green

## Requirements

- macOS (Apple Silicon or Intel)
- Microsoft Teams desktop app with presence enabled
- A connected Luxafor device

## Install (Homebrew)

```bash
brew update
brew tap jonasrappy/tap
brew install jonasrappy/tap/teams-luxafor-sync
brew services start jonasrappy/tap/teams-luxafor-sync
```

`brew services start` enables autostart at user login.

## Verify service

```bash
brew services list | grep -E '^teams-luxafor-sync\s'
```

Logs:

```bash
tail -f "$(brew --prefix)/var/log/teams-luxafor-sync.log"
tail -f "$(brew --prefix)/var/log/teams-luxafor-sync-error.log"
```

## Update

```bash
brew update
brew upgrade jonasrappy/tap/teams-luxafor-sync
brew services restart teams-luxafor-sync
```

## Stop / Uninstall

```bash
brew services stop teams-luxafor-sync
brew uninstall teams-luxafor-sync
```

## First-run permissions (macOS)

The service may need privacy access to read Teams logs under `~/Library/Group Containers`.
If color sync does not react:

```bash
open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
```

Then grant **Full Disk Access** to:

```bash
$(which teams-luxafor-sync)
```

and restart:

```bash
brew services restart teams-luxafor-sync
```

Quick one-shot diagnostics:

```bash
teams-luxafor-sync -once
```

## Optional env vars

- `POLL_MS` (default `300`)
- `TAIL_BYTES` (default `262144`)
- `FALLBACK_LOG_SCAN_COUNT` (default `5`)
- `REAPPLY_MS` (default `15000`)
- `TEAMS_LOG_DIR` (override log directory)

## Build locally

```bash
go build -o ./bin/teams-luxafor-sync ./cmd/teams-luxafor-sync
./bin/teams-luxafor-sync -once
```
