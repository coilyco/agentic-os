// COI-3521: one Settings page, reached from home, holding this device's settings.
import { expect, kindOf, loadFonts, reopenSeat, SEATS, seatEntry, test } from "../harness.ts";

const stored = (page: import("@playwright/test").Page, key: string) => page.evaluate((k) => localStorage.getItem(k), key);

test("Settings opens from home, changes take effect, and Back returns", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const home = page.getByRole("region", { name: "Seats", exact: true });
  await home.getByRole("button", { name: "Settings", exact: true }).click();

  const settings = page.getByRole("region", { name: "Settings" });
  await expect(settings, "the page").toBeVisible();
  await expect(settings).toContainText("These apply to this device only.");
  await expect(settings.getByRole("heading", { name: "Alerts" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "Seats by role" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "New seats open in" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "Terminal text" })).toBeVisible();
  await expect(settings.getByRole("heading", { name: "Hosts on this device" })).toBeVisible();
  // The home screen replaced the host page, the install card's old place.
  await expect(settings.getByRole("heading", { name: "Install Treehouse as an app" })).toBeVisible();
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
    await page.getByRole("navigation", { name: "Hosts and sessions" }).getByRole("button", { name: "Settings", exact: true }).click();
  } else {
    await page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ }).click();
    await page.getByText("Hosts and settings").click();
    await page.getByRole("button", { name: "Settings", exact: true }).click();
  }
  const settings = page.getByRole("region", { name: "Settings" });
  await expect(settings).toBeVisible();
  await bed.switchSeat(SEATS.beetle);
  await expect(settings, "picking a seat closes Settings").toHaveCount(0);
  await expect(page.locator(".xterm-rows")).toBeVisible();
});

test.describe("the view a seat opens in", () => {
  test.use({ mockOptions: { transcript: true } });

  test("a seat opens in the view chosen in Settings, and a seat keeps its own pick", async ({ page }) => {
    await page.goto("/");
    await loadFonts(page);
    await page.getByRole("tab", { name: /this Mac/ }).click();
    const home = page.getByRole("region", { name: "Seats", exact: true });
    await home.getByRole("button", { name: "Settings", exact: true }).click();
    const settings = page.getByRole("region", { name: "Settings" });
    const opens = settings.getByRole("radiogroup", { name: "New seats open in" });
    await expect(opens.getByRole("radio", { name: "Auto" }), "Auto until chosen").toHaveAttribute("aria-checked", "true");
    await opens.getByRole("radio", { name: "Default" }).click();
    await expect(opens.getByRole("radio", { name: "Default" })).toHaveAttribute("aria-checked", "true");
    expect(await stored(page, "aterm.seat-view-default.v1"), "stored without a reload").toBe("default");
    await settings.getByRole("button", { name: "Back" }).click();
    await seatEntry(page, SEATS.beetle.name).click();
    const view = page.getByRole("radiogroup", { name: "Seat view" });
    await expect(view.getByRole("radio", { name: "Default" }), "the seat opens in the chosen view").toHaveAttribute("aria-checked", "true");
    await view.getByRole("radio", { name: "Focus" }).click();
    await reopenSeat(page, SEATS.beetle);
    await expect(page.getByRole("radiogroup", { name: "Seat view" }).getByRole("radio", { name: "Focus" }), "the seat's own pick wins").toHaveAttribute("aria-checked", "true");
  });
});

test("Wrap long lines has a device default that is Auto until chosen and is kept at once", async ({ page }) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.getByRole("region", { name: "Seats", exact: true }).getByRole("button", { name: "Settings", exact: true }).click();
  const group = page.getByRole("region", { name: "Settings" }).getByRole("radiogroup", { name: "Wrap long lines" });
  await expect(group.getByRole("radio", { name: "Auto" })).toHaveAttribute("aria-checked", "true");
  await group.getByRole("radio", { name: "Off" }).click();
  await expect(group.getByRole("radio", { name: "Off" })).toHaveAttribute("aria-checked", "true");
  expect(await stored(page, "aterm.terminal-wrap-default.v1"), "stored without a reload").toBe("off");
  await group.getByRole("radio", { name: "Off" }).press("ArrowLeft");
  await expect(group.getByRole("radio", { name: "On" }), "arrows move between the stops").toHaveAttribute("aria-checked", "true");
  expect(await stored(page, "aterm.terminal-wrap-default.v1")).toBe("on");
});
