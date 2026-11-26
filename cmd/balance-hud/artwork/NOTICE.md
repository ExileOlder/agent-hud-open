Original Agent HUD provider marks from agent-hud-open at v0.6.15.
Codex uses chatgpt.png (tinted as in upstream), Antigravity uses antigravity.png,
and the Mistral slot uses deepseek.png to match the previous local HUD's Vibe display.
The optional Claude Code slot uses the original claude.png and its separately loaded animation sheets.
The marks retain their respective owners' branding; this is not an endorsement.

Go Regular and the image/font library are from golang.org/x/image (see
LICENSE-Go-image-font.txt). Only these visual assets are included; the original
chat and remote components are not required.

`hover.png` and `idle.png` contain 25-fps hover and 16-fps idle frames
generated with the original HUD's BotMotion and renderBot (Apache-2.0). To regenerate, point
`ESBUILD=esbuild BROWSER=chromium ./scripts/record-hover.sh /path/to/upstream/src`
at the Agent HUD source containing `packages/hud/src/bots`. The native app
plays the hover sequence once and holds its final frame until the pointer leaves.
An idle visit plays independently on each mark every few dozen seconds.
The compact peek is a native adaptation of the Simple dock's AgentPeek layout.
