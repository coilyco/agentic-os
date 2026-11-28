// COI-3705: a send of only pictures leaves a pending entry and a history entry with its
// images, so a swallowed one can be recalled and sent again.
import { expect, kindOf, reopenSeat, SEATS, test, type Bed, pickView } from "../harness.ts";

test.use({ mockOptions: { images: true, transcript: true } });

const imp = SEATS.imp.id;
const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
const field = (bed: Bed) => bed.page.locator("#composer-input");
const sendingRow = (bed: Bed) => bed.page.locator('[data-row^="sending-"]');
const chip = (bed: Bed) => bed.page.getByRole("button", { name: "Remove image 1" });

async function openTranscript(bed: Bed): Promise<void> {
  bed.mock.setTranscript(imp, [{ id: "p1", kind: "prompt", text: "an earlier message", at: "2026-11-10T21:00:00Z" }]);
  await bed.openSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await pickView(bed.page, "Focus");
  await expect(bed.page.locator(".transcript")).toContainText("an earlier message");
}

/** Attaches a picture, presses Send, and says if its entry is up within a few frames. */
async function sendPicture(bed: Bed): Promise<boolean> {
  await bed.page.locator('form.composer input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: PIXEL });
  await expect(chip(bed)).toBeVisible();
  return bed.page.evaluate(
    () =>
      new Promise<boolean>((done) => {
        document.querySelector<HTMLButtonElement>("form.composer button.primary")!.click();
        // The upload comes first, so the entry may be a few frames behind the press.
        const started = performance.now();
        const look = () => {
          const text = document.querySelector('[data-row^="sending-"]')?.textContent ?? "";
          if (text.includes("An image sent")) done(true);
          else if (performance.now() - started > 3000) done(false);
          else requestAnimationFrame(look);
        };
        requestAnimationFrame(look);
      }),
  );
}

test("a picture sent with no words is pending at once, resolves to the real entry, and is recalled with its image", async ({ bed }) => {
  await openTranscript(bed);
  expect(await sendPicture(bed), "no pending entry for a send of only a picture").toBe(true);
  await expect(chip(bed), "the picture stayed in the field after it was sent").toHaveCount(0);
  await bed.step("image-pending", { compare: false });

  bed.mock.appendTranscript(imp, [{ id: "p2", kind: "prompt", text: "[Image #1]", at: new Date().toISOString() }]);
  await expect(sendingRow(bed)).toHaveCount(0, { timeout: 6000 });
  await expect(bed.page.locator('[data-row="p2"]')).toHaveCount(1);

  // The seat did not take it: the picture is still in the history, and comes back.
  if (kindOf(test.info()) === "desktop") await field(bed).press("ArrowUp");
  else {
    await bed.page.getByRole("button", { name: "Recent sends" }).click();
    await expect(bed.page.getByRole("list", { name: "Recent sends" })).toContainText("1 image");
    await bed.page.getByRole("list", { name: "Recent sends" }).getByRole("button").first().click();
  }
  await expect(chip(bed), "the picture did not come back").toBeVisible();
  await expect(field(bed)).toHaveValue("");
  await expect(bed.page.locator("form.composer button.primary"), "Send is off with a picture to send").toBeEnabled();
  await bed.step("image-recalled", { compare: false });
});

test("the picture is still in the history after the app is reopened", async ({ bed }) => {
  await openTranscript(bed);
  await sendPicture(bed);
  await expect.poll(() => bed.mock.uploads.length).toBe(1);
  await reopenSeat(bed.page, SEATS.imp);
  if (kindOf(test.info()) === "desktop") await field(bed).press("ArrowUp");
  else {
    await bed.page.getByRole("button", { name: "Recent sends" }).click();
    await bed.page.getByRole("list", { name: "Recent sends" }).getByRole("button").first().click();
  }
  await expect(chip(bed)).toBeVisible();
});

test("a picture the seat never takes says so after the timeout", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendPicture(bed);
  // A late redraw reads as work, so the clock steps on until it settles (COI-3818).
  await expect.poll(() => bed.mock.idleMs(), { timeout: 10_000 }).toBeGreaterThan(500);
  await bed.page.clock.fastForward(31_000);
  await expect.poll(async () => (await bed.page.clock.fastForward(2_000), sendingRow(bed).getAttribute("data-state")), { timeout: 15_000 }).toBe("unsent");
  await expect(sendingRow(bed)).toContainText("An image sent");
  await expect(sendingRow(bed).getByRole("alert")).toContainText("Not in the transcript yet");
});

test("walking back to the draft brings back the draft's own picture", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "desktop", "arrow keys");
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("an earlier send");
  await bed.page.locator("form.composer button.primary").click();
  await expect(field(bed)).toHaveValue("");
  await bed.page.locator('form.composer input[type="file"]').setInputFiles({ name: "draft.png", mimeType: "image/png", buffer: PIXEL });
  await expect(chip(bed)).toBeVisible();
  await field(bed).press("ArrowUp");
  await expect(field(bed)).toHaveValue("an earlier send");
  await expect(chip(bed), "the draft's picture rode along with a recalled send").toHaveCount(0);
  await field(bed).press("ArrowDown");
  await expect(field(bed)).toHaveValue("");
  await expect(chip(bed), "the draft's picture was lost").toBeVisible();
});
