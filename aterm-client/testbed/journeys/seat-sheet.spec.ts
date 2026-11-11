// COI-3323 item 6: a phone's seat sheet. Start a seat is on screen, none under the strip.
import { expect, kindOf, SEATS, test } from "../harness.ts";

const opener = /Seats, starting a seat, hosts and settings/;

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "the sheet is a phone's"));

test("Start a seat is on screen with the seats listed, and the sheet is not under the strip", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await bed.page.getByRole("button", { name: opener }).click();
  const height = bed.page.viewportSize()!.height;
  const start = await bed.page.getByRole("heading", { name: "Start a seat" }).boundingBox();
  expect(start && start.y + start.height, "Start a seat is above the fold").toBeLessThan(height);
  await expect(bed.page.locator(".chips [role=tab]").first(), "the first role is on screen, the rest one swipe away").toBeInViewport();
  const menu = await bed.page.locator("#sidebar-menu").boundingBox();
  const strip = await bed.page.locator(".panel").first().boundingBox();
  const end = (menu?.y ?? 0) + (menu?.height ?? 0);
  expect(end <= (strip?.y ?? height) || end >= height - 1, `the sheet ends at ${end}, neither above the strip nor at the edge`).toBe(true);
  await expect(bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first(), "a row says more than idle").toContainText(/\b\d+\s?(s|m|h)\b/);
  await bed.step("sheet");
});

test("many seats scroll inside the sheet and still end at the screen's edge", async ({ bed }) => {
  for (let i = 0; i < 8; i++) {
    bed.mock.addSession({ name: `eng-platform-extra-${i}`, role: "eng-platform", identity: `Extra-Ox-${i}`, seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, scene: "claude-idle", quiet_seconds: 30 + i * 400 });
  }
  await bed.openSeat(SEATS.imp);
  await bed.page.getByRole("button", { name: opener }).click();
  const height = bed.page.viewportSize()!.height;
  const menu = await bed.page.locator("#sidebar-menu").boundingBox();
  expect((menu?.y ?? 0) + (menu?.height ?? 0), "a long sheet runs to the bottom edge").toBeGreaterThanOrEqual(height - 1);
  await bed.page.getByRole("button", { name: "Send feedback" }).scrollIntoViewIfNeeded();
  await expect(bed.page.locator("#tab-role-frontend-eng")).toBeAttached();
  await bed.step("many-seats");
});
