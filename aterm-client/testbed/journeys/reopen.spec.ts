// COI-3323 item 10, COI-3518: a launch returns to the last host, on its overview.
import { expect, SEATS, test } from "../harness.ts";

test("reopening the page returns to the host's seat overview, with no tap and no seat open", async ({ bed }) => {
  await bed.openSeat(SEATS.beetle);
  await expect(bed.screen()).toContainText("The gate is green");
  await bed.page.reload();
  await expect(bed.page.getByRole("heading", { name: "Seats", level: 1 }), "the overview").toBeVisible();
  await expect(bed.page.locator(".xterm-rows"), "no terminal is open").toHaveCount(0);
  await expect(bed.page.getByRole("heading", { name: "Pick a host" }), "no host list in the way").toHaveCount(0);
  await bed.step("reopened");
});

test("a first launch has nothing to return to, so it opens on Pick a host", async ({ bed }) => {
  await bed.page.goto("/");
  await expect(bed.page.getByRole("heading", { name: "Pick a host" })).toBeVisible();
  // The install card appears on a timer, so a host page is looked at, not compared.
  await bed.step("first-launch", { compare: false });
});
