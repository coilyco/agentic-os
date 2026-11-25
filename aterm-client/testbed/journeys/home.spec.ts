// COI-3518: a host with no seat open rests on the overview. The logo is its own screen.
import type { Locator } from "@playwright/test";
import { card, expect, loadFonts, SEATS, test } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

test("a host with no seat open shows the seats by what they want of you, and a seat opens from there", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const overview = page.getByRole("region", { name: "Seats", exact: true });
  await expect(overview, "the overview").toBeVisible();
  // Every group heading present is one of the three, in order.
  const headings = await overview.getByRole("heading", { level: 2 }).allTextContents();
  const order = ["Needs you", "Working", "Done"];
  const seen = headings.map((text) => order.findIndex((title) => text.startsWith(title)));
  expect(seen.every((index) => index >= 0), `headings ${headings.join(" | ")}`).toBe(true);
  expect(seen, "in the order needs you, working, done").toEqual([...seen].sort((a, b) => a - b));
  await expect(overview.getByRole("button", { name: new RegExp(SEATS.imp.name) }), "a seat row").toBeVisible();
  await expect(page.getByText("Pick a seat from the list to open its terminal."), "the old host page is gone").toHaveCount(0);
  await page.mouse.move(0, 0);
  await page.screenshot({ path: info.outputPath("overview.png") });
  // A seat opens from a row, and Back to the list returns here.
  await overview.getByRole("button", { name: new RegExp(SEATS.imp.name) }).click();
  await expect(page.locator(".xterm-rows")).toBeVisible();
  await expect(overview, "the overview gives way to the seat").toHaveCount(0);
});

// COI-3740: the coil, then the canopy, then the words. Returns the marks in order.
async function byline(scope: Locator): Promise<{ marks: string[]; words: number; rest: number }> {
  return scope.evaluate((root) => {
    const by = root.querySelector(".by")!;
    const marks = [...by.querySelectorAll("img")].map((img) => img.classList[0]);
    const words = by.querySelector(".words")!;
    const size = (el: Element) => parseFloat(getComputedStyle(el).fontSize);
    const rest = [...root.querySelectorAll("*")].filter((el) => !by.contains(el) && el.childElementCount === 0 && el.textContent?.trim());
    return { marks, words: size(words), rest: Math.max(0, ...rest.map(size)) };
  });
}

test("Treehouse by coilyco is the largest thing on the seat overview, coil then canopy", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const overview = page.getByRole("region", { name: "Seats", exact: true });
  const brand = overview.getByRole("button", { name: /^Treehouse by coilyco/ });
  await expect(brand, "the byline opens the overview, above the fold").toBeInViewport();
  const read = await byline(overview);
  expect(read.marks, "the coilyco coil, then the Treehouse canopy").toEqual(["coil", "canopy"]);
  expect(read.words, `${read.words}px, past every other line's ${read.rest}px`).toBeGreaterThan(read.rest);
  const images = brand.locator("img");
  const [coil, canopy] = [(await images.nth(0).boundingBox())!, (await images.nth(1).boundingBox())!];
  expect(coil.x + coil.width, "the coil sits left of the canopy").toBeLessThanOrEqual(canopy.x);
  expect(await images.evaluateAll((imgs) => imgs.every((img) => (img as HTMLImageElement).complete && (img as HTMLImageElement).naturalWidth > 0)), "both marks loaded").toBe(true);
  for (const colorScheme of ["dark", "light"] as const) {
    await page.emulateMedia({ colorScheme });
    await page.mouse.move(0, 0);
    await page.screenshot({ path: info.outputPath(`overview-byline-${colorScheme}.png`) });
  }
});

test("the logo screen is the pair and the words, and one tap returns to the seats", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.getByRole("button", { name: /^Treehouse by coilyco\. Logo screen/ }).click();
  const home = page.getByRole("region", { name: "Treehouse" });
  await expect(home, "the logo screen").toBeVisible();
  expect(await home.innerText(), "the words and nothing else").toBe("Treehouse by coilyco");
  expect((await byline(home)).marks, "coil, then canopy").toEqual(["coil", "canopy"]);
  for (const logo of await home.locator("img").all()) {
    expect(await logo.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0), "the mark loaded").toBe(true);
    const box = (await logo.boundingBox())!;
    expect(box.width, `${info.project.name}: a readable size`).toBeGreaterThanOrEqual(96);
    expect(box.x + box.width, "inside the screen").toBeLessThanOrEqual(page.viewportSize()!.width);
  }
  await page.mouse.move(0, 0);
  await page.screenshot({ path: info.outputPath("logo.png") });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("heading", { name: "Seats", level: 1 }), "Escape returns to the seats").toBeVisible();
});

const seat = (n: string, identity: string, extra: Record<string, unknown> = {}) => ({
  name: `eng-platform-${n}`,
  role: "eng-platform",
  identity,
  seat: "claude",
  ready: true,
  bracketed_paste: true,
  kai_drafting: false,
  pending: 0,
  scene: "claude-idle",
  state: "idle",
  ref: n.padEnd(8, "0").slice(0, 8),
  background: 0,
  ...extra,
});
const MIXED = [
  seat("asks0001", "Asks-Ox", { activity: "Waiting on a merge call", activity_age_seconds: 30 }),
  // Not ready is how the client reads a seat as working: ready and silent is idle.
  seat("busy0001", "Busy-Ox", { ready: false, activity: "Rebuilding the gate", activity_age_seconds: 30, state_seconds: 360 }),
  seat("calm0001", "Calm-Ox", { activity: "Pushed the branch", activity_age_seconds: 30, state_seconds: 600 }),
] as unknown as SessionSpec[];

test.describe("seats in all three states", () => {
  test.use({ mockOptions: { sessions: MIXED } });

  test("a seat that asks moves to needs you with Answer, above the one working and the one done", async ({ page, mock }, info) => {
    await page.goto("/");
    await loadFonts(page);
    await page.getByRole("tab", { name: /this Mac/ }).click();
    const overview = page.getByRole("region", { name: "Seats", exact: true });
    await expect(overview.getByRole("heading", { level: 2 })).toHaveText([/^Working/, /^Done/]);
    mock.push({
      type: "ask",
      ask: { id: "ask-1", session: "eng-platform-asks0001", header: "Merge", question: "Merge the fix?", options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: false, multi: false },
    });
    await expect(overview.getByRole("heading", { level: 2 })).toHaveText([/^Needs you/, /^Working/, /^Done/]);
    const rows = overview.getByRole("listitem");
    await expect(rows.nth(0)).toContainText("Asks-Ox");
    await expect(rows.nth(0)).toContainText("Answer");
    await expect(rows.nth(1)).toContainText("Busy-Ox");
    await expect(rows.nth(2)).toContainText("Calm-Ox");
    await page.mouse.move(0, 0);
    await page.screenshot({ path: info.outputPath("overview-mixed.png") });
    // A tap on the row opens the seat with its question on the screen.
    await rows.nth(0).getByRole("button").first().click();
    await expect(card(page)).toBeVisible();
  });
});
