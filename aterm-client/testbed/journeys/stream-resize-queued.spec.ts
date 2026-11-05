// COI-3292, the negative control COI-3245 lacked (COI-3246). Output still queued in
// xterm when a size frame lands draws at the old size, as a drained window would.
// Relative cursor moves only heal when drawn at the right width.
import xterm from "@xterm/xterm";
import type { Page } from "@playwright/test";
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const { Terminal } = xterm;

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "one size is enough to see it"));

const FRAMES = 20000;
const NARROW = 30;

// Erase three rows (cursor-up, erase-below), then print three that wrap at 30.
function frames(count: number): string {
  let out = "";
  for (let index = 0; index < count; index++) {
    if (index > 0) out += "\x1b[3A\x1b[J";
    for (let line = 0; line < 3; line++) out += `frame ${String(index).padStart(5, "0")} line ${line} ${"x".repeat(21)}\r\n`;
  }
  return out;
}

const pageRows = (page: Page) =>
  page.locator(".xterm-rows > div").evaluateAll((rows) => rows.map((row) => (row.textContent ?? "").replace(/ /g, " ").trimEnd()));

// What a window that had drained every byte before the resize shows.
async function drained(cols: number, rows: number): Promise<string[]> {
  const term = new Terminal({ cols, rows, scrollback: 100, allowProposedApi: true });
  await new Promise<void>((done) => term.write(frames(FRAMES), done));
  term.resize(NARROW, rows);
  const buffer = term.buffer.active;
  return Array.from({ length: rows }, (_, at) => buffer.getLine(buffer.baseY + at)?.translateToString(true).trimEnd() ?? "");
}

test("a size frame behind a queued burst draws what a drained window would", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  // The mock repaints its scene when the client reports a size, as a harness does.
  // That would land over the burst, so a blank scene leaves only the burst to read.
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await expect(bed.screen()).not.toContainText("Churning");
  const cols = bed.cols(SEATS.imp);
  const rows = (await pageRows(bed.page)).length;
  const wanted = await drained(cols, rows);
  // One frame, so xterm still parses it when the next websocket message lands.
  pushOutput(bed.mock, SEATS.imp.id, `\x1bc${frames(FRAMES)}`);
  bed.mock.push({ type: "size", session: SEATS.imp.id, rows, cols: NARROW });
  await expect
    .poll(async () => (await pageRows(bed.page)).join("\n"), { timeout: 30000, message: "the screen differs from a drained window's: the burst was drawn at the wrong width" })
    .toEqual(wanted.join("\n"));
  await bed.step("after-queued-resize", { compare: false });
});

// The first read after a reconnect, not the one it settles to. COI-3246 saw stale
// text for a moment past 1500 frames. It holds without the fix too: a guard only.
test("the first screen after a reconnect mid-burst holds nothing from before it", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  for (let k = 0; k < 3000; k++) {
    pushOutput(bed.mock, SEATS.imp.id, `build log ${k} ${"x".repeat(60)}\r\n`);
    if (k === 1500) bed.mock.dropClients();
  }
  let first = "";
  await expect.poll(async () => (first = (await pageRows(bed.page)).join("\n")), { timeout: 20000 }).toContain("accept edits");
  expect(first, "the first read after the reconnect still shows output from before it").not.toContain("build log");
});
