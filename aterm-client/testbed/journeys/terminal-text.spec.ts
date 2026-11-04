// COI-3238 item 1: a seat's text should hold up next to iTerm. Solid boxes, links,
// find, and history that goes back.
import { expect, kindOf, pushOutput, quirk, SEATS, test } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "desktop terminal behavior"));

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
  pushOutput(bed.mock, SEATS.imp.id, "\r\nsee https://code.claude.com/docs/en/permission-modes for the modes\r\n");
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
  await expect(bed.page.locator(".xterm").first(), "the pointer does not become a link cursor over a URL").toHaveClass(/xterm-cursor-pointer/);
  await bed.step("link-hover");
});

test("find text in the terminal", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const find = bed.page.getByRole("searchbox").or(bed.page.getByRole("button", { name: /^(find|search)/i }));
  await expect(find.first(), "the terminal has no find control").toBeVisible();
});

test("history reaches back past a thousand lines", quirk("COI-3238 item 1"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const flood = Array.from({ length: 3000 }, (_, i) => `history ${String(i + 1).padStart(4, "0")}`).join("\r\n");
  pushOutput(bed.mock, SEATS.imp.id, `\r\n${flood}\r\n`);
  await expect(bed.screen()).toContainText("history 3000");
  await bed.page.locator(".xterm-viewport").first().evaluate((el) => (el.scrollTop = 0));
  await expect(bed.screen(), "the first line of 3000 is gone, scrollback is only 1000").toContainText("history 0001");
  await bed.step("top-of-history");
});
