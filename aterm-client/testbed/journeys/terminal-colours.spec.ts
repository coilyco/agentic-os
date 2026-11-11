// COI-3549: the terminal reads its colours through a DOM probe, not a literal. This pins
// the four xterm paints to Kai's Treehouse values (COI-3551), so a retune that moves one
// is a deliberate edit here and not a silent drift.
import { expect, SEATS, test } from "../harness.ts";

// --terminal and --text-soft, plus the one seat mark (COI-3621) as the cursor and, at
// alpha 0x55, the selection.
const BACKGROUND = "#271a0c";
const FOREGROUND = "#ddd6cf";
const ACCENT = "#ddd6cf";
const SELECTION_ALPHA = 0x55 / 255;

const channels = (hex: string) => [1, 3, 5].map((at) => parseInt(hex.slice(at, at + 2), 16));
const rgb = (hex: string) => `rgb(${channels(hex).join(", ")})`;
// xterm paints a translucent selection as its blend over the background.
const blended = channels(BACKGROUND).map((under, i) => Math.round(under + ((channels(ACCENT)[i] ?? 0) - under) * SELECTION_ALPHA));
const parts = (css: string) => (css.match(/\d+/g) ?? []).slice(0, 3).map(Number);

test("the terminal paints the background, text, cursor, and selection colours it always had", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  // A harness screen hides the cursor, so the seat shows a plain shell line with one.
  bed.mock.setScene(SEATS.imp.id, { render: () => ["colour check ", "$ "] });
  await expect(bed.screen()).toContainText("colour check");
  const page = bed.page;
  await page.locator(".xterm-helper-textarea").focus();
  // The block cursor blinks between two paints. Hold it on the one xterm sets for it.
  await page.addStyleTag({ content: ".xterm-cursor { animation: none !important }" });

  const paint = await page.evaluate(() => {
    const read = (selector: string) => getComputedStyle(document.querySelector(selector)!);
    const cursor = read(".xterm-cursor");
    const outlined = document.querySelector(".xterm-cursor")!.classList.contains("xterm-cursor-outline");
    return {
      background: read(".xterm-scrollable-element").backgroundColor,
      foreground: read(".xterm-rows").color,
      cursor: outlined ? cursor.outlineColor : cursor.backgroundColor,
    };
  });
  expect(paint.background, "terminal background").toBe(rgb(BACKGROUND));
  expect(paint.foreground, "terminal text").toBe(rgb(FOREGROUND));
  expect(paint.cursor, "cursor").toBe(rgb(ACCENT));

  const row = page.locator(".xterm-rows > div", { hasText: "colour check" }).first();
  const box = (await row.boundingBox())!;
  await page.mouse.move(box.x + 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + Math.min(box.width - 2, 200), box.y + box.height / 2, { steps: 6 });
  await page.mouse.up();
  const selection = page.locator(".xterm-selection div").first();
  await expect(selection, "a selection is drawn").toBeVisible();
  const painted = parts(await selection.evaluate((node) => getComputedStyle(node).backgroundColor));
  // One level of rounding either way is xterm's blend, two is a different colour.
  painted.forEach((value, i) => expect(Math.abs(value - (blended[i] ?? 0)), `selection channel ${i}: ${painted} vs ${blended}`).toBeLessThanOrEqual(1));
});

// COI-3552: the 16 ANSI colours are tokens too, and an escape code picks them up.
test("an escape code paints the palette chosen for the brown ground", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => ["\x1b[31mred \x1b[91mbright \x1b[0mplain"] });
  await expect(bed.screen()).toContainText("red bright plain");
  const painted = await bed.page.evaluate(() => {
    const colour = (cls: string) => getComputedStyle(document.querySelector(`.xterm-rows .${cls}`)!).color;
    return { red: colour("xterm-fg-1"), bright: colour("xterm-fg-9") };
  });
  expect(painted.red, "ANSI red").toBe(rgb("#ee7769"));
  expect(painted.bright, "ANSI bright red").toBe(rgb("#ff9a8d"));
});
