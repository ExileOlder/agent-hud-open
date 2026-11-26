// Build-time only: render the original HUD's hover pose and greeting into
// transparent frames. The desktop binary decodes the atlas, not JavaScript.
import { makeForm } from "./bots/bot-form";
import { BotMotion } from "./bots/bot-motion";
import { renderBot } from "./bots/bot-renderer";
import { botState } from "./bots/bot-state";
import { SplitMix } from "./bots/bot-pace";

declare const MARKS: string[];
const cell = 80, side = 54, count = 40, columns = 8;
const sheet = document.createElement("canvas");
sheet.width = columns * cell;
sheet.height = 3 * count / columns * cell;
const ctx = sheet.getContext("2d")!;

async function record() {
  for (const [slot, key] of ["openai", "antigravity", "deepseek"].entries()) {
    const image = new Image();
    image.src = MARKS[slot];
    await image.decode();
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
    motion.set(botState({look: "idle", hovered: true}), 0);
    const scratch = document.createElement("canvas");
    for (let i = 0; i < count; i++) {
      motion.step(i / 25);
      renderBot(form, motion, side, 0, scratch);
      const {x, y, footing, rotation, scaleX, scaleY} = motion.pose;
      const left = i % columns * cell, top = (slot * count / columns + Math.floor(i / columns)) * cell;
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
record().catch(error => { document.body.textContent = String(error.stack ?? error); });
