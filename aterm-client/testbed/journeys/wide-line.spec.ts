// COI-3317: a line wider than a phone, because a wider client holds the PTY, can be
// scrolled sideways to its last character. Kai's preference is a scroll, not a wrap.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import type { Scene } from "../scenes.ts";

const TAIL = "END-OF-THE-LONG-LINE";
const WIDE = 120;

// A seat whose newest line is wider than any phone, and whose input row sits below it.
const wideScene: Scene = {
  render(cols, rows) {
    const long = `⏺ ${"The command that was run, with every flag spelled out, ".repeat(1)}${TAIL}`.slice(0, cols);
    const tail = [long, "", "❯ ", "─".repeat(cols)];
    return [...Array(Math.max(0, rows - tail.length)).fill(""), ...tail];
  },
};

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's narrow box"));

const reach = (page: import("@playwright/test").Page) =>
  page.evaluate((tail) => {
    const host = document.querySelector<HTMLElement>(".term-host")!;
    const row = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].find((r) => r.textContent?.includes(tail));
    const frame = host.getBoundingClientRect();
    const box = row?.getBoundingClientRect();
    return {
      overflow: host.scrollWidth - host.clientWidth,
      left: host.scrollLeft,
      hostRight: frame.right,
      rowFound: Boolean(row),
      rowRight: box?.right ?? 0,
      panning: host.classList.contains("panning"),
    };
  }, TAIL);

test("a line wider than the phone scrolls sideways to its last character", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, WIDE);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText(TAIL);
  await expect(bed.page.locator(".edge.right"), "a cue that more text lies to the right").toHaveClass(/\bon\b/);
  await expect(bed.page.locator(".edge.left")).not.toHaveClass(/\bon\b/);
  await bed.step("before", { compare: false });
  const before = await reach(bed.page);
  expect(before.panning, "the terminal pans when the PTY is wider than the box").toBe(true);
  expect(before.overflow, "pixels of text past the right edge").toBeGreaterThan(100);
  await bed.page.evaluate(() => {
    const host = document.querySelector<HTMLElement>(".term-host")!;
    host.scrollLeft = host.scrollWidth;
  });
  await bed.page.waitForTimeout(400);
  const after = await reach(bed.page);
  expect(after.left, "stays where the person scrolled to").toBeGreaterThan(100);
  await expect(bed.page.locator(".edge.right"), "no cue once the end is reached").not.toHaveClass(/\bon\b/);
  await expect(bed.page.locator(".edge.left"), "a cue that text lies to the left").toHaveClass(/\bon\b/);
  await bed.step("scrolled-right", { compare: false });
});

test("a swipe sideways moves the text, and new output does not snap it back", async ({ bed, browserName }) => {
  test.skip(browserName !== "chromium", "touch events are sent through Chromium only");
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, WIDE);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText(TAIL);
  await expect.poll(async () => (await reach(bed.page)).overflow, "the wide PTY has laid out").toBeGreaterThan(100);
  await bed.page.waitForTimeout(1000);
  const client = await bed.page.context().newCDPSession(bed.page);
  const box = (await bed.page.locator(".term-host").boundingBox())!;
  // A finger, as touch events: it lands, drifts left and a little down, and lifts.
  const at = (step: number) => [{ x: box.x + box.width * 0.8 - step * 30, y: box.y + box.height * 0.5 + step * 3, id: 1 }];
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: at(0) });
  for (let step = 1; step <= 12; step++) {
    await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: at(step) });
    await bed.page.waitForTimeout(16);
  }
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await bed.page.waitForTimeout(500);
  const swiped = await reach(bed.page);
  expect(swiped.left, "a sideways swipe over the text moves it").toBeGreaterThan(100);
  // The seat writes again, as a working one does every second.
  bed.mock.push({ type: "output", session: SEATS.imp.id, data: Buffer.from("\x1b[1;1Hworking").toString("base64") });
  await bed.page.waitForTimeout(600);
  const written = await reach(bed.page);
  expect(written.left, "output arriving does not return the view to column 0").toBe(swiped.left);
});

test("the view stays where it was panned when the viewport changes height", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.holdWide(SEATS.imp.id, WIDE);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await expect(bed.screen()).toContainText(TAIL);
  // Let the first layout and its timers finish, so only the change below moves the view.
  await bed.page.waitForTimeout(1500);
  await bed.page.evaluate(() => {
    const host = document.querySelector<HTMLElement>(".term-host")!;
    host.scrollLeft = host.scrollWidth;
  });
  const panned = (await reach(bed.page)).left;
  expect(panned).toBeGreaterThan(100);
  // A phone's address bar or keyboard changes the height, which is a resize here.
  const size = bed.page.viewportSize()!;
  await bed.page.setViewportSize({ width: size.width, height: size.height - 160 });
  await bed.page.waitForTimeout(700);
  expect((await reach(bed.page)).left, "a height change returns the view to column 0").toBe(panned);
  // Another client attaching makes the daemon send the size again.
  bed.mock.holdWide(SEATS.imp.id, WIDE + 20);
  await bed.page.waitForTimeout(700);
  expect((await reach(bed.page)).left, "a size frame returns the view to column 0").toBeGreaterThan(100);
});
