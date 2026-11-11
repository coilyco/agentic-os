// Journey: start a seat. The mock plays the daemon's `launch`, then lists the new seat.
// COI-3480: the client opens that seat itself, unless a person has gone elsewhere first.
import { expect, kindOf, SEATS, test } from "../harness.ts";

const SEATS_BUTTON = /Seats, starting a seat, hosts and settings/;

async function toStartPanel(bed: Parameters<Parameters<typeof test>[2]>[0]["bed"], phone: boolean) {
  await bed.openSeat(SEATS.beetle);
  if (phone) await bed.page.getByRole("button", { name: SEATS_BUTTON }).click();
  await bed.page.locator("#tab-role-frontend-eng").click();
  await expect(bed.page.getByRole("group", { name: "Launch on a harness" })).toBeVisible();
}

const newSeatHeading = (bed: { page: import("@playwright/test").Page }) => bed.page.getByRole("heading", { name: "New-Seat", level: 1 });

test("start a seat", async ({ bed }, info) => {
  await toStartPanel(bed, kindOf(info) === "phone");
  await bed.step("role-panel");
  await bed.page.getByRole("button", { name: "Launch on claude" }).click();
  await expect.poll(() => bed.mock.launches).toEqual([{ role: "frontend-eng", seat: "claude" }]);
  // The roster lists the new seat and the client opens it: no hunt through the list.
  await expect(newSeatHeading(bed)).toBeAttached();
  await expect(bed.page.getByRole("group", { name: "Launch on a harness" }), "the Start panel has gone").toHaveCount(0);
  await expect(bed.page.locator(".xterm-rows")).toBeVisible();
  // Not compared: the Linux baseline needs the CI workflow run with update on.
  await bed.step("opened", { compare: false });
});

test.describe("a slow roster", () => {
  test.use({ mockOptions: { launchDelayMs: 2500 } });

  test("the Start panel says it launched, then the new seat opens when it is listed", async ({ bed }, info) => {
    await toStartPanel(bed, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Launch on claude" }).click();
    await expect(bed.page.getByRole("status").filter({ hasText: /launched/ })).toBeVisible();
    await expect(newSeatHeading(bed), "not open before the roster lists it").toHaveCount(0);
    await bed.step("waiting", { compare: false });
    await expect(newSeatHeading(bed)).toBeAttached({ timeout: 8000 });
    await bed.step("opened-late", { compare: false });
  });

  test("a tap on another seat while it waits cancels the move", async ({ bed }, info) => {
    await toStartPanel(bed, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Launch on claude" }).click();
    await expect.poll(() => bed.mock.launches).toHaveLength(1);
    await bed.switchSeat(SEATS.imp);
    await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
    // Past the mock's delay the new seat is listed, and still the person's pick stands.
    await expect.poll(async () => {
      if (kindOf(info) === "phone") await bed.page.getByRole("button", { name: SEATS_BUTTON }).click();
      const listed = await bed.page.getByRole("tab", { name: /New-Seat/ }).count();
      if (kindOf(info) === "phone") await bed.page.keyboard.press("Escape");
      return listed;
    }, { timeout: 8000 }).toBeGreaterThan(0);
    await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
    await expect(newSeatHeading(bed)).toHaveCount(0);
  });
});
