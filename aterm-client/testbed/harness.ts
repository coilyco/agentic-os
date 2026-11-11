// What every journey shares: a mock daemon per test, the client pointed at it,
// and a step helper that saves a screenshot and compares it to its baseline.
import { test as base, expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import { startMock, type Mock, type MockOptions } from "./mock-daemon.ts";

export { expect };

/** The seats on screen in list order: a desktop's list tabs, else the overview's rows. */
export async function listedSeats(page: Page): Promise<string[]> {
  const tabs = await page.locator('[id^="tab-session-"]').evaluateAll((all) => all.map((tab) => tab.getAttribute("id")!.replace("tab-session-", "")));
  return tabs.length ? tabs : page.locator("[data-seat]").evaluateAll((rows) => rows.map((row) => row.getAttribute("data-seat")!));
}

/** A seat as found with none open: its tab on a desktop, else its overview row. */
export function seatEntry(page: Page, who: string | RegExp): Locator {
  const name = typeof who === "string" ? new RegExp(who) : who;
  return page.getByRole("tab", { name }).or(page.getByRole("region", { name: "Seats", exact: true }).getByRole("button", { name })).first();
}

export type Kind = "desktop" | "phone" | "landscape";

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
  step(name: string, options?: { mask?: Locator[]; compare?: boolean; hide?: string[] }): Promise<void>;
  /** The terminal's columns, as the PTY was last sized for this seat. */
  cols(seat: Seat): number;
}

export const test = base.extend<{ bed: Bed; mock: Mock; mockOptions: MockOptions }>({
  // A journey sets it with test.use({ mockOptions: { images: true } }).
  mockOptions: [{}, { option: true }],
  mock: async ({ mockOptions }, use) => {
    const mock = await startMock(mockOptions);
    await use(mock);
    await mock.stop();
  },
  baseURL: async ({ mock }, use) => {
    await use(mock.url);
  },
  bed: async ({ page, mock }, use, info) => {
    let count = 0;
    const phone = kindOf(info) !== "desktop";
    const bed: Bed = {
      mock,
      page,
      async openSeat(seat) {
        await page.goto("/");
        await loadFonts(page);
        await page.getByRole("tab", { name: /this Mac/ }).click();
        await seatEntry(page, seat.name).click();
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
        await loadFonts(page);
        // A pointer left over an option paints its hover style on a timer. Park it.
        await page.mouse.move(0, 0);
        // Resizes return from the mock as redraws. Look once the client is quiet.
        await expect.poll(() => mock.idleMs(), { timeout: 10000 }).toBeGreaterThan(500);
        await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
        // The terminal anchors to its bottom row on timers after the crop.
        await page.waitForTimeout(700);
        // Every run keeps its own pictures, pass or fail, for a person to look at.
        await page.screenshot({ path: info.outputPath(file) });
        // ATERM_TESTBED_COMPARE=0 judges a fix's behavior and ignores how it now looks.
        if (options?.compare === false || process.env.ATERM_TESTBED_COMPARE === "0") return;
        // `hide` keeps a floating control out of the picture, as the baseline has it.
        const hidden = (options?.hide ?? []).map((selector) => `${selector} { visibility: hidden !important }`).join("\n");
        const sheet = hidden ? await page.addStyleTag({ content: hidden }) : undefined;
        try {
          await expect(page).toHaveScreenshot(file, { mask: options?.mask ?? [] });
        } finally {
          await sheet?.evaluate((node) => (node as HTMLElement).remove());
        }
      },
    };
    await use(bed);
  },
});

/** Load every Latin font face, since a face the page has not drawn yet loads late. */
export const loadFonts = (page: Page) =>
  page.evaluate(async () => {
    const faces = [...document.fonts].filter((face) => face.unicodeRange.includes("U+0000"));
    await Promise.all(faces.map((face) => face.load().catch(() => face)));
    await document.fonts.ready;
  });

/** Marks a journey a reported quirk: red until its fix lands. See the walkthrough. */
export const quirk = (ref: string) => ({ tag: "@quirk", annotation: { type: "quirk", description: ref } });

/** The dialog card a native choice or an ask puts over the terminal. */
export const card = (page: Page) => page.locator("section.card");

/** Raw terminal output into a seat, as the daemon pushes it. */
export function pushOutput(mock: Mock, session: string, text: string): void {
  mock.push({ type: "output", session, data: Buffer.from(text, "utf8").toString("base64") });
}
