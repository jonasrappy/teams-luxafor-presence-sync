# Teams -> Luxafor Sync (macOS)

Syncs a Luxafor USB indicator with Microsoft Teams presence on macOS.

## Behavior

- Busy-like statuses -> `red`
- All other statuses -> `green`

Busy-like statuses currently include:
`Busy`, `DoNotDisturb`, `InACall`, `InAConferenceCall`, `InAMeeting`, `Presenting`, `Focusing`, `BeRightBack`.

## Requirements

- macOS
- Microsoft Teams desktop app installed and signed in
- Luxafor device connected (e.g. Luxafor Flag)
- Node.js 18+

## Install

```bash
git clone <YOUR_REPO_URL>
cd teams-luxafor-presence-sync
chmod +x install.sh uninstall.sh
./install.sh
```

What `install.sh` does:
- Installs npm dependencies
- Generates a user-specific LaunchAgent plist from template
- Registers + starts `com.teams-luxafor-sync` at login
- Runs a first-run permissions preflight and opens Privacy settings if Node lacks log access

On first install, macOS may require **Full Disk Access** for your `node` binary to read Teams logs.
If prompted by `install.sh`, grant access and press Enter to continue.

## Verify It Is Running

```bash
launchctl print "gui/$(id -u)/com.teams-luxafor-sync" >/dev/null && echo "running"
tail -f ~/Library/Logs/teams-luxafor-sync.log
```

## Stop / Start

```bash
launchctl bootout "gui/$(id -u)/com.teams-luxafor-sync"
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.teams-luxafor-sync.plist
launchctl kickstart -k "gui/$(id -u)/com.teams-luxafor-sync"
```

## Uninstall

```bash
./uninstall.sh
```

## Optional Configuration

Set environment variables in the generated plist (`~/Library/LaunchAgents/com.teams-luxafor-sync.plist`):

- `POLL_MS` (default `3000`)
- `TAIL_BYTES` (default `262144`)
- `FALLBACK_LOG_SCAN_COUNT` (default `5`)
- `TEAMS_LOG_DIR` (manual override if Teams logs are in a non-standard path)
- `REAPPLY_MS` (default `15000`, force-sync interval if lamp was manually changed)

After changes:

```bash
launchctl bootout "gui/$(id -u)/com.teams-luxafor-sync"
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.teams-luxafor-sync.plist
launchctl kickstart -k "gui/$(id -u)/com.teams-luxafor-sync"
```

## Troubleshooting

1. No color changes:
- Ensure Teams is running and logged in.
- Confirm log output: `tail -f ~/Library/Logs/teams-luxafor-sync.log`
- Temporarily run foreground mode:
  ```bash
  npm start
  ```

2. Device not found / Luxafor error:
- Replug the Luxafor USB device.
- Close any other app controlling Luxafor.

3. Node path changed after Node upgrade:
- Run `./install.sh` again to regenerate plist with current Node path.
