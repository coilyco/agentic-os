// COI-3646: a seat whose view is Focus opens in Focus on a cold start, with no switching.
import { expect, reopenSeat, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const transcript = [
  { id: "p1", kind: "prompt", text: "Why does the wrapped row hide the stripe?" },
  { id: "r1", kind: "reply", text: "The stripe was placed by row number.", complete: true },
];
const focus = (bed: Bed) => bed.page.getByRole("radio", { name: "Focus" });
const feed = (bed: Bed) => bed.page.locator(".transcript");

// A stored view of Focus: the device default.
const storeFocus = (bed: Bed) =>
  bed.page.addInitScript(() => {
    localStorage.setItem("aterm.seat-view-default.v1", "focus");
  });

test("a cold start opens a Focus seat in Focus", async ({ bed }) => {
  await storeFocus(bed);
  bed.mock.setTranscript(imp, transcript);
  await bed.openSeat(SEATS.imp);
  await reopenSeat(bed.page, SEATS.imp);
  await expect(focus(bed)).toHaveAttribute("aria-checked", "true");
  await expect(feed(bed)).toContainText("The stripe was placed by row number.");
  await expect(bed.page.locator(".rawscreen")).toHaveAttribute("aria-hidden", "true");
});

test("a first read the host refuses for now does not strand the seat on its screen", async ({ bed }) => {
  await storeFocus(bed);
  bed.mock.setTranscript(imp, transcript);
  bed.mock.refuseTranscript(imp, "passkey_required");
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.locator(".fallback")).toBeVisible();
  bed.mock.refuseTranscript(imp, null);
  await expect(feed(bed), "the host answers now, and Focus follows").toContainText("The stripe was placed by row number.", { timeout: 8000 });
  await expect(bed.page.locator(".fallback")).toHaveCount(0);
});

test("a transcript the host cannot read stays on the screen and is not asked again", async ({ bed }) => {
  await storeFocus(bed);
  bed.mock.refuseTranscript(imp, "unreadable_format");
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.locator(".fallback")).toBeVisible();
  const asked = bed.mock.transcriptCalls.length;
  await bed.page.waitForTimeout(6500);
  expect(bed.mock.transcriptCalls.length, "no read after the first").toBe(asked);
  await expect(feed(bed)).toHaveCount(0);
});
