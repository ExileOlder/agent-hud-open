# Balance HUD

A quota-focused rewrite of [Agent HUD](https://github.com/jazzenchen/agent-hud-open) for Ubuntu/X11 and Windows x64. It has no WebView, tray framework, session indexing, agent messaging, remote access or telemetry.

## Get the portable build

Open a successful [Native builds and tests](https://github.com/ExileOlder/agent-hud-open/actions/workflows/check.yml?branch=codex/balance-hud) run and download the artifact for your system. Extract the archive inside it: `BalanceHUD-windows-x64.zip` on Windows x64, or `tar -xf BalanceHUD-ubuntu-x64.tar.xz` on Ubuntu. Run `BalanceHUD.exe` on Windows or `./balance-hud` from an Ubuntu/X11 session. Keep the Antigravity authorization launcher beside the HUD executable. On Ubuntu, the application links to X11, XRandR, Xext, Xft and SQLite; on Ubuntu 22.04 their runtime packages are `libx11-6 libxrandr2 libxext6 libxft2 libsqlite3-0`. Connect the account sources below after extraction.

The archive contains no local settings, API keys or authorized accounts.

## Build

Use Go 1.24 or later.

- **Ubuntu/X11**: install the C compiler, pkg-config, libx11-dev, libxrandr-dev, libxext-dev, libxft-dev and libsqlite3-dev. Run `CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o build/balance-hud ./cmd/balance-hud`.
- **Windows x64**: run `go build -trimpath -ldflags="-s -w -H windowsgui" -o build/balance-hud.exe ./cmd/balance-hud` on Windows, or set `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` to cross-compile from Linux. This build requires native testing on Windows.
- Run `go test ./...` for quota, display and notification checks. On Linux, run `python3 scripts/smoke-x11.py /path/to/balance-hud` with Xvfb, X11 utilities and libXtst installed to exercise the actual packaged desktop. This uses a private display and temporary configuration; your open HUD and accounts are untouched.
- Run `bash scripts/package.sh` to build the portable archive, including the Antigravity helper. On Ubuntu, set `BALANCE_HUD_LINUX_ARCHIVE=xz` to match CI; leave it unset for the gzip format. This requires Git and Python in addition to the build dependencies above. Windows portable packaging uses Visual Studio C++ Build Tools and the Windows SDK; cross-packaging from Linux uses an x64 MinGW compiler (`WINDOWS_MONITOR_CC` can override its command).

## Connect quota sources

- **Codex:** sign in to the Codex CLI with your ChatGPT subscription. The HUD invokes the existing app-server for rate limits without sending a prompt. If `codex` is not on PATH, pass `-codex /path/to/codex` (on Windows, use the native `.exe`).
- **Antigravity:** the portable build reads quota through its built-in monitor. Use the included `agy-auth-monitor` command or **Settings → Accounts** to authorize. For source builds, build the standalone helper with `scripts/build-monitor.sh` on Ubuntu, or `GOOS=windows GOARCH=amd64 scripts/build-monitor.sh` for Windows. Put it alongside the HUD binary or pass its path through `-antigravity`. The monitor has its own account store; authorization leaves the active clients unchanged.
- **Mistral Vibe**: use the OS credential store. On Ubuntu install `libsecret-tools` and `zenity`, then run `./build/balance-hud -setup-mistral` and enter the key in the masked prompt. On Windows open **Credential Manager → Windows Credentials → Add a generic credential**. Set the address to `BalanceHUD:MistralVibe`, the user name to `mistral-vibe`, and the password to your Vibe key. The process environment variable `MISTRAL_VIBE_API_KEY` is also supported for managed launches.

- **Claude Code (optional):** in **Settings → Accounts**, turn on **Claude Code · show in HUD**. Use **Sign in** there if needed, then refresh. Balance HUD asks the official Claude Code CLI for subscription usage; it sends no prompt and handles no Claude OAuth token. The Claude row stays hidden by default. Use `-claude /path/to/claude` if the official CLI is installed outside PATH.

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

Reopening Balance HUD on the same desktop leaves the existing float and inbox in place. The four default positions are Codex, Antigravity 1, Antigravity 2 and Deepseek. Claude Code can be added as the fifth position. Hover for the original Simple dock’s one-line readings (Deepseek shows the balance, total and remaining percentage of the Mistral subscription together); drag with the primary button: the outlined strip follows the pointer and docks to the nearest edge when released. Each Antigravity ring shows the lowest remaining quota for that account; the hover line still lists each window. Hover and click the **gear** to open the separate **Balance HUD Settings** window. **Display** controls placement, percentages, motion, and refresh. With motion on, the quota pill slides out and fades in on hover, then slides back and fades out. In **Accounts**, authorize Antigravity accounts, then click either position to cycle through authorized accounts. Selecting the other position’s account swaps the two. This changes the HUD only; it does not switch your active Antigravity client. Use the Codex and Deepseek rows to open their sign-in or credential setup, then refresh balances. Right-click the strip to toggle numbers, middle-click to refresh, and Shift-right-click to quit. A missing reading shows `--`. If a refresh fails, the last reading for the same account is shown with a muted ring and an asterisk when numbers are enabled. The default refresh interval is five minutes. Each provider's reading appears as soon as it arrives, while other readings refresh independently. Appearance is stored in the OS user configuration directory.

Use `-desktop=false` for a one-shot JSON read from a terminal build. This output includes account labels; review it before sharing. Test builds and current credentials are not part of the repository.

Licensed under Apache-2.0; the upstream license is kept in LICENSE.

## Local notifications

With the HUD open, new Codex turns that finish and pending Codex Desktop question
cards send a paper plane to the message badge at the end of the dock. Hover or
click the badge to open the message list. Use a row’s **×** to dismiss it, or
**全部清空** to clear the list. With animation enabled, the source
mark shows a question, approval, or completion gesture when that event arrives.
Pending questions and approvals observed through a live client state keep
their mark visible with intermittent movement until that state clears. Clearing the inbox does
not dismiss a client's outstanding request. Another still-running turn keeps
its working expression on completion. The 32 newest uncleared notices are
kept in a small local file beside appearance settings and restored without
replaying their paper flights. Only notice types and routing/deduplication
IDs are stored; clearing the inbox removes the file.
Balance HUD reads event types and IDs
from Codex's local history projection and shows only a short status.
Double-click a Codex notice to return to its conversation. Double-click a floating
mark to open its client. If DeepSeek Harness is installed, the fourth mark
opens Harness while continuing to show the Mistral Vibe subscription balance.
Otherwise the fourth mark opens Mistral Vibe on the web. Linux uses the platform's SQLite library, and Windows uses
its inbox WinSQLite3 library. If the Codex version has no compatible history
projection, CLI completion hooks are still available.

Completion hooks from Antigravity, Mistral Vibe, and Claude Code land in the
same message list. Double-click a message to open its client. Keep Balance HUD
running to receive hook messages.

With DeepSeek Harness 0.2's local projection cache, working turns and pending
questions reach the HUD directly. Approvals are detected from unmatched request
IDs within the active turn's flushed local log. The HUD displays a short status;
the reason and response stay in Harness. The approval pose clears when the
decision or turn end is flushed. The same flushed log drives the compaction
pose, including standalone manual compaction. For completed turns the HUD verifies the
outcome in the same log and only shows a completion notice for success. The watcher uses `DSH_HOME` or `~/.dsh`. The
Mistral quota source and the Harness task state are independent. On Linux the
mark uses the installed `deepseek-harness-tools.desktop` shortcut; Windows
uses the Harness’s `dsh://open` handler.

For automatic reminders, add the indicated hooks for the clients you use. **Merge** the entry into your existing configuration; don't replace other hooks. Use the absolute path to the installed executable, quoted for the client's shell, and authorize the new hook if the client requests it. The examples below show Ubuntu paths.

- **[Codex](https://developers.openai.com/codex/hooks), only for versions without history integration:** add the handler under `hooks.Stop` in `${CODEX_HOME:-$HOME/.codex}/hooks.json`. Start a new session and approve the hook if prompted. Double-click the completion reminder to open its conversation when the hook supplies a session ID.

  ```json
  {"hooks":{"Stop":[{"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed codex -hook","timeout":5}]}]}}
  ```

- **Codex approval reminders:** in Balance HUD, choose **Settings → Accounts →
  Codex · approval reminders → Set up**. This adds the `PermissionRequest` handler
  to your Codex user configuration and leaves other hooks in place. Then open
  **Review hooks** in Codex Desktop (or `/hooks` in the CLI) and trust the callback.
  To add it manually, merge the example below into `hooks.json`. Balance HUD posts one private
  status to the inbox with a link to the requesting conversation. The hook
  returns no decision and never supplies input to Codex. It covers requests
  delivered by `PermissionRequest`; other prompts still use their own event
  sources.

  ```json
  {"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed codex -hook","timeout":5}]}]}}
  ```

- **[Antigravity](https://antigravity.google/docs/hooks):** add the named entry to the client's global `~/.gemini/config/hooks.json`, or the corresponding config in each separate client home. The hook ignores an early stop while background tasks are still running. An authorized quota account alone does not produce task notifications; install the hook in each actual client profile you use.

  ```json
  {"balance-hud":{"Stop":[{"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed antigravity -hook","timeout":5}]}]}}
  ```

- **[Mistral Vibe CLI](https://docs.mistral.ai/vibe/code/cli/hooks):** merge the entries below into `~/.vibe/hooks.toml` (or the active Vibe home). Replace the old Balance HUD `turn_completed` entry to avoid duplicate reminders. The Vibe subscription provides quota; task events come from the CLI.

  ```toml
  [[hooks]]
  name = "balance-hud-work"
  type = "pre_llm"
  command = "'/path/to/balance-hud' -completed mistral -hook"
  timeout = 5

  [[hooks]]
  name = "balance-hud-tool"
  type = "pre_tool"
  command = "'/path/to/balance-hud' -completed mistral -hook"
  timeout = 5

  [[hooks]]
  name = "balance-hud-notices"
  type = "notification"
  match = "re:approval_requested|question_asked|turn_completed|turn_failed"
  command = "'/path/to/balance-hud' -completed mistral -hook"
  timeout = 5

  [[hooks]]
  name = "balance-hud-end"
  type = "session_end"
  command = "'/path/to/balance-hud' -completed mistral -hook"
  timeout = 5
  ```

- **[Claude Code](https://code.claude.com/docs/en/hooks):** in `${CLAUDE_CONFIG_DIR:-$HOME/.claude}/settings.json`, use the same command handler for `Stop`, `StopFailure`, `SessionEnd`, `UserPromptSubmit`, `PermissionRequest`, `PostToolUse`, and `PostToolUseFailure`. Add `PreCompact` and `PostCompact` to show the separate compaction pose. Add the handler under `PreToolUse` without a matcher to track tool activity and questions. Replace the earlier HUD-only `AskUserQuestion` handler if present. Retain one Balance HUD entry per event and leave other client hooks in place. Example group for `PreToolUse`:

  ```json
  {"hooks":[{"type":"command","command":"'/path/to/balance-hud' -completed claude -hook","timeout":5}]}
  ```

  Keep `Stop` for completion only if you do not want working animations or waiting reminders. Claude reports a finish only once background and scheduled work are clear.

### Match Antigravity activity to its quota account

For separate Antigravity profiles, create `BalanceHUD/activity-antigravity.json` in the OS user configuration directory. Each entry pairs the label displayed in the HUD’s Accounts page with the absolute path to that client profile’s `.gemini` directory. The association follows the account when you reorder positions. Restart Balance HUD after changing the file.

```json
[
  {"Account":"first@example.test","GeminiHome":"/home/me/.gemini"},
  {"Account":"second@example.test","GeminiHome":"/home/me/other-profile/.gemini"}
]
```

On Windows use the same fields with absolute Windows paths (JSON accepts forward slashes). Balance HUD reads lifecycle metadata for working animations. A running Antigravity version that publishes a local daemon registration also supplies pending approval and question state. The standard Gemini home is detected automatically; add the profile mapping above for additional homes. Those notices show only a short status and double-click opens Antigravity to respond there. For new requests from a shared daemon, the HUD verifies the account the running client reports. Old pending requests keep their original assignment when the client switches accounts. If a new request arrives without a verified account, the inbox still shows the notice without assigning it to either mark. Quota refresh and completion hooks continue to work without this mapping.

Other clients with a success-only hook can call `balance-hud -completed agent`. In Windows PowerShell commands use `& 'C:/path/to/BalanceHUD.exe' -completed codex -hook` (substitute the client name); in Git Bash use `'C:/path/to/BalanceHUD.exe'` without `&`. This program does not change agent configurations for you.

The marks use the original Agent HUD artwork. The original character renderer supplies the hover sequence, occasional idle visit, and thinking pose. The native HUD draws changing frames only while a sequence plays, without loading a WebView. Use **Turn off animation** for static marks. To regenerate the atlas from the matching upstream source, see [artwork notes](cmd/balance-hud/artwork/NOTICE.md).
