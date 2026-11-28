// COI-3803: Cloudflare's check never passes in a seat's streamed browser, so a sign-in
// page and a challenge page open in the device's own browser instead.
import type { Page } from "@playwright/test";
import { expect, SEATS, test } from "../harness.ts";

const AUTHORIZE = "https://claude.ai/oauth/authorize?code=true&state=SECRET";
const CHECKED = "https://shop.example.com/cart";
const card = (page: Page) => page.getByRole("region", { name: "A seat opened a page" });
const driven = (bed: { mock: { browserCalls: { type: string }[] } }) => bed.mock.browserCalls.filter((call) => call.type === "browser_control" || call.type === "browser_navigate");

/** The app's opener bridge, recording each address it was asked to open. */
async function inApp(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const w = window as unknown as { __TAURI__: unknown; opened: string[] };
    w.opened = [];
    w.__TAURI__ = { core: { invoke: async (command: string, args?: { url?: string }) => void (command === "plugin:opener|open_url" && w.opened.push(args?.url ?? "")) } };
  });
}
const opened = (page: Page) => page.evaluate(() => (window as unknown as { opened: string[] }).opened);

test("a seat's sign-in page offers only the device's browser, in the app", async ({ bed }) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: AUTHORIZE });
  await expect(card(bed.page)).toContainText("claude.ai/oauth/authorize?…");
  await expect(card(bed.page).getByRole("button", { name: /Show here/ }), "the seat's browser can never pass the check").toHaveCount(0);
  await card(bed.page).getByRole("button", { name: /Open in browser/ }).click();
  await expect(card(bed.page)).toBeHidden();
  expect(await opened(bed.page)).toEqual([AUTHORIZE]);
  expect(driven(bed), "the seat's streamed browser was left alone").toEqual([]);
});

test("a seat's sign-in page offers only a new tab, in a browser", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: AUTHORIZE });
  await expect(card(bed.page).getByRole("button", { name: /Open in browser/ })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Show here/ })).toHaveCount(0);
  const tab = bed.page.context().waitForEvent("page");
  await card(bed.page).getByRole("button", { name: /Open in browser/ }).click();
  expect((await tab).url()).toBe(AUTHORIZE);
  expect(driven(bed)).toEqual([]);
});

test("a Cloudflare check in the pane shows one button that opens the page in the device's browser", async ({ bed }) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.beetle);
  // A wide screen may keep the side panel folded away, so open it first.
  const side = bed.page.getByRole("button", { name: /^Side panel/ });
  if ((await side.count()) && (await side.getAttribute("aria-expanded")) === "false") await side.click();
  await bed.page.getByRole("tab", { name: /^Browser/ }).click();
  await expect.poll(() => bed.mock.browserCalls.some((call) => call.type === "browser_watch")).toBe(true);
  const check = bed.page.getByRole("group", { name: "Cloudflare check" });
  await expect(check, "an ordinary page has no button").toHaveCount(0);
  bed.mock.loadPage(SEATS.beetle.id, { url: CHECKED, title: "Just a moment...", cloudflareCheck: true });
  await expect(check).toBeVisible();
  await expect(check.getByRole("button")).toHaveCount(1);
  await bed.step("challenge", { compare: false });
  await check.getByRole("button", { name: "Open in your browser" }).click();
  await expect.poll(() => opened(bed.page)).toEqual([CHECKED]);
  expect(driven(bed), "the pane never clicks the check or takes control").toEqual([]);
  bed.mock.loadPage(SEATS.beetle.id, { url: CHECKED, title: "Cart" });
  await expect(check, "the button leaves with the check").toHaveCount(0);
});
