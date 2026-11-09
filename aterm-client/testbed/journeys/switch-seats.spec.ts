// Journey: move between seats and see each one's own screen, never the last one's.
import { expect, SEATS, test } from "../harness.ts";

test("switch seats", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await bed.switchSeat(SEATS.beetle);
  await expect(bed.page.getByRole("heading", { name: SEATS.beetle.name, level: 1 })).toBeAttached();
  await expect(bed.screen()).toContainText("The gate is green");
  await expect(bed.screen()).not.toContainText("Churning");
  await bed.step("second-seat");
  await bed.switchSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await bed.step("back-to-first");
});
