// COI-3324: on a phone's side the panel has no strip, and one button raises it.
import { expect, SEATS, test } from "../harness.ts";
import type { Page } from "@playwright/test";

const reveal = (page: Page) => page.getByRole("button", { name: /Open the side panel/ });
const hide = (page: Page) => page.getByRole("button", { name: "Hide the side panel" });

test("no strip, and one 44px button inside the screen opens the panel", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await expect(bed.page.getByRole("tab", { name: /^Messages/ }), "the strip is gone").toBeHidden();
  const { width, height } = bed.page.viewportSize()!;
  const composer = await bed.page.getByRole("textbox", { name: /^Message / }).boundingBox();
  expect((composer?.y ?? 0) + (composer?.height ?? 0), "no strip is reserved under the composer").toBeGreaterThanOrEqual(height - 20);
  const button = await reveal(bed.page).boundingBox();
  expect(button?.width).toBeGreaterThanOrEqual(44);
  expect(button?.height).toBeGreaterThanOrEqual(44);
  expect((button?.x ?? 0) + (button?.width ?? 0), "inside the right edge").toBeLessThanOrEqual(width);
  expect(button?.y, "inside the top edge").toBeGreaterThanOrEqual(0);
  await bed.step("closed");
  await reveal(bed.page).click();
  await expect(bed.page.locator("#side-panel")).toBeVisible();
  const panel = await bed.page.locator("aside.panel").boundingBox();
  expect(panel?.x, "the rail stays uncovered").toBeGreaterThanOrEqual(52);
  expect(panel?.height, "the panel runs the full height").toBeGreaterThanOrEqual(height - 1);
  for (const tab of await bed.page.getByRole("tab").all()) expect((await tab.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await bed.step("open");
});

test("the hide button and Escape close it, and focus goes back to the button", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await reveal(bed.page).click();
  await expect(bed.page.locator("#side-panel")).toBeVisible();
  await hide(bed.page).click();
  await expect(bed.page.locator("#side-panel")).toBeHidden();
  await expect(reveal(bed.page), "focus returns to where the panel came from").toBeFocused();
  await reveal(bed.page).click();
  await bed.page.keyboard.press("Escape");
  await expect(bed.page.locator("#side-panel")).toBeHidden();
  await expect(reveal(bed.page)).toBeVisible();
});

test("an open panel and its tab survive rotating there and back", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await reveal(bed.page).click();
  await bed.page.getByRole("tab", { name: /^Views/ }).click();
  const { width, height } = bed.page.viewportSize()!;
  await bed.page.setViewportSize({ width: height, height: width });
  await expect(bed.page.locator("#side-panel"), "portrait keeps it raised").toBeVisible();
  await expect(bed.page.getByRole("tab", { name: /^Views/ })).toHaveAttribute("aria-selected", "true");
  await bed.step("portrait");
  await bed.page.setViewportSize({ width, height });
  await expect(bed.page.locator("#side-panel"), "and so does landscape again").toBeVisible();
  await expect(bed.page.getByRole("tab", { name: /^Views/ })).toHaveAttribute("aria-selected", "true");
  await bed.step("landscape-again");
});
