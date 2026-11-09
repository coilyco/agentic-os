// COI-3497: on a desktop, a line wider than the pane shows more is there.
// The edge arrows were phone-only, so a wide PTY clipped without a sign.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import type { Scene } from "../scenes.ts";

const TAIL = "END-OF-THE-LONG-LINE";

// A seat whose newest line is as wide as the PTY, with its input row below it.
const wideScene: Scene = {
  render(cols, rows) {
    const long = `⏺ ${"Read(aterm-client/src/lib/daemon-host.ts) with every flag spelled out, ".repeat(3)}${TAIL}`.slice(0, cols);
    const tail = [long, "", "❯ ", "─".repeat(cols)];
    return [...Array(Math.max(0, rows - tail.length)).fill(""), ...tail];
  },
};

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a desktop pane"));

const panBar = (page: import("@playwright/test").Page) =>
  page.evaluate(() => {
    const bar = document.querySelector<HTMLElement>(".pan")!;
    const thumb = bar.querySelector<HTMLElement>("i")!.getBoundingClientRect();
    const track = bar.getBoundingClientRect();
    return { shown: getComputedStyle(bar).opacity === "1", thumbShare: thumb.width / track.width, thumbLeft: thumb.left - track.left };
  });

test("a PTY wider than the pane shows arrows and a position bar, and the rest can be reached", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, 340);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText("daemon-host.ts");
  await expect(bed.page.locator(".edge.right"), "an arrow that more text lies to the right").toHaveClass(/\bon\b/);
  await expect(bed.page.locator(".edge.right"), "visible on a desktop, not only a phone").toBeVisible();
  await expect.poll(async () => (await panBar(bed.page)).shown, "the position bar shows").toBe(true);
  const before = await panBar(bed.page);
  expect(before.thumbShare, "the bar's thumb is the visible share of the line").toBeLessThan(0.9);
  await bed.step("before", { compare: false });

  await bed.page.evaluate(() => {
    const host = document.querySelector<HTMLElement>(".term-host")!;
    host.scrollLeft = host.scrollWidth;
  });
  await expect(bed.page.locator(".edge.left"), "an arrow that text lies to the left").toHaveClass(/\bon\b/);
  await expect(bed.page.locator(".edge.right"), "no arrow once the end is reached").not.toHaveClass(/\bon\b/);
  await expect.poll(async () => (await panBar(bed.page)).thumbLeft, "the thumb moved with the pan").toBeGreaterThan(before.thumbLeft + 20);
  await bed.step("at-the-end", { compare: false });
});

test("narrowing the window keeps the sign that more is there", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, 130);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText("daemon-host.ts");
  await bed.page.setViewportSize({ width: 1000, height: 800 });
  await expect(bed.page.locator(".edge.right"), "still marked after the window narrows").toHaveClass(/\bon\b/);
  await expect.poll(async () => (await panBar(bed.page)).shown).toBe(true);
});

test("a PTY that fits the pane shows no arrows and no bar", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, 60);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText("daemon-host.ts");
  await expect(bed.page.locator(".edge.right")).not.toHaveClass(/\bon\b/);
  await expect(bed.page.locator(".edge.left")).not.toHaveClass(/\bon\b/);
  expect((await panBar(bed.page)).shown, "no bar when nothing is hidden").toBe(false);
});
