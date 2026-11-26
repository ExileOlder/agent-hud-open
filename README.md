# Balance HUD

A quota-only rewrite of [Agent HUD](https://github.com/jazzenchen/agent-hud-open) for Ubuntu/X11 and Windows x64. It has no WebView, tray framework, session indexing, agent messaging, remote access or telemetry.

## Build

Use Go 1.24 or later.

- **Ubuntu/X11**: install the C compiler, pkg-config, libx11-dev and libxrandr-dev. Run `CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o build/balance-hud ./cmd/balance-hud`.
- **Windows x64**: run `go build -trimpath -ldflags="-s -w -H windowsgui" -o build/balance-hud.exe ./cmd/balance-hud` on Windows, or set `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` to cross-compile from Linux. This build requires native testing on Windows.
- Run `go test ./...` to test quota decoding and display projection. Tests use fictional accounts.

## Connect quota sources

- **Codex:** sign in to the Codex CLI with your ChatGPT subscription. The HUD invokes the existing app-server for rate limits without sending a prompt. If `codex` is not on PATH, pass `-codex /path/to/codex` (on Windows, use the native `.exe`).
- **Antigravity:** build the separate helper with `scripts/build-monitor.sh` on Ubuntu, or `GOOS=windows GOARCH=amd64 scripts/build-monitor.sh` for Windows. Keep its MIT license with it. Put the helper alongside the HUD binary or pass its path through `-antigravity`. See authorization below.
- **Mistral Vibe**: use the OS credential store. On Ubuntu install `libsecret-tools` and `zenity`, then run `./build/balance-hud -setup-mistral` and enter the key in the masked prompt. On Windows open **Credential Manager → Windows Credentials → Add a generic credential**. Set the address to `BalanceHUD:MistralVibe`, the user name to `mistral-vibe`, and the password to your Vibe key. The process environment variable `MISTRAL_VIBE_API_KEY` is also supported for managed launches.

### Authorize Antigravity twice

Keep the store location the same for the helper and HUD. Authorization only saves a background monitoring account; it leaves the active Antigravity client untouched.

Ubuntu (run in a terminal, with `agy` on PATH):

```sh
export AGY_AUTH_HOME="${XDG_CONFIG_HOME:-$HOME/.config}/BalanceHUD/Antigravity"
./build/linux-amd64/agy-auth-monitor login --monitor-only --alias account1
./build/linux-amd64/agy-auth-monitor login --monitor-only --alias account2
```

Windows PowerShell (with `agy.exe` on PATH):

```powershell
$env:AGY_AUTH_HOME = Join-Path $env:APPDATA 'BalanceHUD\Antigravity'
.\agy-auth-monitor.exe login --monitor-only --alias account1
.\agy-auth-monitor.exe login --monitor-only --alias account2
```

Use a different account on the second browser authorization. If the CLI is outside PATH, set `AGY_AUTH_AGY_PATH` to its executable before authorization and refresh. Existing Linux installations may use their current monitor and authorized store via explicit `-antigravity` and `AGY_AUTH_HOME`.

## Use the float

The four positions are Codex, Antigravity 1, Antigravity 2 and Mistral Vibe. Hover for details; drag with the primary button to dock to an edge of the primary display. Right-click to toggle numbers, middle-click to refresh, and Shift-right-click to quit. A missing reading shows `--`. If a refresh fails, the last reading for the same account is shown with a muted ring and an asterisk when numbers are enabled. The default refresh interval is five minutes. Appearance is stored in the OS user configuration directory.

Use `-desktop=false` for a one-shot JSON read from a terminal build. This output includes account labels; review it before sharing. Test builds and current credentials are not part of the repository.

Licensed under Apache-2.0; the upstream license is kept in LICENSE.
