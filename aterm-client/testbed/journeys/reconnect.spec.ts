// Journey: a phone loses the daemon and gets it back with no tap (COI-3352), one test
// per way the link was cut, on a faked clock so minutes cost seconds.
import type { Page } from "@playwright/test";
import { expect, SEATS, test } from "../harness.ts";
import type { Mock } from "../mock-daemon.ts";

/** Moves fake time a second at a time until `done`. A second is under every client
 * timeout, and each step lets the page hear replies that arrive in real time. */
async function advanceUntil(page: Page, ms: number, done: () => boolean | Promise<boolean>): Promise<boolean> {
  for (let left = ms; left > 0; left -= 1000) {
    await page.clock.runFor(1000);
    await page.waitForTimeout(40);
    if (await done()) return true;
  }
  return false;
}

/** The page is backgrounded or comes back, as a phone does when it locks and wakes. */
const visibility = (page: Page, state: "hidden" | "visible") =>
  page.evaluate((next) => {
    Object.defineProperty(document, "visibilityState", { value: next, configurable: true });
    document.dispatchEvent(new Event("visibilitychange"));
  }, state);

/** Times the client attached: it asks for the roster on every welcome. */
const attaches = (mock: Mock) => mock.heard.filter((type) => type === "roster").length;

test.beforeEach(async ({ page }, info) => {
  test.skip(info.project.name === "desktop-1920", "the link does not depend on the screen");
  test.setTimeout(120_000);
  await page.clock.install();
});

async function attached(page: Page, openSeat: () => Promise<void>): Promise<void> {
  await openSeat();
  // From here fake time moves only when a test moves it.
  await page.clock.pauseAt(new Date(Date.now() + 5000));
}

test("a daemon restart of a minute and a half is ridden out", async ({ bed, mock }, info) => {
  await attached(bed.page, () => bed.openSeat(SEATS.imp));
  const before = attaches(mock);
  mock.dials("refuse");
  mock.dropClients();
  await advanceUntil(bed.page, 90 * 1000, () => false);
  await bed.page.screenshot({ path: info.outputPath("1-restarting.png") });
  expect(attaches(mock)).toBe(before);
  mock.dials("ok");
  // The wait has grown by now, so a restart this long needs the next try, not the first.
  expect(await advanceUntil(bed.page, 120 * 1000, () => attaches(mock) > before)).toBe(true);
  await expect(bed.page.getByText(/is reconnecting/)).toBeHidden();
});

test("a socket that goes silent without closing is noticed and redialed", async ({ bed, mock }, info) => {
  await attached(bed.page, () => bed.openSeat(SEATS.imp));
  const before = attaches(mock);
  // The phone slept, or moved from wifi to cellular. No close ever arrives.
  mock.silenceClients();
  const noticed = await advanceUntil(bed.page, 90 * 1000, () => attaches(mock) > before);
  await bed.page.screenshot({ path: info.outputPath("1-after.png") });
  expect(noticed, "the client never noticed the link was dead").toBe(true);
  await expect(bed.page.getByText(/is reconnecting/)).toBeHidden();
});

test("a page brought back to the foreground tests a link that died while it was away", async ({ bed, mock }) => {
  await attached(bed.page, () => bed.openSeat(SEATS.imp));
  const before = attaches(mock);
  await visibility(bed.page, "hidden");
  mock.silenceClients();
  // A background page's timers are frozen, so hours pass with no probe and no close.
  await bed.page.clock.fastForward(3 * 60 * 60 * 1000);
  await visibility(bed.page, "visible");
  expect(await advanceUntil(bed.page, 30 * 1000, () => attaches(mock) > before), "opening the page did not test the link").toBe(true);
});

test("a dial that hangs is dropped for a fresh one when the page comes back", async ({ bed, mock }, info) => {
  // WebKit queues a new dial behind one still connecting, and a page cannot cancel it.
  // Measured: the mock saw no new dial for 12 s.
  test.skip(info.project.name === "iphone", "WebKit queues a new dial behind a hung one");
  await attached(bed.page, () => bed.openSeat(SEATS.imp));
  const before = attaches(mock);
  mock.dials("hang");
  mock.dropClients();
  // The redial goes out and is never answered.
  await advanceUntil(bed.page, 3 * 1000, () => false);
  mock.dials("ok");
  await visibility(bed.page, "hidden");
  await visibility(bed.page, "visible");
  // Well under the ten seconds a hung dial waits before it gives up by itself.
  expect(await advanceUntil(bed.page, 4 * 1000, () => attaches(mock) > before), "a dial already out blocked the one the person's return asked for").toBe(true);
});

