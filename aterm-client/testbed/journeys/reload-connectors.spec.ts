// COI-3787: an idle seat offers Reload connectors in its own view, asks once, and says
// what the daemon answered, with no terminal on the path.
import { expect, SEATS, test } from "../harness.ts";

test("Reload connectors asks first, reloads the seat by its ref, and says it is back", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const header = bed.page.locator("header");
  const reload = header.getByRole("button", { name: `Reload connectors for ${SEATS.imp.name}` });
  await expect(reload, "a seat the daemon has not read idle offers no reload").toHaveCount(0);
  bed.mock.patchSession(SEATS.imp.id, { tui: "idle", ref: "imp00001" });
  await expect(reload).toBeVisible();
  await reload.click();
  await expect(header.getByRole("group", { name: `Reload ${SEATS.imp.name}?` })).toBeVisible();
  await bed.step("ask", { compare: false });
  await header.getByRole("button", { name: "Reload", exact: true }).click();
  await expect.poll(() => bed.mock.reloads).toEqual([`${SEATS.imp.id}@imp00001`]);
  await expect(header.getByRole("status")).toHaveText(`${SEATS.imp.id} is back on its conversation and idle.`);
  await bed.step("reloaded", { compare: false });
});
