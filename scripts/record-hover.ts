// Build-time only: render the original HUD's hover pose and greeting into
// transparent frames. The desktop binary decodes the atlas, not JavaScript.
import { makeForm } from "./bots/bot-form";
import { BotMotion } from "./bots/bot-motion";
import { renderBot } from "./bots/bot-renderer";
import { botState } from "./bots/bot-state";
import { SplitMix } from "./bots/bot-pace";

const marks = document.images;
const cell = 80, side = 54, count = 40, columns = 8;
const idleFPS = 16, idleCount = 88;
const query = new URLSearchParams(location.search);
const idle = query.has("idle");
const work = query.has("work");
const workFPS = 12, workCount = 48;
const keys = query.has("claude") ? ["claude"] : ["openai", "antigravity", "deepseek"];
const frames = work ? workCount : idle ? idleCount : count;
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
    motion.set(botState({look: "idle"}), 0);
    motion.step(0);
    let start = 0, frameRate = 25;
    if (work) {
      motion.set(botState({look: "thinking"}), 0);
      frameRate = workFPS;
      start = workFPS;
      for (let i = 1; i < start; i++) motion.step(i / frameRate);
    } else if (idle) {
      // Run the upstream visit state machine until the first real movement,
      // respecting its frame clock instead of approximating the motion curves.
      frameRate = idleFPS;
      for (let i = 1; i <= 36 * idleFPS; i++) {
        motion.step(i / idleFPS);
        if (motion.morph !== 0) { start = i; break; }
      }
      if (!start) throw new Error("No upstream idle visit for " + key);
    } else {
      motion.set(botState({look: "idle", hovered: true}), 0);
    }
    const scratch = document.createElement("canvas");
    for (let i = 0; i < frames; i++) {
      motion.step((start + i) / frameRate);
      renderBot(form, motion, side, 0, scratch);
      const {x, y, footing, rotation, scaleX, scaleY} = motion.pose;
      const left = i % columns * cell, top = (slot * frames / columns + Math.floor(i / columns)) * cell;
      ctx.save();
      ctx.beginPath();
      ctx.rect(left, top, cell, cell);
      ctx.clip();
      ctx.translate(left + cell / 2 + x * side, top + cell / 2 + (y + footing) * side);
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
