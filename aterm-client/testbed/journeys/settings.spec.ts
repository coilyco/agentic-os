// COI-3521: one Settings page, reached from home, holding this device's settings.
import { expect, kindOf, loadFonts, SEATS, test } from "../harness.ts";

const stored = (page: import("@playwright/test").Page, key: string) => page.evaluate((k) => localStorage.getItem(k), key);

test("Settings opens from home, changes take effect, and Back returns", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const home = page.getByRole("region", { name: "Treehouse" });
  await home.getByRole("button", { name: "Settings" }).click();

  const settings = page.getByRole("region", { name: "Settings" });
  await expect(settings, "the page").toBeVisible();
  await expect(settings).toContainText("These apply to this device only.");
  await expect(settings.getByRole("heading", { name: "Alerts" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "Terminal text" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "Hosts on this device" })).toBeVisible();
  await page.mouse.move(0, 0);
  await page.screenshot({ path: info.outputPath("settings.png"), fullPage: true });

  // Sound is on by default for the coordinator's ping. Turning it off is stored at once.
  const sound = settings.getByRole("switch", { name: "Sound" });
  await expect(sound).toBeChecked();
  await sound.uncheck();
  expect(JSON.parse((await stored(page, "aterm.attention.v1")) ?? "{}").sound, "stored without a reload").toBe(false);

  const output = settings.locator("output");
  await expect(output).toHaveText("14px");
  await settings.getByRole("button", { name: "Smaller text" }).click();
  await expect(output).toHaveText("13px");
  expect(await stored(page, "aterm.terminal-text-size.v1")).toBe("13");
  await settings.getByRole("button", { name: /Reset to 14px/ }).click();
  await expect(output).toHaveText("14px");

  await settings.getByRole("button", { name: "Back" }).click();
  await expect(settings).toHaveCount(0);
  await expect(home, "back on the home screen").toBeVisible();
});

test("Settings opens from a seat's menu, and picking a seat closes it", async ({ bed, page }, info) => {
  await bed.openSeat(SEATS.imp);
  if (kindOf(info) === "desktop") {
    await page.getByRole("navigation", { name: "Hosts and sessions" }).getByRole("button", { name: "Settings" }).click();
  } else {
    await page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
    await page.getByText("Alerts and hosts").click();
    await page.getByRole("button", { name: "Settings" }).click();
  }
  const settings = page.getByRole("region", { name: "Settings" });
  await expect(settings).toBeVisible();
  await bed.switchSeat(SEATS.beetle);
  await expect(settings, "picking a seat closes Settings").toHaveCount(0);
  await expect(page.locator(".xterm-rows")).toBeVisible();
});
