// COI-3518: a host with no seat open rests on the overview. The logo is its own screen.
import { card, expect, loadFonts, SEATS, test } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

test("a host with no seat open shows the seats by what they want of you, and a seat opens from there", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const overview = page.getByRole("region", { name: "Seats" });
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

test("the logo screen is the logo and nothing else, and one tap returns to the seats", async ({ page }, info) => {
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.getByRole("button", { name: "Treehouse logo screen" }).click();
  const home = page.getByRole("region", { name: "Treehouse" });
  await expect(home, "the logo screen").toBeVisible();
  const logo = home.locator("img");
  expect(await logo.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0), "the logo loaded").toBe(true);
  expect(await home.innerText(), "no words on it").toBe("");
  const box = (await logo.boundingBox())!;
  expect(box.width, `${info.project.name}: a readable size`).toBeGreaterThanOrEqual(120);
  expect(box.x + box.width, "inside the screen").toBeLessThanOrEqual(page.viewportSize()!.width);
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
    const overview = page.getByRole("region", { name: "Seats" });
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
