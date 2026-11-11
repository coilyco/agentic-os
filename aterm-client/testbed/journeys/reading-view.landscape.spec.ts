// COI-3341: a phone on its side reads one seat edge to edge. The seat list
// folds into a rail, the lists open beside it, and rotating back restores portrait.
import { expect, SEATS, test, seatEntry } from "../harness.ts";
import type { Page } from "@playwright/test";

const switcher = (page: Page) => page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ });

test("the seat list is a rail, and its lists open beside it", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const button = await switcher(bed.page).boundingBox();
  expect(button?.width, "a rail button, not a bar across the top").toBeLessThanOrEqual(52);
  await bed.step("rail");
  await switcher(bed.page).click();
  const menu = await bed.page.locator("#sidebar-menu").boundingBox();
  const height = bed.page.viewportSize()!.height;
  expect(menu?.y, "the lists start at the top of the screen").toBe(0);
  expect((menu?.y ?? 0) + (menu?.height ?? 0), "and run to the bottom, past the tab strip").toBe(height);
  await bed.page.locator("#tab-role-frontend-eng").scrollIntoViewIfNeeded();
  await expect(bed.page.locator("#tab-role-frontend-eng"), "Start a seat is reachable").toBeInViewport();
  await bed.step("seat-list");
  await seatEntry(bed.page, SEATS.beetle.name).click();
  await expect(bed.page.getByRole("heading", { name: SEATS.beetle.name, level: 1 })).toBeAttached();
  await expect(bed.page.locator("#sidebar-menu"), "choosing a seat puts the lists away").toBeHidden();
  await expect(bed.screen()).toContainText("The gate is green");
});

test("rotating keeps the seat and restores each layout", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  const named = switcher(bed.page).getByText(SEATS.imp.name, { exact: true });
  await expect(named, "the rail carries no name").toBeHidden();
  const { width, height } = bed.page.viewportSize()!;
  await bed.page.setViewportSize({ width: height, height: width });
  await expect(named, "portrait brings the bar and its name back").toBeVisible();
  await expect(bed.screen()).toContainText("Churning");
  await bed.step("portrait");
  await bed.page.setViewportSize({ width, height });
  await expect(named).toBeHidden();
  await expect(bed.screen()).toContainText("Churning");
  await bed.step("landscape-again");
});

test.describe("a phone under 720px wide on its side", () => {
  test.use({ viewport: { width: 667, height: 375 } });
  test("keeps the rail beside the seat, not above it", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    const rail = await bed.page.locator(".sidebar").boundingBox();
    const main = await bed.page.locator("main").boundingBox();
    expect(rail?.width, "the rail is a column").toBeLessThanOrEqual(52);
    expect(main?.y, "the seat starts at the top, not under a bar").toBe(0);
  });
});
