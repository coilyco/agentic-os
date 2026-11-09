// COI-3517: a seat that starts waiting makes no sound, even with Sound on.
import { expect, SEATS, test } from "../harness.ts";

test("another seat asking plays no chime, whatever the Sound switch says", async ({ bed }) => {
  await bed.page.addInitScript(() => {
    localStorage.setItem("aterm.attention.v1", JSON.stringify({ sound: true, visual: true }));
    const w = window as unknown as { __tones: number };
    w.__tones = 0;
    const real = AudioContext.prototype.createOscillator;
    AudioContext.prototype.createOscillator = function (this: AudioContext) {
      w.__tones += 1;
      return real.call(this);
    };
  });
  await bed.openSeat(SEATS.imp);
  await bed.page.waitForTimeout(2200);
  bed.mock.push({
    type: "ask",
    ask: { id: "quiet-1", session: SEATS.beetle.id, header: "Scope", question: "Ship it?", options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: false, multi: false },
  });
  await bed.page.waitForTimeout(1200);
  const tones = await bed.page.evaluate(() => (window as unknown as { __tones: number }).__tones);
  expect(tones, "oscillators started for a waiting seat").toBe(0);
});