test("a page opened while the daemon is unreachable attaches when it comes up", async ({ bed, mock }, info) => {
  const { page } = bed;
  mock.dials("refuse");
  await page.goto("/");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await expect(page.getByRole("heading", { name: /is not answering/ })).toBeVisible();
  await page.screenshot({ path: info.outputPath("1-not-answering.png") });
  await page.clock.pauseAt(new Date(Date.now() + 5000));
  mock.dials("ok");
  const seat = page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first();
  await advanceUntil(page, 150 * 1000, () => seat.isVisible());
  await expect(seat, "the host stayed unreachable until someone pressed Retry").toBeVisible();
  await page.screenshot({ path: info.outputPath("2-attached.png") });
});

// Kai's cold start (COI-3352): one failed first dial stranded the page until Retry.
// The client now asks again within the first second of backoff.
test("a cold start whose first dial fails attaches on its own within the first backoff", async ({ bed, mock }, info) => {
  const { page } = bed;
  mock.refuseNext(1);
  // Frozen from the first byte, so the failed first dial can be seen before the retry.
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  await page.goto("/");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await expect(page.getByRole("heading", { name: /is not answering/ })).toBeVisible();
  await page.screenshot({ path: info.outputPath("1-first-dial-failed.png") });
  const seat = page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first();
  // The first wait is 1 s, spread 20%. Three seconds covers it and the handshake.
  expect(await advanceUntil(page, 3000, () => seat.isVisible()), "one failed first dial stranded the page until Retry").toBe(true);
  await page.screenshot({ path: info.outputPath("2-attached.png") });
});

test("a cold start whose attach dial fails, after the page-load probe passed, attaches on its own", async ({ bed, mock }, info) => {
  const { page } = bed;
  await page.goto("/");
  const host = page.getByRole("tab", { name: /this Mac/ });
  await expect(host).toContainText("running");
  // The probe passed. The dial that attaches the host is the one that fails.
  mock.refuseNext(1);
  await host.click();
  // The probe answered before, so the page calls this a host that stopped answering.
  await expect(page.getByRole("heading", { name: /stopped answering/ })).toBeVisible();
  await page.screenshot({ path: info.outputPath("1-attach-dial-failed.png") });
  await page.clock.pauseAt(new Date(Date.now() + 5000));
  const seat = page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first();
  expect(await advanceUntil(page, 3000, () => seat.isVisible()), "a failed attach dial took the closed path and never redialed").toBe(true);
  await page.screenshot({ path: info.outputPath("2-attached.png") });
});

test("after an hour with no answer the page rests, and opening it again dials at once", async ({ bed, mock }, info) => {
  test.skip(info.project.name !== "phone-320", "an hour of fake time costs minutes of real time, so one screen size carries it");
  await attached(bed.page, () => bed.openSeat(SEATS.imp));
  const before = attaches(mock);
  mock.dials("refuse");
  mock.dropClients();
  const banner = bed.page.getByRole("status").filter({ hasText: "stopped answering" });
  // Minute steps are safe: an unanswered dial ends on its own ten second timer.
  test.setTimeout(300_000);
  for (let spent = 0; spent < 65 * 60 * 1000; spent += 60_000) {
    await bed.page.clock.runFor(60_000);
    await bed.page.waitForTimeout(15);
  }
  await expect(banner).toBeVisible();
  await bed.page.screenshot({ path: info.outputPath("1-resting.png") });
  // Nothing is scheduled now: another hour of fake time dials nothing.
  const dials = mock.dialCount();
  await bed.page.clock.runFor(60 * 60 * 1000);
  expect(mock.dialCount()).toBe(dials);
  // The person opens the app again.
  mock.dials("ok");
  await visibility(bed.page, "hidden");
  await visibility(bed.page, "visible");
  // No fake time passes: the dial must go out the moment the page is shown, and a dial
  // needs only the socket's real time.
  await expect.poll(() => attaches(mock), { message: "opening the page after the hour did not dial", timeout: 15_000 }).toBeGreaterThan(before);
  await expect(banner).toBeHidden();
});
