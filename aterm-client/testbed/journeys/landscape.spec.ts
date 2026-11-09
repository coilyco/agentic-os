// Journeys for COI-3341: a phone in landscape. Height is scarce, so the seat's terminal
// should fill the screen. These record how many rows of output a person can read.
import { expect, quirk, SEATS, test } from "../harness.ts";

const measure = (page: import("@playwright/test").Page) =>
  page.evaluate(() => {
    const host = document.querySelector(".term-host")!.getBoundingClientRect();
    const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].filter((row) => {
      const box = row.getBoundingClientRect();
      return box.height > 0 && box.top >= host.top - 1 && box.bottom <= host.bottom + 1 && (row.textContent ?? "").trim() !== "";
    });
    return { rows: rows.length, share: Math.round((host.height / innerHeight) * 100), height: innerHeight };
  });

test("the seat's terminal fills the screen", quirk("COI-3341"), async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  const seen = await measure(bed.page);
  // Recorded on every run, so a before and after sit side by side in the report.
  info.annotations.push({ type: "visible-rows", description: `${seen.rows} rows, terminal is ${seen.share}% of ${seen.height}px` });
  console.log(`${info.project.name}: ${seen.rows} rows of output visible, terminal is ${seen.share}% of the screen height`);
  await bed.step("seat-output");
  expect(seen.share, "percent of the screen height the terminal takes").toBeGreaterThanOrEqual(85);
});
