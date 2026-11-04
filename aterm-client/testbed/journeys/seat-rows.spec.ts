// COI-3238 item 3: a seat's row should say what it is doing, not only idle or working.
import { expect, kindOf, quirk, SEATS, test } from "../harness.ts";

test("a row says how long the seat has been in its state", quirk("COI-3238 item 3"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  // The phone keeps its rows in a sheet. Open it so the rows are on screen.
  if (kindOf(test.info()) === "phone") await bed.page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
  await bed.step("rows");
  const row = bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first();
  await expect(row, "the row shows no time in state").toContainText(/\b\d+\s?(s|m|h|sec|min|hr)\b/);
});
