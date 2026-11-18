// COI-3681: while the open seat works, the composer shows the sidebar's spinner, from the
// same state value and stays typable. One burst drives both, and quiet clears both.
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const SHEET = /Seats, starting a seat, hosts and settings/;

test("the composer and the sidebar row show working together, and clear together", async ({ bed }) => {
  const page = bed.page;
  const phone = kindOf(test.info()) === "phone";
  const field = page.locator("#composer-input");
  const ring = page.locator('.field[data-working="true"]');
  const row = page.locator(`[id="tab-session-${SEATS.imp.id}"]`);

  await bed.openSeat(SEATS.imp);
  // The worst state first: a quiet seat shows no ring in either place.
  await expect(ring, "an idle seat's field shows a ring").toHaveCount(0);
  await expect(field).not.toHaveAttribute("aria-describedby", /composer-working/);

  // Working is recent output, so keep it coming while the screen is read.
  const pump = setInterval(() => pushOutput(bed.mock, SEATS.imp.id, "."), 300);
  try {
    await expect(ring, "no ring round the field while the seat is busy").toHaveCount(1);
    await expect(field, "the field was locked while the seat is busy").toBeEditable();
    await expect(field).toHaveAttribute("aria-describedby", "composer-working");
    await expect(page.locator("#composer-working")).toHaveText(/is working\. You can still type\./);
    await field.fill("queued while you work");
    await expect(field).toHaveValue("queued while you work");
    await bed.step("working", { compare: false });
    if (phone) await page.getByRole("button", { name: SHEET }).click();
    await expect(row, "the sidebar row is not working while the composer is").toHaveAttribute("data-activity", "working");
    if (phone) await page.keyboard.press("Escape");
  } finally {
    clearInterval(pump);
  }

  // Quiet clears the field's ring and the row's spinner on the same push.
  await expect(ring, "the ring stayed after the seat went quiet").toHaveCount(0, { timeout: 8000 });
  if (phone) await page.getByRole("button", { name: SHEET }).click();
  await expect(row, "the row still spins after the composer cleared").not.toHaveAttribute("data-activity", "working");
});
