// What every journey shares: a mock daemon per test, the client pointed at it,
// and a step helper that saves a screenshot and compares it to its baseline.
import { test as base, expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import { startMock, type Mock } from "./mock-daemon.ts";

export { expect };

export type Kind = "desktop" | "phone";

export const kindOf = (info: TestInfo): Kind => info.project.metadata.kind as Kind;

export const SEATS = {
  imp: { id: "frontend-eng-imp-dragonfly", name: "Imp-Dragonfly" },
  beetle: { id: "eng-platform-beetle-ox", name: "Beetle-Ox" },
  griffin: { id: "prod-director-griffin-goose", name: "Griffin-Goose" },
  turtle: { id: "sysadmin-senior-turtle-ox", name: "Turtle-Ox" },
} as const;
export type Seat = (typeof SEATS)[keyof typeof SEATS];

export interface Bed {
  mock: Mock;
  page: Page;
  /** Open the client at the mock, pick "this Mac", and open a seat. */
  openSeat(seat: Seat): Promise<void>;
  /** Move to another seat as this screen does: the sidebar, or a phone's seat sheet. */
  switchSeat(seat: Seat): Promise<void>;
  /** The terminal's rows as text, for asserting what a person can read. */
  screen(): Locator;
  /** Save a numbered screenshot and compare it to the baseline of the same name. */
  step(name: string, options?: { mask?: Locator[]; compare?: boolean }): Promise<void>;
  /** The terminal's columns, as the PTY was last sized for this seat. */
  cols(seat: Seat): number;
}

export const test = base.extend<{ bed: Bed; mock: Mock }>({
  mock: async ({}, use) => {
    const mock = await startMock();
    await use(mock);
    await mock.stop();
  },
  baseURL: async ({ mock }, use) => {
    await use(mock.url);
  },
  bed: async ({ page, mock }, use, info) => {
    let count = 0;
    const phone = kindOf(info) === "phone";
    const bed: Bed = {
      mock,
      page,
      async openSeat(seat) {
        await page.goto("/");
        await page.evaluate(() => document.fonts.ready);
        await page.getByRole("tab", { name: /this Mac/ }).click();
        await page.getByRole("tab", { name: new RegExp(seat.name) }).first().click();
        await expect(page.locator(".xterm-rows")).toBeVisible();
      },
      async switchSeat(seat) {
        if (phone) await page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
        await page.getByRole("tab", { name: new RegExp(seat.name) }).first().click();
      },
      screen: () => page.locator(".xterm-rows").first(),
      cols: (seat) => mock.boxOf(seat.id)?.cols ?? 0,
      async step(name, options) {
        count += 1;
        const file = `${String(count).padStart(2, "0")}-${name}.png`;
        await page.evaluate(() => document.fonts.ready);
        // Resizes return from the mock as redraws. Look once the client is quiet.
        await expect.poll(() => mock.idleMs(), { timeout: 10000 }).toBeGreaterThan(500);
        await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
        // The terminal anchors to its bottom row on timers after the crop.
        await page.waitForTimeout(700);
        // Every run keeps its own pictures, pass or fail, for a person to look at.
        await page.screenshot({ path: info.outputPath(file) });
        // ATERM_TESTBED_COMPARE=0 judges a fix's behavior and ignores how it now looks.
        if (options?.compare === false || process.env.ATERM_TESTBED_COMPARE === "0") return;
        await expect(page).toHaveScreenshot(file, { mask: options?.mask ?? [] });
      },
    };
    await use(bed);
  },
});

/** Marks a journey a reported quirk: red until its fix lands. See the walkthrough. */
export const quirk = (ref: string) => ({ tag: "@quirk", annotation: { type: "quirk", description: ref } });

/** The dialog card a native choice or an ask puts over the terminal. */
export const card = (page: Page) => page.locator("section.card");

/** Raw terminal output into a seat, as the daemon pushes it. */
export function pushOutput(mock: Mock, session: string, text: string): void {
  mock.push({ type: "output", session, data: Buffer.from(text, "utf8").toString("base64") });
}
