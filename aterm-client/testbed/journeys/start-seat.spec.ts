// Journey: start a seat. The mock plays the daemon's `launch`, then lists the new seat.
import { expect, kindOf, SEATS, test } from "../harness.ts";

test("start a seat", async ({ bed }, info) => {
  await bed.openSeat(SEATS.beetle);
  if (kindOf(info) === "phone") await bed.page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
  await bed.page.locator("#tab-role-frontend-eng").click();
  await expect(bed.page.getByRole("group", { name: "Launch on a harness" })).toBeVisible();
  await bed.step("role-panel");
  await bed.page.getByRole("button", { name: "Launch on claude" }).click();
  await expect.poll(() => bed.mock.launches).toEqual([{ role: "frontend-eng", seat: "claude" }]);
  await bed.step("starting");
  await expect(bed.page.getByRole("status").filter({ hasText: /launched/ })).toBeVisible();
  // The new seat is listed with the others: in the sidebar, or in a phone's seat sheet.
  if (kindOf(info) === "phone") await bed.page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
  await expect(bed.page.getByRole("tab", { name: /New-Seat/ }).first()).toBeVisible();
  await bed.step("listed");
});
