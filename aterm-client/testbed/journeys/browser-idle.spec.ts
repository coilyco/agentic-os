// COI-3823: Chromium casts only what repaints, so a still page sends no frames,
// and an idle live page must never read as a stalled stream. COI-3825: a page that
// leaves the daemon's probe unanswered does, and the strip clears when it answers.
import { expect, SEATS, test, type Bed } from "../harness.ts";

// A 4x4 JPEG, the shape of a real `browser_frame`'s data.
const JPEG = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAoHBwgHBgoICAgLCgoLDhgQDg0NDh0VFhEYIx8lJCIfIiEmKzcvJik0KSEiMEExNDk7Pj4+JS5ESUM8SDc9Pjv/2wBDAQoLCw4NDhwQEBw7KCIoOzs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozv/wAARCAAEAAQDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAf/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFAEBAAAAAAAAAAAAAAAAAAAAAv/EABQRAQAAAAAAAAAAAAAAAAAAAAD/2gAMAwEAAhEDEQA/AK0ABP/Z";
const metadata = { offset_top: 0, page_scale_factor: 1, device_width: 390, device_height: 844, scroll_offset_x: 0, scroll_offset_y: 0, timestamp: 0 };

/** The seat's Browser tab open on a live page holding one frame. */
async function openPage(bed: Bed) {
  await bed.page.clock.install();
  bed.mock.loadPage(SEATS.beetle.id, { url: "https://example.com/", title: "Example Domain" });
  await bed.openSeat(SEATS.beetle);
  // A desktop opens this seat with its side panel shut. A phone always shows the strip.
  const reveal = bed.page.getByRole("button", { name: "Side panel" });
  if (await reveal.isVisible()) await reveal.click();
  await bed.page.getByRole("tab", { name: /^Browser/ }).click();
  await expect.poll(() => bed.mock.browserCalls.some((call) => call.type === "browser_watch" && call.session === SEATS.beetle.id)).toBe(true);
  // Stamped with the page's clock, as Chromium stamps a frame with when it painted it.
  const timestamp = (await bed.page.evaluate(() => Date.now())) / 1000;
  bed.mock.push({ type: "browser_frame", session: SEATS.beetle.id, data: JPEG, metadata: { ...metadata, timestamp }, seq: 1 });
  const pane = bed.page.getByRole("application", { name: /Example Domain/ });
  await expect(pane.locator("img")).toBeAttached();
  return pane;
}

/** A second at a time, so the connection probe's real reply lands inside its deadline. */
async function wait(bed: Bed, seconds: number) {
  for (let second = 0; second < seconds; second++) {
    await bed.page.clock.runFor(1000);
    await bed.page.waitForTimeout(20);
  }
}

test("a page that stops answering shows one strip, and it clears with no tap when the page answers", async ({ bed }) => {
  await openPage(bed);
  await wait(bed, 40);
  const status = bed.page.getByRole("status").filter({ hasText: "The page stopped answering." });
  await expect(status).toHaveCount(0);

  bed.mock.setStalled(SEATS.beetle.id, true);
  await expect(status, "one strip while the page is hung").toHaveCount(1);
  // The installed clock also flows between steps, so the age is 40s and a little more.
  await expect(status).toContainText(/Last picture (4\d|5\d)s ago\./);
  await expect(status).toContainText("A script stuck in a loop, or a dialog open on the page, does this.");
  await expect(status, "a host without browser-close names no Close page").not.toContainText("Close page");
  await wait(bed, 80);
  await expect(status, "the picture's age keeps time while it hangs").toContainText("Last picture 2m ago.");
  await bed.step("page-hung", { compare: false });

  bed.mock.setStalled(SEATS.beetle.id, false);
  await expect(status, "the strip clears with no tap").toHaveCount(0);
  const answering = bed.page.getByRole("status").filter({ hasText: "The page is answering again." });
  await expect(answering, "a screen reader hears it answer").toHaveCount(1);
  expect((await answering.boundingBox())?.width, "and nothing is drawn for it").toBeLessThanOrEqual(1);
  await bed.step("page-recovered", { compare: false });
});

test.describe("on a host that closes pages", () => {
  test.use({ mockOptions: { browserClose: true } });

  test("a hung page names Close page, and closing it ends the strip", async ({ bed }) => {
    await openPage(bed);
    bed.mock.setStalled(SEATS.beetle.id, true);
    const status = bed.page.getByRole("status").filter({ hasText: "The page stopped answering." });
    await expect(status).toContainText("Close page ends it.");
    // The seat drives this page, so Close page asks first, as on any page.
    await bed.page.getByRole("button", { name: "Close page" }).click();
    await bed.page.getByRole("button", { name: "Close it anyway" }).click();
    await expect.poll(() => bed.mock.browserCalls.some((call) => call.type === "browser_close" && call.session === SEATS.beetle.id)).toBe(true);
    await expect(status).toHaveCount(0);
    await expect(bed.page.getByText(/You closed the page\./)).toBeVisible();
    await bed.step("hung-page-closed", { compare: false });
  });
});

test("an idle live page shows no stall warning however long it sits", async ({ bed }) => {
  const pane = await openPage(bed);

  // One frame, then nothing for 30 seconds, as a page nobody touches sends.
  await wait(bed, 30);
  await expect(pane.locator("img"), "the last frame stays drawn").toBeAttached();
  await expect(bed.page.getByText(/stalled/i), "a still page is not a stalled stream").toHaveCount(0);
  await bed.step("idle-30s", { compare: false });
});
