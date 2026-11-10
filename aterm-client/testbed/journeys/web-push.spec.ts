// Journey: the Notify when closed switch (COI-2559), worst states first. Delivery to a
// phone is COI-2568 and is not here.
import type { Page } from "@playwright/test";
import { expect, test } from "../harness.ts";

const overview = (page: Page) => page.getByRole("region", { name: "Seats", exact: true });
const notify = (page: Page) => page.getByRole("switch", { name: "Notify when closed" });

// Phones show Alerts on the host page and desktops in the sidebar.
async function openAlerts(page: Page): Promise<void> {
  await page.goto("/");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await expect(page.getByRole("switch", { name: "Sound" }).first()).toBeVisible();
  // A healthy attached host shows the seat overview, whose ages move with time, so the
  // steps below mask it (COI-3518).
  await expect(overview(page)).toBeVisible();
}

test.describe("a host with no push key", () => {
  test("offers no switch, and nothing about notifications", async ({ bed }) => {
    await openAlerts(bed.page);
    await expect(notify(bed.page)).toHaveCount(0);
    await bed.step("no-switch", { mask: [overview(bed.page)] });
  });
});

test.describe("a host with a push key", () => {
  test.use({ mockOptions: { webPush: true } });

  test("offers the switch off, before anything is asked", async ({ bed }, info) => {
    await openAlerts(bed.page);
    await expect(notify(bed.page).first()).not.toBeChecked();
    expect(bed.mock.pushCalls).toEqual([]);
    // Safari in a tab has no push manager, so there the switch is held back and says why.
    if (info.project.name.startsWith("iphone")) {
      await expect(notify(bed.page).first()).toBeDisabled();
      await expect(bed.page.getByText(/add the app to the Home Screen first/)).toBeVisible();
    } else {
      await expect(notify(bed.page).first()).toBeEnabled();
    }
    await bed.step("off", { mask: [overview(bed.page)] });
  });

  // Headless Chromium reports notifications denied. A failed subscribe: push.test.ts.
  test("a browser that blocks notifications says so, and the switch stays off", async ({ bed }, info) => {
    test.skip(info.project.name.startsWith("iphone"), "Safari in a tab holds the switch back first, covered above.");
    await openAlerts(bed.page);
    await notify(bed.page).first().click();
    await expect(bed.page.getByRole("alert").filter({ hasText: /blocked for this site/ })).toBeVisible();
    await expect(notify(bed.page).first()).not.toBeChecked();
    // Permission goes first, so the daemon heard nothing.
    expect(bed.mock.pushCalls).toEqual([]);
    await bed.step("notifications-blocked", { mask: [overview(bed.page)] });
  });
});

test.describe("a locked device on a host with a push key", () => {
  test.use({ mockOptions: { webPush: true, lock: true } });

  test("is told why before the browser is asked for anything", async ({ bed }, info) => {
    test.skip(info.project.name.startsWith("iphone"), "Safari in a tab holds the switch back first, covered above.");
    await openAlerts(bed.page);
    await notify(bed.page).first().click();
    await expect(bed.page.getByRole("alert").filter({ hasText: /coilyco\.dev/ })).toBeVisible();
    await expect(notify(bed.page).first()).not.toBeChecked();
    expect(bed.mock.pushCalls).toEqual([]);
    await bed.step("locked", { mask: [overview(bed.page)] });
  });
});
