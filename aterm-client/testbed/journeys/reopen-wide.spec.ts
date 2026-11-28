// COI-3839: a reopened seat wrote the bytes it held into a new xterm still at its 80x24
// default, before the host's `size` frame, so a 159-column screen wrapped at 80 and every
// redraw after it landed on the wrong rows. The held bytes now wait for the size.
import { expect, SEATS, test } from "../harness.ts";
import type { Scene } from "../scenes.ts";

const WIDE = 159;
const HEAD = "HEAD-OF-THE-WIDE-ROW";
const TAIL = "TAIL-OF-THE-WIDE-ROW";

// One row as wide as the PTY, with a marker at each end.
const wideScene: Scene = {
  render(cols, rows) {
    const fill = Math.max(0, cols - HEAD.length - TAIL.length - 2);
    const wide = `${HEAD} ${"·".repeat(fill)} ${TAIL}`;
    const tail = [wide, "", "❯ ", "─".repeat(cols)];
    return [...Array(Math.max(0, rows - tail.length)).fill(""), ...tail];
  },
};

test.use({ mockOptions: { offsets: true } });
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("aterm.terminal-wrap-default.v1", "off"));
});

const shape = (page: import("@playwright/test").Page) =>
  page.evaluate(
    ([head, tail]) => {
      const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].map((row) => row.textContent ?? "");
      return { together: rows.some((row) => row.includes(head!) && row.includes(tail!)), head: rows.some((row) => row.includes(head!)) };
    },
    [HEAD, TAIL],
  );

test("a reopened seat draws what it held at the host's width, not xterm's default", async ({ bed }) => {
  bed.mock.holdWide(SEATS.imp.id, WIDE);
  bed.mock.setScene(SEATS.imp.id, wideScene);
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText(HEAD);
  expect((await shape(bed.page)).together, "first open: the wide row is one row").toBe(true);
  await bed.switchSeat(SEATS.beetle);
  await expect(bed.screen()).toContainText("The gate is green");
  await bed.switchSeat(SEATS.imp);
  await expect(bed.screen()).toContainText(HEAD);
  expect((await shape(bed.page)).together, "reopened: the held row is still one row").toBe(true);
  await bed.step("reopened-wide", { compare: false });
});
