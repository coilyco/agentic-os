// COI-3253: two-finger trackpad scrolling reads the history, whether or not the harness
// reports the mouse. A full-screen app keeps its mouse.
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const FLOOD = "\x1b[3J\x1b[2J\x1b[H" + Array.from({ length: 400 }, (_, i) => `Line ${i + 1} of the seat output`).join("\r\n") + "\r\n";
const MOUSE_ON = "\x1b[?1000h\x1b[?1006h";

const topLine = (page: import("@playwright/test").Page) =>
  page.evaluate(() => {
    const host = document.querySelector<HTMLElement>(".term-host")!.getBoundingClientRect();
    const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].filter((row) => {
      const box = row.getBoundingClientRect();
      return box.top >= host.top - 1 && box.bottom <= host.bottom + 1 && /^Line \d+/.test((row.textContent ?? "").trim());
    });
    return Number(/^Line (\d+)/.exec((rows[0]?.textContent ?? "").trim())?.[1] ?? NaN);
  });

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a trackpad on a desktop"));

async function open(bed: import("../harness.ts").Bed, tail: string) {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await bed.page.waitForTimeout(600);
  pushOutput(bed.mock, SEATS.imp.id, FLOOD + tail);
  await expect.poll(() => topLine(bed.page), "output arrived").toBeGreaterThan(300);
  const box = (await bed.page.locator(".term-host").boundingBox())!;
  await bed.page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
}

test("the wheel scrolls the history on the normal screen", async ({ bed }) => {
  await open(bed, "");
  const before = await topLine(bed.page);
  await bed.page.mouse.wheel(0, -300);
  await bed.page.mouse.wheel(0, -300);
  await expect.poll(() => topLine(bed.page)).toBeLessThan(before - 3);
});

test("with the mouse reported, the wheel still scrolls the history and sends nothing", async ({ bed }) => {
  await open(bed, MOUSE_ON);
  const before = await topLine(bed.page);
  await bed.page.mouse.wheel(0, -300);
  await bed.page.mouse.wheel(0, -300);
  await expect.poll(() => topLine(bed.page), "history moved").toBeLessThan(before - 3);
  expect(bed.mock.typed(SEATS.imp.id).filter((text) => text.startsWith("\x1b[<")), "no wheel reports sent").toEqual([]);
});

test("a full-screen app keeps its mouse", async ({ bed }) => {
  await open(bed, "");
  pushOutput(bed.mock, SEATS.imp.id, "\x1b[?1049h" + MOUSE_ON);
  await bed.page.waitForTimeout(400);
  await bed.page.mouse.wheel(0, -300);
  await expect.poll(() => bed.mock.typed(SEATS.imp.id).filter((text) => text.startsWith("\x1b[<")).length, "the app got the wheel").toBeGreaterThan(0);
});
