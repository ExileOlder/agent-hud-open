Original Agent HUD provider marks from agent-hud-open at v0.6.15.
Codex uses chatgpt.png (tinted as in upstream), Antigravity uses antigravity.png,
and the Deepseek slot uses deepseek.png; its quota comes from Mistral Vibe.
The optional Claude Code slot uses the original claude.png and its separately loaded animation sheets.
The marks retain their respective owners' branding; this is not an endorsement.

The build-time atlas scaler uses golang.org/x/image (see
LICENSE-Go-image-font.txt). HUD text uses native system fonts, as upstream
does; the binary does not embed a font.

`hover.atlas`, `idle.atlas`, and `work.atlas` contain 24-fps hover, 16-fps idle,
and 6-fps thinking frames rendered with the original HUD’s BotMotion and
renderBot (Apache-2.0). Claude Code uses separate atlases from the same recorder. To regenerate, point
`ESBUILD=esbuild BROWSER=chromium ./scripts/record-hover.sh /path/to/upstream/src`
at the Agent HUD source containing `packages/hud/src/bots`. The recorder
scales each frame to the native dock’s 58 px grid. The build trims transparent
borders and stores identical crops once; a frame index restores their original
positions before the native app draws them. Each provider is compressed
and decoded separately. The two Antigravity positions share one image cache. The native app
plays the hover greeting once, keeps the character awake, then eases back to
the logo from the pose actually shown when the pointer leaves. Each idle visit uses its own 20–35 second
wait, as in the original. Thinking plays a contiguous entrance into the 15-second gaze, breath,
and ellipsis period; subsequent cycles skip the entrance. A tool cycle keeps the original 3-second burst and
1.5-second rest.
The compact peek is a native adaptation of the Simple dock's AgentPeek layout.

The `tool.atlas` atlas samples the same Apache-2.0 procedural renderer used above.
In upstream that renderer shares its work motion across tool kinds. The separate
Anthropic Clawd desktop sprites are not included in this distribution.
