// COI-3648: the open seat holds a bounded window, not every item it has ever read.
import { expect, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const HOLD = 400;
const item = (n: number) => ({ id: `r${n}`, kind: "reply", text: `Item number ${n}.`, complete: true });
const run = (from: number, count: number) => Array.from({ length: count }, (_, at) => item(from + at));

const rows = (bed: Bed) => bed.page.locator(".transcript .feed > :is(.turn, .step, .note)");
const feed = (bed: Bed) => bed.page.locator(".transcript .feed");
const gap = (bed: Bed) => feed(bed).evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);

async function openVerbose(bed: Bed) {
  await bed.openSeat(SEATS.imp);
  await bed.page.getByRole("radio", { name: "Focus" }).click();
  await bed.page.getByRole("radio", { name: "Verbose" }).click();
  await expect(feed(bed)).toBeVisible();
}

test("reading far back drops the newest rows, and Jump to latest comes back to them", async ({ bed }) => {
  bed.mock.setTranscript(imp, run(1, 2000));
  await openVerbose(bed);
  await expect(feed(bed)).toContainText("Item number 2000.");
  for (let page = 0; page < 12; page++) {
    const before = await rows(bed).count();
    await bed.page.locator(".earlier").click();
    await expect(bed.page.locator(".earlier")).toBeEnabled();
    await expect.poll(() => rows(bed).count()).toBeGreaterThan(Math.min(before, HOLD - 1));
  }
  expect(await rows(bed).count(), "twelve pages back holds the window and no more").toBeLessThanOrEqual(HOLD);
  await expect(feed(bed), "the oldest page read is held").toContainText("Item number 1500.");
  const jump = bed.page.getByRole("button", { name: /Jump to latest/ });
  await expect(jump, "the newest rows are gone, so the way back is offered").toBeVisible();
  await jump.click();
  await expect(feed(bed)).toContainText("Item number 2000.");
  expect(await rows(bed).count(), "back on the newest page").toBeLessThanOrEqual(60);
  await expect.poll(() => gap(bed)).toBeLessThan(4);
});

test("a seat watched through hundreds of arrivals holds the window and stays pinned", async ({ bed }) => {
  bed.mock.setTranscript(imp, run(1, 50));
  await openVerbose(bed);
  let next = 51;
  for (let batch = 0; batch < 5; batch++) {
    bed.mock.appendTranscript(imp, run(next, 100));
    next += 100;
    await expect(feed(bed)).toContainText(`Item number ${next - 1}.`, { timeout: 6000 });
  }
  expect(await rows(bed).count(), "550 items arrived, the window held").toBeLessThanOrEqual(HOLD);
  await expect.poll(() => gap(bed), { message: "still pinned to the newest" }).toBeLessThan(4);
  bed.mock.appendTranscript(imp, run(next, 3));
  await expect(feed(bed)).toContainText(`Item number ${next + 2}.`, { timeout: 6000 });
});
