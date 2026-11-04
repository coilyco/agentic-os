// COI-3323 item 10: a launch returns to the last host and seat, or says why it cannot.
import { expect, SEATS, test } from "../harness.ts";

test("reopening the page returns to the last seat, with no tap", async ({ bed }) => {
  await bed.openSeat(SEATS.beetle);
  await expect(bed.screen()).toContainText("The gate is green");
  await bed.page.reload();
  await expect(bed.page.getByRole("heading", { name: SEATS.beetle.name, level: 1 })).toBeAttached();
  await expect(bed.screen()).toContainText("The gate is green");
  await expect(bed.page.getByRole("heading", { name: "Pick a host" }), "no host list in the way").toHaveCount(0);
  await bed.step("reopened");
});

test("a first launch has nothing to return to, so it opens on Pick a host", async ({ bed }) => {
  await bed.page.goto("/");
  await expect(bed.page.getByRole("heading", { name: "Pick a host" })).toBeVisible();
  await bed.step("first-launch");
});

test("a seat that is no longer running is named, on the host's page", async ({ bed }) => {
  await bed.page.addInitScript(() => localStorage.setItem("aterm.last-open.v1", JSON.stringify({ host: "local", seat: { id: "no-such-seat", who: "Ghost-Ox" } })));
  await bed.page.goto("/");
  await expect(bed.page.getByRole("status").filter({ hasText: "Ghost-Ox is not running any more" })).toBeVisible();
  await expect(bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first(), "the other seats are listed to pick from").toBeVisible();
  await bed.step("seat-gone");
});
