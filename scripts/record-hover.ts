// Build-time only: render the original HUD's hover pose and greeting into
// transparent frames. The desktop binary decodes the atlas, not JavaScript.
import { makeForm } from "./bots/bot-form";
import { BotMotion } from "./bots/bot-motion";
import { renderBot } from "./bots/bot-renderer";
import { botState, type BotLook } from "./bots/bot-state";
import { SplitMix } from "./bots/bot-pace";

const marks = document.images;
const query = new URLSearchParams(location.search);
const tool = query.get("tool") as BotLook | null;
const cue = query.get("cue") as BotLook | null;
const compact = tool || cue;
const cell = compact ? 64 : 80, side = compact ? 43 : 54, count = 96, columns = 8;
const idleFPS = 16, idleCount = 88;
const asleep = query.has("sleep");
const idle = query.has("idle") || asleep;
const work = query.has("work");
const workFPS = 6, workCount = 104; // contiguous entrance + first steady cycle; final tile is held thinking
const toolFPS = 8, toolCount = 40; // 4.5s: 3s of beats, 1.5s of rest
const doneCount = 40;
const keys = compact || asleep ? ["openai", "antigravity", "deepseek", "claude"] : query.has("claude") ? ["claude"] : ["openai", "antigravity", "deepseek"];
const wait = cue === "question" || cue === "permission";
const frames = wait ? 40 : cue ? doneCount : tool ? toolCount : work ? workCount : idle ? idleCount : count;
const sheet = document.createElement("canvas");
sheet.width = columns * cell;
sheet.height = keys.length * frames / columns * cell;
const ctx = sheet.getContext("2d")!;

async function record() {
  for (const [slot, key] of keys.entries()) {
    const image = marks[key === "claude" ? 3 : slot];
    if (!image.naturalWidth) throw new Error("Unreadable asset for " + key);
    const source = document.createElement("canvas");
    source.width = source.height = 256;
    const ink = source.getContext("2d")!;
    ink.drawImage(image, 0, 0, 256, 256);
    if (key === "openai") {
      ink.globalCompositeOperation = "source-in";
      ink.fillStyle = "#10a37f";
      ink.fillRect(0, 0, 256, 256);
    }
    const form = makeForm(key, source, false)!;
    const motion = new BotMotion();
    motion.random = new SplitMix(1);
    motion.fit(form.spec);
    motion.set(botState({look: asleep ? "asleep" : "idle"}), 0);
    motion.step(0);
    let start = 0, frameRate = 24, lastSample = 0;
    if (cue) {
      frameRate = cue === "done" ? 16 : 12;
      start = frameRate;
      motion.set(botState({look: "thinking"}), 0);
      for (let i = 1; i <= start; i++) motion.step(i / frameRate);
      motion.set(botState({look: cue}), start / frameRate);
      lastSample = start / frameRate;
    } else if (work || tool) {
      motion.set(botState({look: tool ?? "thinking"}), 0);
      frameRate = tool ? toolFPS : workFPS;
      // The loop starts after the one-time entrance. Align to the original
      // movement period so the join preserves gaze/breath/beat phase.
      start = tool ? frameRate * 4.5 : 0;
    } else if (idle) {
      // Run the upstream visit state machine until the first real movement,
      // respecting its frame clock instead of approximating the motion curves.
      frameRate = idleFPS;
      for (let i = 1; i <= (asleep ? 301 : 36) * idleFPS; i++) {
        motion.step(i / idleFPS);
        if (motion.morph !== 0) { start = i; break; }
      }
      if (!start) throw new Error("No upstream idle visit for " + key);
      lastSample = start / frameRate;
    } else {
      motion.set(botState({look: "idle", hovered: true}), 0);
    }
    const scratch = document.createElement("canvas");
    for (let i = 0; i < frames; i++) {
      // 24 entrance frames; the two following 8-frame groups sample the
      // alternating repeat movements at 3.8s and 6.4s from BotMoves.
      const age = wait && i >= 24
        ? (i < 32 ? 3.8 : 6.4) + ((i - 24) % 8) / frameRate
        : i / frameRate;
      const time = start / frameRate + age;
      if (work && i === frames - 1)
        motion.set(botState({look: "thinking", holds: true}), lastSample);
      for (let t = lastSample + 1 / 24; t < time - 0.001; t += 1 / 24) motion.step(t);
      motion.step(time);
      lastSample = time;
      renderBot(form, motion, side, 0, scratch);
      const {x, y, footing, rotation, scaleX, scaleY} = motion.pose;
      const left = i % columns * cell, top = (slot * frames / columns + Math.floor(i / columns)) * cell;
      ctx.save();
      ctx.beginPath();
      ctx.rect(left, top, cell, cell);
      ctx.clip();
      ctx.translate(left + cell / 2 + x * side, top + cell / 2 + (cue && cue !== "done" ? 5 : 0) + (y + footing) * side);
      ctx.rotate(rotation * Math.PI / 180);
      ctx.scale(scaleX, scaleY);
      ctx.translate(0, -footing * side);
      ctx.drawImage(scratch, -scratch.width / 2, -scratch.height / 2);
      ctx.restore();
    }
  }
  document.body.textContent = sheet.toDataURL("image/png");
}
window.addEventListener("load", () => {
  record().catch(error => { document.body.textContent = String(error.stack ?? error); });
});
