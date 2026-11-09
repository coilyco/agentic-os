// COI-3517: the coordinator seat's ping sounds and raises a notice, once per id.
import { expect, SEATS, test } from "../harness.ts";

const ping = (id: string) => ({
  type: "notify",
  notify: { id, session: SEATS.griffin.id, role: "prod-director", identity: "Griffin-Goose", title: "Decision needed", body: "Pick the order", at: new Date().toISOString() },
});

test("a notify frame plays the chime and shows a notice, and the same id does it once", async ({ bed }) => {
  await bed.page.addInitScript(() => {
    const w = window as unknown as { __tones: number; __notes: { title: string; options: { body: string; tag: string } }[]; Notification: unknown };
    w.__tones = 0;
    w.__notes = [];
    const real = AudioContext.prototype.createOscillator;
    AudioContext.prototype.createOscillator = function (this: AudioContext) {
      w.__tones += 1;
      return real.call(this);
    };
    w.Notification = class {
      static permission = "granted";
      static requestPermission = async () => "granted";
      constructor(title: string, options: { body: string; tag: string }) {
        w.__notes.push({ title, options });
      }
    };
  });
  await bed.openSeat(SEATS.imp);
  await bed.page.mouse.click(5, 5);
  await bed.page.waitForTimeout(1800);
  const read = () => bed.page.evaluate(() => ({ tones: (window as unknown as { __tones: number }).__tones, notes: (window as unknown as { __notes: { title: string; options: { body: string; tag: string } }[] }).__notes }));

  expect((await read()).tones, "quiet before any ping").toBe(0);
  bed.mock.push(ping("n1"));
  await expect.poll(async () => (await read()).tones, "the chime played").toBeGreaterThan(0);
  const first = await read();
  expect(first.notes, "one notice").toHaveLength(1);
  expect(first.notes[0]).toEqual({ title: "Decision needed", options: expect.objectContaining({ body: "Pick the order", tag: `aterm-${SEATS.griffin.id}` }) });

  bed.mock.push(ping("n1"));
  await bed.page.waitForTimeout(800);
  const again = await read();
  expect(again.tones, "the same id does not chime again").toBe(first.tones);
  expect(again.notes, "or notify again").toHaveLength(1);

  bed.mock.push(ping("n2"));
  await expect.poll(async () => (await read()).notes.length, "a new id does").toBe(2);
});
