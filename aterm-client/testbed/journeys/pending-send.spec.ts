// COI-3690: a sent message shows in the transcript on the next frame, marked pending, and
// becomes the real entry with no copy beside it. One the seat never takes says so.
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const history = [
  { id: "p1", kind: "prompt", text: "Why is the stripe wrong?", at: "2026-11-10T21:00:00Z" },
  { id: "r1", kind: "reply", text: "A folded row breaks it.", complete: true, at: "2026-11-10T21:00:02Z" },
];
const sendingRow = (bed: Bed) => bed.page.locator('[data-row^="sending-"]');
const mine = (bed: Bed, words: string) => bed.page.locator(".transcript .turn.me").filter({ hasText: words });

async function openTranscript(bed: Bed): Promise<void> {
  bed.mock.setTranscript(imp, history);
  await bed.openSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await bed.page.getByRole("radio", { name: "Focus" }).click();
  await expect(bed.page.locator(".transcript")).toContainText("A folded row breaks it.");
}

/** Presses Send in the page and says if the entry is on screen by the next frame. */
const sendAndLook = (bed: Bed, words: string) =>
  bed.page.evaluate(async (typed) => {
    const field = document.querySelector<HTMLTextAreaElement>("#composer-input")!;
    field.value = typed;
    field.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise((done) => setTimeout(done, 50));
    const start = performance.now();
    document.querySelector<HTMLButtonElement>("form.composer button.primary")!.click();
    return new Promise<{ shown: boolean; ms: number }>((done) =>
      requestAnimationFrame(() => done({ shown: document.querySelector('[data-row^="sending-"]')?.textContent?.includes(typed) === true, ms: performance.now() - start })),
    );
  }, words);

test("a sent message is on screen by the next frame, then is the real entry with no copy", async ({ bed }) => {
  await openTranscript(bed);
  const seen = await sendAndLook(bed, "ship the stripe fix");
  expect(seen.shown, "the message was not in the transcript one frame after Send").toBe(true);
  await expect(sendingRow(bed)).toContainText("Sending.");
  await bed.step("pending", { compare: false });

  // The harness writes it. The next read carries it, and the entry goes with it.
  bed.mock.appendTranscript(imp, [{ id: "p2", kind: "prompt", text: "ship the stripe fix", at: new Date().toISOString() }]);
  await expect(sendingRow(bed), "the pending entry stayed after the real one came").toHaveCount(0, { timeout: 6000 });
  await expect(mine(bed, "ship the stripe fix"), "the message shows once").toHaveCount(1);
  await expect(bed.page.locator('[data-row="p2"]')).toHaveCount(1);
});

test("a message the seat never takes stays visibly unsent, and a late prompt still settles it", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendAndLook(bed, "did you get this");
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "pending");

  await bed.page.clock.fastForward(31_000);
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "unsent");
  await expect(sendingRow(bed).getByRole("alert")).toContainText("Not in the transcript yet");
  await expect(sendingRow(bed).getByRole("button", { name: "Dismiss" })).toBeVisible();
  await bed.step("unsent", { compare: false });

  bed.mock.appendTranscript(imp, [{ id: "p3", kind: "prompt", text: "did you get this", at: new Date().toISOString() }]);
  await bed.page.clock.runFor(3000);
  await expect(sendingRow(bed)).toHaveCount(0);
  await expect(mine(bed, "did you get this")).toHaveCount(1);
});

test("an unsent entry can be dismissed", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendAndLook(bed, "swallowed by a dialog");
  await bed.page.clock.fastForward(31_000);
  await sendingRow(bed).getByRole("button", { name: "Dismiss" }).click();
  await expect(sendingRow(bed)).toHaveCount(0);
});
