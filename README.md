# teams-luxafor-sync

Syncs Luxafor color with Microsoft Teams presence on macOS.

- Busy-like statuses -> red
- All other statuses -> green

## Install with Homebrew

```bash
brew tap jonasrappy/tap
brew install teams-luxafor-sync
brew services start teams-luxafor-sync
```

## Check status

```bash
brew services list | rg teams-luxafor-sync
```

Logs:

```bash
tail -f "$(brew --prefix)/var/log/teams-luxafor-sync.log"
tail -f "$(brew --prefix)/var/log/teams-luxafor-sync-error.log"
```

## Stop / Uninstall

```bash
brew services stop teams-luxafor-sync
brew uninstall teams-luxafor-sync
```

## First-run permissions (macOS)

The process may need macOS privacy access to read Teams logs.
If sync does not react, grant Full Disk Access to the installed binary path:

```bash
$(brew --prefix)/bin/teams-luxafor-sync
```

## Optional env vars

- `POLL_MS` (default `3000`)
- `TAIL_BYTES` (default `262144`)
- `FALLBACK_LOG_SCAN_COUNT` (default `5`)
- `REAPPLY_MS` (default `15000`)
- `TEAMS_LOG_DIR` (override log directory)

## Build locally

```bash
go build -o ./bin/teams-luxafor-sync ./cmd/teams-luxafor-sync
./bin/teams-luxafor-sync -once
```
