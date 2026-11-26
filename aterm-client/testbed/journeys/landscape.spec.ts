// Journeys for COI-3341: a phone in landscape. Height is scarce, so the seat's terminal
// should fill the screen. These record how many rows of output a person can read.
import { expect, SEATS, test } from "../harness.ts";

const measure = (page: import("@playwright/test").Page) =>
  page.evaluate(() => {
    const host = document.querySelector(".term-host")!.getBoundingClientRect();
    const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].filter((row) => {
      const box = row.getBoundingClientRect();
      return box.height > 0 && box.top >= host.top - 1 && box.bottom <= host.bottom + 1 && (row.textContent ?? "").trim() !== "";
    });
    return { rows: rows.length, share: Math.round((host.height / innerHeight) * 100), height: innerHeight };
  });

test("the seat's terminal fills the screen", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  const seen = await measure(bed.page);
  // Recorded on every run, so a before and after sit side by side in the report.
  info.annotations.push({ type: "visible-rows", description: `${seen.rows} rows, terminal is ${seen.share}% of ${seen.height}px` });
  console.log(`${info.project.name}: ${seen.rows} rows of output visible, terminal is ${seen.share}% of the screen height`);
  await bed.step("seat-output");
  expect(seen.share, "percent of the screen height the terminal takes").toBeGreaterThanOrEqual(85);
});

// COI-3371, Kai's Pixel: the field stops at three line heights and then scrolls, so it
// shows 2 to 3 rows and never stands much taller than the buttons beside it.
test("a long draft grows to three line heights, then scrolls", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await field.fill(Array.from({ length: 8 }, (_, n) => `line ${n + 1} of a long reply`).join("\n"));
  const box = await field.evaluate((area: HTMLTextAreaElement) => {
    const style = getComputedStyle(area);
    return { height: area.getBoundingClientRect().height, line: parseFloat(style.lineHeight), scrolls: area.scrollHeight > area.clientHeight };
  });
  info.annotations.push({ type: "field", description: `${Math.round(box.height)}px, ${(box.height / box.line).toFixed(1)} line heights` });
  expect(box.height, "three line heights, padding included").toBeLessThanOrEqual(3 * box.line + 3);
  expect(box.height, "it did grow past one line").toBeGreaterThan(2 * box.line);
  expect(box.scrolls, "the rest scrolls inside the field").toBe(true);
  await bed.step("long-draft", { compare: false });
});

test.describe("with the transcript", () => {
  test.use({ mockOptions: { transcript: true } });

  test("the transcript's freshness line is not shown on its side", async ({ bed }) => {
    bed.mock.setTranscript(SEATS.imp.id, [{ id: "p1", kind: "prompt", text: "an earlier message", at: "2026-11-10T21:00:00Z" }]);
    await bed.openSeat(SEATS.imp);
    await bed.page.getByRole("radio", { name: "Focus" }).click();
    await expect(bed.page.locator(".transcript")).toContainText("an earlier message");
    await expect(bed.page.getByText(/^From the transcript/)).toHaveCount(0);
  });
});
