// COI-3510: with a host attached and no seat open, the app shows the Treehouse logo.
import { expect, loadFonts, SEATS, test } from "../harness.ts";

test("a host with no seat open shows the logo and how to go on, not the host page", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const home = page.getByRole("region", { name: "Treehouse" });
  await expect(home, "the home screen").toBeVisible();
  const logo = home.locator("img");
  await expect(logo).toBeVisible();
  expect(await logo.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0), "the logo loaded").toBe(true);
  await expect(home).toContainText("seats running. Pick one from the list.");
  await expect(page.getByText("Pick a seat from the list to open its terminal."), "the old host page is gone").toHaveCount(0);
  const box = (await logo.boundingBox())!;
  const view = page.viewportSize()!;
  expect(box.width, `${info.project.name}: a readable size`).toBeGreaterThanOrEqual(120);
  expect(box.x + box.width, "inside the screen").toBeLessThanOrEqual(view.width);
  await page.mouse.move(0, 0);
  await page.screenshot({ path: info.outputPath("home.png") });
  // A seat still opens from here.
  await page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
  await expect(page.locator(".xterm-rows")).toBeVisible();
  await expect(home, "home gives way to the seat").toHaveCount(0);
});
