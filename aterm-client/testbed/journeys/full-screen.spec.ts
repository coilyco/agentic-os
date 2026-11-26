// COI-3741: a sheet, drawer or picker is its own screen, nothing of the seat behind it.
// Going back finds the draft and the scroll where they were.
import type { Page } from "@playwright/test";
import { expect, kindOf, loadFonts, SEATS, test } from "../harness.ts";

const DRAFT = "half a thought about the gate";

/** Whether any of five points across the element lands on the element itself. */
function shows(page: Page, selector: string): Promise<boolean> {
  return page.evaluate((sel) => {
    const el = document.querySelector(sel);
    if (!el) return false;
    const box = el.getBoundingClientRect();
    const spots = [[0.5, 0.5], [0.1, 0.1], [0.9, 0.1], [0.1, 0.9], [0.9, 0.9]];
    return spots.some(([fx, fy]) => {
      const [x, y] = [box.left + box.width * fx!, box.top + box.height * fy!];
      if (x < 0 || y < 0 || x >= innerWidth || y >= innerHeight) return false;
      const hit = document.elementFromPoint(x, y);
      return !!hit && el.contains(hit);
    });
  }, selector);
}

/** Marks the seat's scroller, or its screen, and scrolls it up a little. */
function markSpot(page: Page): Promise<void> {
  return page.evaluate(() => {
    const all = [...document.querySelectorAll<HTMLElement>(".screen *")];
    const scroller = all.find((el) => el.scrollHeight > el.clientHeight + 80 && getComputedStyle(el).overflowY !== "visible");
    const target = scroller ?? document.querySelector<HTMLElement>(".screen")!;
    target.dataset.spot = "";
    if (scroller) scroller.scrollTop = Math.max(0, scroller.scrollHeight - scroller.clientHeight - 40);
  });
}

/** Where the marked scroller sits, or -1 once a rebuilt seat lost the mark. */
function scrollSpot(page: Page): Promise<number> {
  return page.evaluate(() => document.querySelector<HTMLElement>("[data-spot]")?.scrollTop ?? -1);
}

async function behind(page: Page, name: string, open: () => Promise<void>, close: () => Promise<void>): Promise<void> {
  const spot = await scrollSpot(page);
  await open();
  await expect.poll(() => shows(page, ".screen"), `${name}: none of the seat's screen shows`).toBe(false);
  expect(await shows(page, "form.composer"), `${name}: nor its message box`).toBe(false);
  await page.mouse.move(0, 0);
  await page.screenshot({ path: test.info().outputPath(`${name}.png`) });
  await close();
  await expect.poll(() => shows(page, ".screen"), `${name}: back on the seat`).toBe(true);
  await expect(page.locator("#composer-input"), `${name}: the draft is kept`).toHaveValue(DRAFT);
  expect(await scrollSpot(page), `${name}: the same seat, scrolled where it was`).toBe(spot);
}

test.describe("every surface over a seat", () => {
  test.use({ mockOptions: { feedback: true, version: "aos-v0.578.0" } });

  test("is its own screen, and going back keeps the draft and the scroll", async ({ bed, page }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await loadFonts(page);
    // The pin under the message box takes room, so it lands before the scroll is marked.
    if (phone) bed.mock.setLastReply(SEATS.imp.id, { text: "The gate is green.", ageSeconds: 30 });
    await page.locator("#composer-input").fill(DRAFT);
    await page.locator("#composer-input").blur();
    // A late font or frame refits the screen, so the mark waits for a quiet one.
    await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
    await markSpot(page);
    expect(await scrollSpot(page), "the seat is marked").toBeGreaterThanOrEqual(0);
    expect(await shows(page, ".screen"), "the seat shows before anything opens").toBe(true);
    const escape = () => page.keyboard.press("Escape");

    await behind(page, "feedback", () => page.getByRole("button", { name: /Send feedback/ }).first().click(), escape);
    if (!phone) return;

    const opener = page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ });
    await behind(page, "seat-menu", () => opener.click(), escape);
    await behind(page, "side-panel", () => page.getByRole("tab", { name: "Messages" }).click(), escape);

    const pin = page.getByRole("button", { name: /Last reply from Imp-Dragonfly/ });
    if (await pin.count()) await behind(page, "last-reply", () => pin.click(), escape);
  });
});
