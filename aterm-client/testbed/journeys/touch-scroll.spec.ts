// COI-3253: Kai reported no scrolling on her phone. A touch swipe moves the history.
// Chromium only: the swipe is a CDP touch gesture and WebKit has no such call.
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const topLine = (page: import("@playwright/test").Page) =>
  page.evaluate(() => {
    const host = document.querySelector<HTMLElement>(".term-host")!.getBoundingClientRect();
    const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].filter((row) => {
      const box = row.getBoundingClientRect();
      return box.top >= host.top - 1 && box.bottom <= host.bottom + 1 && /^Line \d+/.test((row.textContent ?? "").trim());
    });
    const n = Number(/^Line (\d+)/.exec((rows[0]?.textContent ?? "").trim())?.[1] ?? NaN);
    return n;
  });

test("a finger swipe scrolls the seat's history up, and Jump to latest brings it back", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "phone" || info.project.name === "iphone", "a Chromium phone");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await bed.page.waitForTimeout(600);
  pushOutput(bed.mock, SEATS.imp.id, "\x1b[3J\x1b[2J\x1b[H" + Array.from({ length: 400 }, (_, i) => `Line ${i + 1} of the seat output`).join("\r\n") + "\r\n");
  await expect.poll(() => topLine(bed.page), "output arrived and follows the newest line").toBeGreaterThan(300);
  const before = await topLine(bed.page);

  const host = (await bed.page.locator(".term-host").boundingBox())!;
  const client = await bed.page.context().newCDPSession(bed.page);
  // A finger put down, dragged down in small steps, and lifted, as a screen sends it.
  const x = Math.round(host.x + host.width / 2);
  const y0 = Math.round(host.y + host.height / 3);
  const touch = (type: "touchStart" | "touchMove" | "touchEnd", y?: number) => client.send("Input.dispatchTouchEvent", { type, touchPoints: y === undefined ? [] : [{ x, y }] });
  await touch("touchStart", y0);
  for (let step = 1; step <= 20; step++) {
    await touch("touchMove", y0 + step * 20);
    await bed.page.waitForTimeout(16);
  }
  await touch("touchEnd");
  await expect.poll(() => topLine(bed.page), "a swipe down shows earlier lines").toBeLessThan(before - 5);
  await bed.step("scrolled-up", { compare: false });

  await bed.page.getByRole("button", { name: /Jump to latest/ }).click();
  await expect.poll(() => topLine(bed.page), "back at the newest line").toBeGreaterThan(300);
});
