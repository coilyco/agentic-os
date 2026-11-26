// COI-3348 slice A: the host's roles beside Settings. COI-3749 named the entry Roles.
import { expect, kindOf, loadFonts, SEATS, test } from "../harness.ts";

const entry = (page: import("@playwright/test").Page) => page.getByRole("button", { name: "Roles", exact: true });

test("with no host connected the section says so and offers the host list", async ({ page, bed }) => {
  await page.goto("/");
  await loadFonts(page);
  await entry(page).click();
  const section = page.getByRole("region", { name: "Roles" });
  await expect(section.getByText("No host is connected.")).toBeVisible();
  await bed.step("no-host");
  await section.getByRole("button", { name: "Pick a host" }).click();
  await expect(section, "picking a host leaves the section").toHaveCount(0);
});

test("it opens from home, lists each role with its seat and pool, and Back returns", async ({ page, bed }) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const home = page.getByRole("region", { name: "Seats", exact: true });
  await home.getByRole("button", { name: "Roles", exact: true }).click();

  const section = page.getByRole("region", { name: "Roles" });
  await expect(section).toContainText("Editing arrives once this host can store configuration.");
  await expect(section.getByRole("heading", { name: "Roles" })).toBeVisible();
  const platform = section.getByRole("listitem").filter({ hasText: "Platform Engineer" });
  await expect(platform).toContainText("Beetle-Ox");
  await expect(platform).toContainText("Later seats: Stag-Ox, Weevil-Ox, Cricket-Ox");
  await expect(platform).toContainText("Runs on: claude, codex");
  await expect(section.getByRole("listitem").filter({ hasText: "Frontend Engineer" })).toContainText("Imp-Dragonfly");
  await bed.step("roles");

  await section.getByRole("button", { name: "Back" }).click();
  await expect(section).toHaveCount(0);
  await expect(home, "back on the home screen").toBeVisible();
});

test("it opens from a seat's menu, and picking a seat closes it", async ({ bed, page }, info) => {
  await bed.openSeat(SEATS.imp);
  if (kindOf(info) === "desktop") {
    await page.getByRole("navigation", { name: "Hosts and sessions" }).getByRole("button", { name: "Roles", exact: true }).click();
  } else {
    await page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ }).click();
    await entry(page).click();
  }
  const section = page.getByRole("region", { name: "Roles" });
  await expect(section).toBeVisible();
  await bed.switchSeat(SEATS.beetle);
  await expect(section, "picking a seat closes it").toHaveCount(0);
  await expect(page.locator(".xterm-rows")).toBeVisible();
});

test("the keyboard reaches Back and Enter closes the section", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const home = page.getByRole("region", { name: "Seats", exact: true });
  await home.getByRole("button", { name: "Roles", exact: true }).click();
  const section = page.getByRole("region", { name: "Roles" });
  await expect(section).toBeVisible();
  await section.getByRole("button", { name: "Back" }).focus();
  await expect(section.getByRole("button", { name: "Back" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(section).toHaveCount(0);
});

test("a phone's seat menu shows Roles and Settings on its first tap, and no Configuration row", async ({ bed, page }, info) => {
  test.skip(kindOf(info) !== "phone", "the seat menu is a phone's");
  await bed.openSeat(SEATS.imp);
  await page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ }).click();
  const menu = page.locator("#sidebar-menu");
  await expect(menu.getByRole("button", { name: "Roles", exact: true }), "one tap, no second fold (COI-3744)").toBeVisible();
  await expect(menu.getByRole("button", { name: "Settings", exact: true })).toBeAttached();
  await expect(menu.getByRole("tab", { name: /this Mac/ }), "the hosts are listed too").toBeAttached();
  await expect(menu.getByRole("button", { name: "Configuration" }), "the entry is named for what it holds (COI-3749)").toHaveCount(0);
  await expect(menu.getByText("Hosts and settings")).toHaveCount(0);
  await menu.getByRole("button", { name: "Roles", exact: true }).click();
  const roles = page.getByRole("region", { name: "Roles" });
  await expect(roles.getByRole("listitem").filter({ hasText: "Platform Engineer" }), "the tap opens the roles list").toBeVisible();
});
