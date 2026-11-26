# Balance HUD

A quota-focused rewrite of [Agent HUD](https://github.com/jazzenchen/agent-hud-open) for Ubuntu/X11 and Windows x64. It has no WebView, tray framework, session indexing, agent messaging, remote access or telemetry.

## Build

Use Go 1.24 or later.

- **Ubuntu/X11**: install the C compiler, pkg-config, libx11-dev, libxrandr-dev, libxext-dev, libxft-dev and libsqlite3-dev. Run `CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o build/balance-hud ./cmd/balance-hud`.
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

The four positions are Codex, Antigravity 1, Antigravity 2 and Mistral Vibe. Hover for the original Simple dock’s one-line readings (Mistral shows balance, total and the remaining percentage together); drag with the primary button: the outlined strip follows the pointer and docks to the nearest edge when released. Each Antigravity ring shows the lowest remaining quota for that account; the hover line still lists each window. Hover and click the **gear** to open the separate **Balance HUD Settings** window. **Display** controls placement, percentages, motion, and refresh. With motion on, the quota pill slides out and fades in on hover, then slides back and fades out. In **Accounts**, authorize Antigravity accounts, then click either position to cycle through authorized accounts. Selecting the other position’s account swaps the two. This changes the HUD only; it does not switch your active Antigravity client. Use the Codex and Mistral rows to open their sign-in or credential setup, then refresh balances. Right-click the strip to toggle numbers, middle-click to refresh, and Shift-right-click to quit. A missing reading shows `--`. If a refresh fails, the last reading for the same account is shown with a muted ring and an asterisk when numbers are enabled. The default refresh interval is five minutes. Appearance is stored in the OS user configuration directory.

Use `-desktop=false` for a one-shot JSON read from a terminal build. This output includes account labels; review it before sharing. Test builds and current credentials are not part of the repository.

Licensed under Apache-2.0; the upstream license is kept in LICENSE.

## Local notifications

With the HUD open, new Codex turns that finish and pending Codex Desktop question
cards trigger a brief notice. Balance HUD reads event types and IDs from Codex's
local history projection. It does not show the answer, question, or option text.
Double-click a Codex notice to return to its conversation. Double-click a floating
mark to open its client via the system's registered handler; the Mistral Vibe mark
opens its web client. Linux uses the platform's SQLite library, and Windows uses
its inbox WinSQLite3 library. If the Codex version has no compatible history
projection, CLI completion hooks are still available.

Other agents can send a brief local completion reminder with the installed
`balance-hud -completed agent` command. On Ubuntu, hook-driven reminders use
`notify-send` (`libnotify-bin`); on Windows keep Balance HUD open to receive them.

For automatic reminders, add just the indicated completion hook to each client you use. **Merge** the entry into your existing configuration; don't replace other hooks. Use the absolute path to the installed executable, quoted for the client's shell, and authorize the new hook if the client requests it. The examples below show Ubuntu paths.

- **[Codex](https://developers.openai.com/codex/hooks), only for versions without history integration:** add the handler under `hooks.Stop` in `${CODEX_HOME:-$HOME/.codex}/hooks.json`. Start a new session and approve the hook if prompted.

  ```json
  {"hooks":{"Stop":[{"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed codex -hook","timeout":5}]}]}}
  ```

- **[Antigravity](https://antigravity.google/docs/hooks):** add the named entry to the client's global `~/.gemini/config/hooks.json`, or the corresponding config in each separate client home. The hook ignores an early stop while background tasks are still running. An authorized quota account alone does not produce task notifications; install the hook in each actual client profile you use.

  ```json
  {"balance-hud":{"Stop":[{"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed antigravity -hook","timeout":5}]}]}}
  ```

- **[Mistral Vibe CLI](https://docs.mistral.ai/vibe/code/cli/hooks):** add a `turn_completed` notification hook to `~/.vibe/hooks.toml` (or the active Vibe home). The Vibe subscription alone does not provide turn callbacks.

  ```toml
  [[hooks]]
  name = "balance-hud-completion"
  type = "notification"
  match = "turn_completed"
  command = "'/path/to/balance-hud' -completed mistral -hook"
  timeout = 5
  ```

Other clients with a success-only hook can call `balance-hud -completed agent`. In Windows PowerShell commands use `& 'C:/path/to/BalanceHUD.exe' -completed codex -hook` (substitute the client name); in Git Bash use `'C:/path/to/BalanceHUD.exe'` without `&`. This program does not change agent configurations for you.

The four marks use the original Agent HUD artwork. The original character renderer supplies both the hover sequence and the occasional idle visit. The native HUD draws changing frames only while a sequence plays, without loading a WebView. Use **Turn off animation** for static marks. To regenerate the atlas from the matching upstream source, see [artwork notes](cmd/balance-hud/artwork/NOTICE.md).
