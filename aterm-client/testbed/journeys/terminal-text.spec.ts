// COI-3238 item 1: a seat's text should hold up next to iTerm. Solid boxes, links,
// find, and history that goes back.
import { expect, kindOf, pushOutput, quirk, SEATS, test } from "../harness.ts";

const text = async (bed: { page: import("@playwright/test").Page }) =>
  bed.page.locator(".xterm-rows > div").evaluateAll((rows) => rows.map((row) => (row.textContent ?? "").replace(/\u00a0/g, " ").trimEnd()));

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "desktop terminal behavior"));

// The mock repaints its scene whenever the client reports a size, as a harness does,
// and that wipes what a journey pushed. A blank scene leaves the pushed bytes alone.
async function blank(bed: Parameters<Parameters<typeof test>[2]>[0]["bed"]): Promise<void> {
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await expect(bed.screen()).not.toContainText("Churning");
}

test("box drawing stays solid", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await bed.step("frame");
  // A box joins its neighbours only when a row is no taller than the font's own cell.
  const gap = await bed.page.evaluate(() => {
    const row = document.querySelector<HTMLElement>(".xterm-rows > div")!;
    const probe = document.createElement("span");
    probe.textContent = "│";
    probe.style.cssText = `font: ${getComputedStyle(row).font}; line-height: normal; position: absolute; visibility: hidden`;
    document.body.append(probe);
    const cell = probe.getBoundingClientRect().height;
    probe.remove();
    return row.getBoundingClientRect().height - cell;
  });
  expect(gap, "pixels of empty gap between rows of box drawing").toBeLessThanOrEqual(1);
});

test("a link in the output is clickable", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await blank(bed);
  pushOutput(bed.mock, SEATS.imp.id, "\x1b[3;1Hsee https://code.claude.com/docs/en/permission-modes for the modes");
  await expect(bed.screen()).toContainText("permission-modes");
  const at = await bed.page.evaluate(() => {
    const walker = document.createTreeWalker(document.querySelector(".xterm-rows")!, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const index = node.textContent?.indexOf("https://") ?? -1;
      if (index < 0) continue;
      const range = document.createRange();
      range.setStart(node, index + 8);
      range.setEnd(node, index + 12);
      const box = range.getBoundingClientRect();
      return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
    }
    return null;
  });
  expect(at, "the pushed URL is not on screen").not.toBeNull();
  await bed.page.mouse.move(at!.x, at!.y);
  // xterm puts the class on its screen element, not on the terminal's root.
  await expect(bed.page.locator(".xterm-screen"), "the pointer does not become a link cursor over a URL").toHaveClass(/xterm-cursor-pointer/);
  await bed.step("link-hover", { compare: false });
});

test("find text in the terminal", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const find = bed.page.getByRole("searchbox").or(bed.page.getByRole("button", { name: /^(find|search)/i }));
  await expect(find.first(), "the terminal has no find control").toBeVisible();
});

test("history reaches back past a thousand lines", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await blank(bed);
  const flood = Array.from({ length: 3000 }, (_, i) => `history ${String(i + 1).padStart(4, "0")}`).join("\r\n");
  // Clear the screen and its history first, so the oldest line is the first one pushed.
  pushOutput(bed.mock, SEATS.imp.id, `\x1b[3J\x1b[2J\x1b[H${flood}\r\n`);
  await expect(bed.screen()).toContainText("history 3000");
  // A wheel event moves xterm 6 about three rows whatever its delta, so 3000 rows would
  // take a thousand events. Shift+PageUp reads the same buffer a page at a time.
  await bed.page.locator(".xterm-helper-textarea").focus();
  const first = async () => (await text(bed)).filter(Boolean)[0];
  for (let page = 0; page < 150 && (await first()) !== "history 0001"; page++) await bed.page.keyboard.press("Shift+PageUp");
  expect(await first(), "history does not reach back to the first of 3000 lines").toBe("history 0001");
  await bed.step("top-of-history", { compare: false });
});
