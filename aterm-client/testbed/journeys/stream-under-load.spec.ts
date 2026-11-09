// COI-3246, garbled text mid-stream. Compares screen text with what the script left.
// A guard only, since both pass without the fix. The proof is stream-resize-queued.
import { expect, kindOf, pushOutput, quirk, SEATS, test } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "one size is enough to see it"));

const FRAMES = 1500;
const redraw = (k: number): string => {
  // An ink-style tick: scroll some output, then erase three rows and print three.
  const filler = Array.from({ length: 12 }, (_, i) => `build log ${k} line ${i} ${"x".repeat(60)}`).join("\r\n");
  const rows = [1, 2, 3].map((r) => `\x1b[${r};1H\x1b[2Krow ${r} frame ${k} cache-control: no-cache`).join("");
  return `${filler}\r\n${rows}`;
};
const text = async (bed: { page: import("@playwright/test").Page }) =>
  bed.page.locator(".xterm-rows > div").evaluateAll((rows) => rows.map((row) => (row.textContent ?? "").replace(/ /g, " ").trimEnd()));

test("a resize mid-burst leaves the screen the script drew", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  const cols = bed.cols(SEATS.imp);
  const rows = bed.mock.boxOf(SEATS.imp.id)!.rows;
  for (let k = 0; k < FRAMES; k++) {
    pushOutput(bed.mock, SEATS.imp.id, redraw(k));
    // The daemon resizes the PTY mid-stream, as it does when a second client attaches.
    if (k === FRAMES / 2) bed.mock.push({ type: "size", session: SEATS.imp.id, rows, cols: cols - 12 });
  }
  const last = FRAMES - 1;
  await expect.poll(async () => (await text(bed)).slice(0, 3), { timeout: 45000 }).toEqual([1, 2, 3].map((r) => `row ${r} frame ${last} cache-control: no-cache`));
  await bed.step("after-burst");
});

test("a reconnect mid-burst leaves the screen the daemon replays", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  for (let k = 0; k < FRAMES; k++) {
    pushOutput(bed.mock, SEATS.imp.id, redraw(k));
    if (k === FRAMES / 2) bed.mock.dropClients();
  }
  // The client redials and the mock replays the seat's screen. The queue drains behind
  // it, so poll until the screen is right and fail on stale text.
  await expect.poll(async () => (await text(bed)).join("\n"), { timeout: 20000, message: "output from before the reconnect is still on screen" }).not.toContain("build log");
  await expect(bed.screen()).toContainText("accept edits");
});
