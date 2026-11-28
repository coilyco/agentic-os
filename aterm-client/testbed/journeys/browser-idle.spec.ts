// COI-3823: Chromium casts only what repaints, so a still page sends no frames,
// and an idle live page must never read as a stalled stream.
import { expect, SEATS, test } from "../harness.ts";

// A 4x4 JPEG, the shape of a real `browser_frame`'s data.
const JPEG = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAoHBwgHBgoICAgLCgoLDhgQDg0NDh0VFhEYIx8lJCIfIiEmKzcvJik0KSEiMEExNDk7Pj4+JS5ESUM8SDc9Pjv/2wBDAQoLCw4NDhwQEBw7KCIoOzs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozv/wAARCAAEAAQDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAf/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFAEBAAAAAAAAAAAAAAAAAAAAAv/EABQRAQAAAAAAAAAAAAAAAAAAAAD/2gAMAwEAAhEDEQA/AK0ABP/Z";
const metadata = { offset_top: 0, page_scale_factor: 1, device_width: 390, device_height: 844, scroll_offset_x: 0, scroll_offset_y: 0, timestamp: 0 };

test("an idle live page shows no stall warning however long it sits", async ({ bed }) => {
  await bed.page.clock.install();
  bed.mock.loadPage(SEATS.beetle.id, { url: "https://example.com/", title: "Example Domain" });
  await bed.openSeat(SEATS.beetle);
  // A desktop opens this seat with its side panel shut. A phone always shows the strip.
  const reveal = bed.page.getByRole("button", { name: "Side panel" });
  if (await reveal.isVisible()) await reveal.click();
  await bed.page.getByRole("tab", { name: /^Browser/ }).click();
  await expect.poll(() => bed.mock.browserCalls.some((call) => call.type === "browser_watch" && call.session === SEATS.beetle.id)).toBe(true);
  bed.mock.push({ type: "browser_frame", session: SEATS.beetle.id, data: JPEG, metadata, seq: 1 });
  const pane = bed.page.getByRole("application", { name: /Example Domain/ });
  await expect(pane.locator("img")).toBeAttached();

  // One frame, then nothing for 30 seconds, as a page nobody touches sends. A second at a
  // time, so the connection probe's real socket reply lands inside its own deadline.
  for (let second = 0; second < 30; second++) {
    await bed.page.clock.runFor(1000);
    await bed.page.waitForTimeout(20);
  }
  await expect(pane.locator("img"), "the last frame stays drawn").toBeAttached();
  await expect(bed.page.getByText(/stalled/i), "a still page is not a stalled stream").toHaveCount(0);
  await bed.step("idle-30s", { compare: false });
});
