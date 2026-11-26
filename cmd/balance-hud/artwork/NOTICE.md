Original Agent HUD provider marks from agent-hud-open at v0.6.15.
Codex uses chatgpt.png (tinted as in upstream), Antigravity uses antigravity.png,
and the Mistral slot uses deepseek.png to match the previous local HUD's Vibe display.
The optional Claude Code slot uses the original claude.png and its separately loaded animation sheets.
The marks retain their respective owners' branding; this is not an endorsement.

The build-time atlas scaler uses golang.org/x/image (see
LICENSE-Go-image-font.txt). HUD text uses native system fonts, as upstream
does; the binary does not embed a font.

`hover.png`, `idle.png`, and `work.png` contain 25-fps hover, 16-fps idle,
and 12-fps thinking frames rendered with the original HUD’s BotMotion and
renderBot (Apache-2.0). Claude Code uses separate atlases from the same recorder. To regenerate, point
`ESBUILD=esbuild BROWSER=chromium ./scripts/record-hover.sh /path/to/upstream/src`
at the Agent HUD source containing `packages/hud/src/bots`. The recorder
scales each frame to the native dock’s 58 px grid before encoding. The native app
plays the hover sequence once and holds its final frame until the pointer leaves.
An idle visit plays independently on each mark every few dozen seconds.
The compact peek is a native adaptation of the Simple dock's AgentPeek layout.

The `tool.png` atlas samples the same Apache-2.0 procedural renderer used above.
In upstream that renderer shares its work motion across tool kinds. The separate
Anthropic Clawd desktop sprites are not included in this distribution.
