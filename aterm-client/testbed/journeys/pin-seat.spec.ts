// COI-3346: anyone can pin a seat. Pinned seats lead the list, the pin outlives a reload,
// and a role the roster marks pinned by default starts that way until a person unpins it.
import type { Page } from "@playwright/test";
import { expect, kindOf, SEATS, test } from "../harness.ts";

const order = (page: Page) => page.locator('[id^="tab-session-"]').evaluateAll((tabs) => tabs.map((tab) => tab.getAttribute("id")!.replace("tab-session-", "")));
const openList = async (page: Page, phone: boolean) => {
  if (phone) await page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
};
// A reload rests on the seat overview, so the list is already on screen.
const reload = async (page: Page) => {
  await page.reload();
  await expect(page.getByRole("heading", { name: "Seats", level: 1 })).toBeVisible();
};
const first = (page: Page) => order(page).then((ids) => ids[0]);

test.describe("pin a seat", () => {
  test("a pinned seat leads the list and the pin survives a reload", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await openList(bed.page, phone);
    expect(await first(bed.page), "roster order before any pin").toBe(SEATS.beetle.id);
    const pin = bed.page.locator(".seat-row").filter({ hasText: "Griffin-Goose" }).getByRole("button", { name: "Pin to the top" });
    await expect(pin).toHaveAttribute("aria-pressed", "false");
    await pin.click();
    await expect(pin).toHaveAttribute("aria-pressed", "true");
    await expect.poll(() => first(bed.page)).toBe(SEATS.griffin.id);
    await bed.step("pinned", { compare: false });
    await reload(bed.page);
    await expect.poll(() => first(bed.page)).toBe(SEATS.griffin.id);
  });

  test("the open seat's header pins and unpins it too", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    const own = phone ? undefined : bed.page.locator("header").getByRole("button", { name: "Pin to the top" });
    if (own) {
      await own.click();
      await expect(own).toHaveAttribute("aria-pressed", "true");
      await own.click();
      await expect(own).toHaveAttribute("aria-pressed", "false");
    }
    await openList(bed.page, phone);
    const row = bed.page.locator(".seat-row").filter({ hasText: "Imp-Dragonfly" }).getByRole("button", { name: "Pin to the top" });
    await row.click();
    await expect.poll(() => first(bed.page)).toBe(SEATS.imp.id);
  });
});

test.describe("pinned by default", () => {
  test.use({ mockOptions: { pinnedRoles: ["prod-director"] } });

  test("a role with the attribute starts pinned, and unpinning it sticks", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await openList(bed.page, phone);
    expect(await first(bed.page), "the coordinator is pinned with no tap").toBe(SEATS.griffin.id);
    const pin = bed.page.locator(".seat-row").filter({ hasText: "Griffin-Goose" }).getByRole("button", { name: "Pin to the top" });
    await expect(pin).toHaveAttribute("aria-pressed", "true");
    await pin.click();
    await expect.poll(() => first(bed.page)).toBe(SEATS.beetle.id);
    await reload(bed.page);
    await expect.poll(() => first(bed.page), "unpinned stays unpinned after a reload").toBe(SEATS.beetle.id);
  });
});
