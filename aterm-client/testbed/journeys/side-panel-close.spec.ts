// COI-3820: the side panel has a visible close, Escape closes it on a desktop, a closed
// panel stays closed for its seat across reloads, and the Browser tab can close its page.
import type { Page } from "@playwright/test";
import { expect, kindOf, reopenSeat, SEATS, test, type Bed } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) === "landscape" || info.project.name === "desktop-1920", "320px and 1280px"));

const close = (page: Page) => page.getByRole("button", { name: "Close the side panel" });
const toggle = (page: Page) => page.locator("#side-toggle");
const panel = (page: Page) => page.locator("aside.panel");

/** A desktop project runs these at 1280px, the width the issue names. */
async function at1280(bed: Bed): Promise<void> {
  if (kindOf(test.info()) === "desktop") await bed.page.setViewportSize({ width: 1280, height: 800 });
}

test.describe("on a desktop", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "the panel beside the seat"));

  test("a visible close and Escape both close the panel, and the header button says which it is", async ({ bed }) => {
    await at1280(bed);
    await bed.openSeat(SEATS.imp);
    await expect(toggle(bed.page)).toHaveAttribute("aria-expanded", "false");
    await toggle(bed.page).click();
    await expect(toggle(bed.page), "the header button shows it open").toHaveAttribute("aria-expanded", "true");
    await expect(close(bed.page), "a close control at the panel's top edge").toBeVisible();
    const box = (await close(bed.page).boundingBox())!;
    expect(box.width, "a 44px target").toBeGreaterThanOrEqual(44);
    await bed.step("open");
    await close(bed.page).click();
    await expect(panel(bed.page)).toHaveCount(0);
    await expect(toggle(bed.page), "focus goes back to the button that opens it").toBeFocused();
    await expect(toggle(bed.page)).toHaveAttribute("aria-expanded", "false");

    await toggle(bed.page).click();
    await bed.page.locator("#side-tab-messages").click();
    await bed.page.keyboard.press("Escape");
    await expect(panel(bed.page), "Escape closes it from the tab strip").toHaveCount(0);
  });

  test("Escape in the shell stays the shell's", async ({ bed }) => {
    await at1280(bed);
    await bed.openSeat(SEATS.imp);
    const shell = bed.mock.addTerminal(SEATS.imp.id);
    await toggle(bed.page).click();
    await bed.page.locator("#side-tab-terminal").click();
    await bed.page.locator(".pane .xterm").click();
    await bed.page.keyboard.press("Escape");
    await expect(panel(bed.page)).toBeVisible();
    await expect.poll(() => bed.mock.typed(shell).join(""), { message: "the shell got Escape" }).toContain("\x1b");
  });

  test("a panel closed for one seat stays closed for it after a reload, and another seat keeps its own", async ({ bed }) => {
    await at1280(bed);
    await bed.openSeat(SEATS.imp);
    await toggle(bed.page).click();
    await close(bed.page).click();
    await bed.switchSeat(SEATS.griffin);
    await toggle(bed.page).click();
    await expect(panel(bed.page)).toBeVisible();
    await reopenSeat(bed.page, SEATS.griffin);
    await expect(panel(bed.page), "Griffin-Goose's stays open").toBeVisible();
    await reopenSeat(bed.page, SEATS.imp);
    await expect(toggle(bed.page)).toBeVisible();
    await expect(panel(bed.page), "Imp-Dragonfly's stays closed").toHaveCount(0);
  });
});

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "the sheet below the seat"));

  test("the raised sheet has a visible close, and stays put away after a reload", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    await bed.page.locator("#side-tab-messages").click();
    await expect(panel(bed.page)).toHaveAttribute("data-open", "true");
    await expect(close(bed.page)).toBeVisible();
    const box = (await close(bed.page).boundingBox())!;
    expect(box.x + box.width, "inside the screen").toBeLessThanOrEqual(bed.page.viewportSize()!.width);
    expect(box.height, "a 44px target").toBeGreaterThanOrEqual(44);
    await bed.step("raised");
    await close(bed.page).click();
    await expect(panel(bed.page)).toHaveAttribute("data-open", "false");
    await reopenSeat(bed.page, SEATS.imp);
    await expect(panel(bed.page), "a reload keeps it put away").toHaveAttribute("data-open", "false");
  });
});

test.describe("closing the page", () => {
  test.use({ mockOptions: { browserClose: true } });

  async function browserTab(bed: Bed): Promise<void> {
    if (kindOf(test.info()) === "desktop") await toggle(bed.page).click();
    await bed.page.locator("#side-tab-browser").click();
  }

  test("a page the seat is driving asks once, and Keep it keeps it", async ({ bed }) => {
    await at1280(bed);
    bed.mock.loadPage(SEATS.imp.id, { url: "https://example.com/cart", title: "Cart" });
    await bed.openSeat(SEATS.imp);
    await browserTab(bed);
    const closePage = bed.page.getByRole("button", { name: "Close page" });
    await expect(closePage).toBeVisible();

    await closePage.click();
    const ask = bed.page.getByRole("group", { name: "Close this page?" });
    await expect(ask).toContainText("Imp-Dragonfly is using this page.");
    await expect(ask.getByRole("button", { name: "Keep it" }), "focus waits on the safe choice").toBeFocused();
    await bed.step("ask");
    await ask.getByRole("button", { name: "Keep it" }).click();
    await expect(ask).toHaveCount(0);
    expect(bed.mock.browserCalls.filter((call) => call.type === "browser_close"), "nothing was closed").toHaveLength(0);

    await closePage.click();
    await ask.getByRole("button", { name: "Close it anyway" }).click();
    await expect.poll(() => bed.mock.browserCalls.filter((call) => call.type === "browser_close").length).toBe(1);
    await expect(bed.page.getByText("You closed the page. Imp-Dragonfly's next page shows up here.")).toBeVisible();
    await expect(bed.page.locator("#side-tab-browser"), "the panel stays, on its Browser tab").toHaveAttribute("aria-selected", "true");

    bed.mock.loadPage(SEATS.imp.id, { url: "https://example.com/next", title: "Next" });
    await expect(bed.page.getByRole("button", { name: "Close page" }), "the seat's next page comes back").toBeVisible();
  });

  test("a page the person holds closes without asking", async ({ bed }) => {
    await at1280(bed);
    bed.mock.loadPage(SEATS.imp.id, { url: "https://example.com/cart", title: "Cart" });
    await bed.openSeat(SEATS.imp);
    await browserTab(bed);
    await bed.page.getByRole("button", { name: "Take control" }).click();
    await expect(bed.page.getByRole("button", { name: /Hand back/ })).toBeVisible();
    await bed.page.getByRole("button", { name: "Close page" }).click();
    await expect.poll(() => bed.mock.browserCalls.filter((call) => call.type === "browser_close").length).toBe(1);
    await expect(bed.page.getByRole("group", { name: "Close this page?" })).toHaveCount(0);
  });
});

test("a host without browser-close offers no Close page", async ({ bed }) => {
  await at1280(bed);
  bed.mock.loadPage(SEATS.imp.id, { url: "https://example.com/cart", title: "Cart" });
  await bed.openSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await toggle(bed.page).click();
  await bed.page.locator("#side-tab-browser").click();
  await expect(bed.page.getByRole("button", { name: "Take control" })).toBeVisible();
  await expect(bed.page.getByRole("button", { name: "Close page" })).toHaveCount(0);
});
