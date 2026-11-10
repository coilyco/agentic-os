// COI-3253: Kai reported no scrolling on her phone. A touch swipe moves the history.
// COI-3537: a full-screen app has no history, so the swipe is its wheel.
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

const OUTPUT = Array.from({ length: 400 }, (_, i) => `Line ${i + 1} of the seat output`).join("\r\n") + "\r\n";

/** A finger down a third of the host, dragged down in small steps, and lifted. */
async function swipeDown(page: import("@playwright/test").Page): Promise<void> {
  const host = (await page.locator(".term-host").boundingBox())!;
  const client = await page.context().newCDPSession(page);
  const x = Math.round(host.x + host.width / 2);
  const y0 = Math.round(host.y + host.height / 3);
  const touch = (type: "touchStart" | "touchMove" | "touchEnd", y?: number) => client.send("Input.dispatchTouchEvent", { type, touchPoints: y === undefined ? [] : [{ x, y }] });
  await touch("touchStart", y0);
  for (let step = 1; step <= 20; step++) {
    await touch("touchMove", y0 + step * 20);
    await page.waitForTimeout(16);
  }
  await touch("touchEnd");
}

const phoneOnly = (info: import("@playwright/test").TestInfo) => test.skip(kindOf(info) !== "phone" || info.project.name === "iphone", "a Chromium phone");

for (const [name, modes] of [["alt screen with the mouse reported", "\x1b[?1049h\x1b[?1000h\x1b[?1006h"], ["alt screen with no mouse", "\x1b[?1049h"]] as const) {
  test(`a swipe in a full-screen app is the wheel it would get from a mouse (${name})`, async ({ bed }, info) => {
    phoneOnly(info);
    await bed.openSeat(SEATS.imp);
    bed.mock.setScene(SEATS.imp.id, { render: () => [] });
    await bed.page.waitForTimeout(600);
    pushOutput(bed.mock, SEATS.imp.id, modes + "\x1b[2J\x1b[H" + OUTPUT);
    await expect.poll(() => bed.page.locator(".xterm-rows").innerText()).toContain("Line 400");
    const before = bed.mock.typed(SEATS.imp.id).length;
    await swipeDown(bed.page);
    // A finger down shows what is above, so the wheel goes up: SGR button 64, or arrow A.
    const sent = () => bed.mock.typed(SEATS.imp.id).slice(before).join("");
    if (modes.includes("1000h")) await expect.poll(sent, "wheel-up reports").toMatch(/\x1b\[<64;\d+;\d+M/);
    else await expect.poll(sent, "up arrows").toMatch(/\x1b\[A/);
    expect(sent()).not.toMatch(/\x1b\[<65|\x1b\[B/);
  });
}

test("a swipe with the mouse reported on the normal screen still reads the history", async ({ bed }, info) => {
  phoneOnly(info);
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await bed.page.waitForTimeout(600);
  pushOutput(bed.mock, SEATS.imp.id, "\x1b[?1000h\x1b[?1006h\x1b[3J\x1b[2J\x1b[H" + OUTPUT);
  await expect.poll(() => topLine(bed.page)).toBeGreaterThan(300);
  const before = await topLine(bed.page);
  const typed = bed.mock.typed(SEATS.imp.id).length;
  await swipeDown(bed.page);
  await expect.poll(() => topLine(bed.page), "earlier lines").toBeLessThan(before - 5);
  expect(bed.mock.typed(SEATS.imp.id).length, "the app was not sent the wheel").toBe(typed);
});

test("the scrollbar, dragged by a finger, scrolls the history with the mouse reported", async ({ bed }, info) => {
  phoneOnly(info);
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await bed.page.waitForTimeout(600);
  pushOutput(bed.mock, SEATS.imp.id, "\x1b[?1000h\x1b[?1006h\x1b[3J\x1b[2J\x1b[H" + OUTPUT);
  await expect.poll(() => topLine(bed.page)).toBeGreaterThan(300);
  const before = await topLine(bed.page);
  const slider = (await bed.page.locator(".xterm .scrollbar.vertical .slider").boundingBox())!;
  const client = await bed.page.context().newCDPSession(bed.page);
  const x = Math.round(slider.x + slider.width / 2);
  const y0 = Math.round(slider.y + slider.height / 2);
  const touch = (type: "touchStart" | "touchMove" | "touchEnd", y?: number) => client.send("Input.dispatchTouchEvent", { type, touchPoints: y === undefined ? [] : [{ x, y }] });
  await touch("touchStart", y0);
  for (let step = 1; step <= 15; step++) {
    await touch("touchMove", y0 - step * 10);
    await bed.page.waitForTimeout(16);
  }
  await touch("touchEnd");
  await expect.poll(() => topLine(bed.page), "earlier lines").toBeLessThan(before - 20);
});
