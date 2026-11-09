// COI-3520: the apps ignore window.open, so an artifact or link opens through their
// opener plugin, faked here at `window.__TAURI__`.
import type { Page } from "@playwright/test";
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const ARTIFACT = "https://claude.ai/artifact/LfwfhS4hgtHYSAB5gXhKsj";
const PAGE = "https://accounts.example.com/login";
const card = (page: Page) => page.getByRole("region", { name: "A seat opened a page" });

/** The app's bridge: records each opener call, and refuses while `refuse` is set. */
async function inApp(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const w = window as unknown as { __TAURI__: unknown; opened: string[]; refuse: string };
    w.opened = [];
    w.refuse = "";
    w.__TAURI__ = {
      core: {
        invoke: async (command: string, args?: { url?: string }) => {
          if (command !== "plugin:opener|open_url") return null;
          if (w.refuse) throw { message: w.refuse };
          w.opened.push(args?.url ?? "");
          return null;
        },
      },
    };
  });
}
const opened = (page: Page) => page.evaluate(() => (window as unknown as { opened: string[] }).opened);
const driven = (bed: { mock: { browserCalls: { type: string }[] } }) => bed.mock.browserCalls.filter((call) => call.type === "browser_control" || call.type === "browser_navigate");

test("in the app an artifact card opens in the device's browser, with no second step", async ({ bed }) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: ARTIFACT });
  await expect(card(bed.page)).toContainText("claude.ai/artifact/LfwfhS4hgtHYSAB5gXhKsj");
  await expect(card(bed.page).getByRole("button", { name: "Show here", exact: false }), "the seat's own browser cannot show claude.ai").toHaveCount(0);
  await bed.step("card", { compare: false });
  await card(bed.page).getByRole("button", { name: /Open in browser/ }).click();
  await expect(card(bed.page)).toBeHidden();
  expect(await opened(bed.page)).toEqual([ARTIFACT]);
  expect(driven(bed), "the seat's streamed browser was left alone").toEqual([]);
  await expect(bed.page.getByRole("heading", { name: SEATS.griffin.name, level: 1 }), "the seat on screen did not change").toBeAttached();
});

test("when the app cannot open it the card stays, says why on screen, and works once it can", async ({ bed }) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: ARTIFACT });
  await bed.page.evaluate(() => ((window as unknown as { refuse: string }).refuse = "no app can open this"));
  await card(bed.page).getByRole("button", { name: /Open in browser/ }).click();
  const failed = bed.page.getByRole("alert").filter({ hasText: "did not open in the browser: no app can open this" });
  await expect(failed).toBeVisible();
  await expect(failed, "the reason is on screen, not behind something").toBeInViewport({ ratio: 1 });
  await expect(card(bed.page), "the card and its address stay to copy").toContainText("claude.ai/artifact/LfwfhS4hgtHYSAB5gXhKsj");
  await bed.step("refused", { compare: false });
  await bed.page.evaluate(() => ((window as unknown as { refuse: string }).refuse = ""));
  await card(bed.page).getByRole("button", { name: /Open in browser/ }).click();
  await expect(card(bed.page)).toBeHidden();
  await expect(failed, "a success clears the reason").toBeHidden();
  expect(await opened(bed.page)).toEqual([ARTIFACT]);
});

test("in the app a plain page a seat wants to drive still shows in the seat's browser", async ({ bed }) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: PAGE });
  await card(bed.page).getByRole("button", { name: /Show here/ }).click();
  await expect.poll(() => driven(bed)).toEqual([
    { type: "browser_control", session: SEATS.beetle.id, take: true },
    { type: "browser_navigate", session: SEATS.beetle.id, url: PAGE },
  ]);
  expect(await opened(bed.page)).toEqual([]);
});

test("in a browser tab an artifact card is unchanged", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: ARTIFACT });
  await expect(card(bed.page).getByRole("button", { name: /Show here/ })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Open in browser/ })).toHaveCount(0);
});

test("a link in a seat's terminal opens through the app", async ({ bed }, info) => {
  await inApp(bed.page);
  await bed.openSeat(SEATS.imp);
  // On its own line and short, so it does not wrap across rows at 320px.
  pushOutput(bed.mock, SEATS.imp.id, "\r\nhttps://claude.ai/artifact/T1\r\n");
  const link = bed.page.locator(".xterm-rows").getByText("https://claude.ai/artifact/T1").first();
  await expect(link).toBeVisible();
  // The rows redraw, so the link is never "stable" to a locator click. Go to where it is.
  const box = (await link.boundingBox())!;
  const [x, y] = [box.x + box.width / 2, box.y + box.height / 2];
  if (kindOf(info) === "phone") {
    // A phone moves the pointer before the tap, and xterm finds the link then.
    await bed.page.mouse.move(x, y);
    await bed.page.touchscreen.tap(x, y);
  } else {
    const key = process.platform === "darwin" ? "Meta" : "Control";
    await bed.page.mouse.move(x, y);
    await bed.page.keyboard.down(key);
    await bed.page.mouse.click(x, y);
    await bed.page.keyboard.up(key);
  }
  await expect.poll(() => opened(bed.page)).toEqual(["https://claude.ai/artifact/T1"]);
});
