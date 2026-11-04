// Journey: open a seat and read what it printed.
import { expect, SEATS, test } from "../harness.ts";

test("read a seat's output", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("The gate is green");
  await bed.step("seat-output");
  // A wide line must wrap, never push the page sideways.
  const overflow = await bed.page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow, "the page scrolls sideways").toBeLessThanOrEqual(0);
});
