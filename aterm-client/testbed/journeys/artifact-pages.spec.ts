// COI-3330: the pages seats leave for Kai are listed, and one opens as the source it was
// published from, in a frame that runs scripts and has no origin of its own.
import { readFileSync } from "node:fs";
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { artifacts: true } });

const DESK = readFileSync(new URL("../fixtures/desk-page.html", import.meta.url), "utf8");
const page = (extra: Record<string, unknown> = {}) => ({
  id: "desk-1",
  title: "Desk",
  url: "https://claude.ai/artifact/desk-1",
  html: DESK,
  standing: true,
  waiting: true,
  ...extra,
});

const reader = (bed: Bed) => bed.page.getByRole("dialog");
const source = (bed: Bed) => bed.page.frameLocator('iframe[title$="a copy of its source"]');
const rowOf = (bed: Bed, title: string) => bed.page.getByRole("button", { name: new RegExp(`^${title}, from `) });

// The overview is where a launch rests, so that is where the list is looked for.
async function onOverview(bed: Bed): Promise<void> {
  await bed.page.goto("/");
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();
  await expect(bed.page.getByRole("heading", { name: "Seats", level: 1 })).toBeVisible();
}

test("a host with no pages shows no list", async ({ bed }) => {
  await onOverview(bed);
  await expect(bed.page.getByRole("heading", { name: /Pages for you/ })).toHaveCount(0);
});

test("a waiting page leads the overview, and it opens as its source in a script-only sandbox", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page());
  await expect(bed.page.getByRole("heading", { name: /Pages for you/ })).toBeVisible();
  const row = rowOf(bed, "Desk");
  await expect(row, "named by its title, seat and state").toHaveAccessibleName("Desk, from Beetle-Ox, waiting for you");
  await expect(row.getByText("Waiting")).toBeVisible();
  await row.click();

  await expect(reader(bed)).toBeVisible();
  const frame = bed.page.locator('iframe[title$="a copy of its source"]');
  await expect(frame, "scripts only, never the app's own origin").toHaveAttribute("sandbox", "allow-scripts");
  const doc = (await frame.getAttribute("srcdoc")) ?? "";
  expect(doc.indexOf("Content-Security-Policy"), "the policy is pinned first").toBeLessThan(doc.indexOf("<title>"));
  expect(doc).toContain("https://fonts.googleapis.com");
  expect(doc).toContain("connect-src 'none'");
  await expect(source(bed).getByRole("heading", { name: "Decisions for today" }), "the source is drawn").toBeVisible();
  await expect(source(bed).getByText("Script ran"), "its script runs").toBeVisible();
  await expect(source(bed).getByText("Storage blocked"), "and it has no storage of its own").toBeVisible();
  await bed.step("reader", { compare: false });
});

test("Open on claude.ai is the first action, and opens the real address outside", async ({ bed }) => {
  await bed.page.context().route("https://claude.ai/**", (route) => route.fulfill({ status: 200, contentType: "text/html", body: "stub" }));
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page());
  await rowOf(bed, "Desk").click();
  const buttons = reader(bed).getByRole("button");
  await expect(buttons.nth(0), "Back leads, as on every screen").toHaveText("Back");
  await expect(buttons.nth(1), "the first action is Open on claude.ai").toHaveText("Open on claude.ai");
  const opened = bed.page.context().waitForEvent("page");
  await buttons.nth(1).click();
  expect((await opened).url()).toContain("https://claude.ai/artifact/desk-1");
});

test("a page whose seat ended says so, and is not waiting", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page({ seat_ended: true }));
  const row = rowOf(bed, "Desk");
  await expect(row).toHaveAccessibleName("Desk, from Beetle-Ox, seat ended");
  await expect(row.getByText("Waiting")).toHaveCount(0);
  await row.click();
  await expect(reader(bed)).toContainText("That seat has ended");
});

test("a page that saves answers on claude.ai says to answer there", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page({ html: `${DESK}<script>window.claude?.use?.("db")</script>` }));
  await rowOf(bed, "Desk").click();
  await expect(reader(bed).getByText("Answer on the page.")).toBeVisible();
});

test("an address that is not a claude.ai artifact is not offered", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page({ url: "https://example.com/artifact/desk-1" }));
  await rowOf(bed, "Desk").click();
  await expect(reader(bed).getByRole("button", { name: "Open on claude.ai" })).toHaveCount(0);
  await expect(reader(bed)).toContainText("not a claude.ai artifact");
  await expect(source(bed).getByRole("heading", { name: "Decisions for today" }), "the source still shows").toBeVisible();
});

test("a page with no source says so and still offers the real page", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page({ html: "" }));
  await rowOf(bed, "Desk").click();
  await expect(reader(bed)).toContainText("no source to show");
  await expect(reader(bed).getByRole("button", { name: "Open on claude.ai" })).toBeVisible();
});

test("a page its seat closes while open says so, and leaves the list", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page());
  await rowOf(bed, "Desk").click();
  await expect(reader(bed)).toBeVisible();
  bed.mock.closeArtifact(SEATS.beetle.id, "desk-1");
  await expect(reader(bed)).toContainText("The seat closed this page");
  await reader(bed).getByRole("button", { name: "Back" }).click();
  await expect(reader(bed)).toHaveCount(0);
  await expect(bed.page.getByRole("heading", { name: /Pages for you/ })).toHaveCount(0);
});

test("the keyboard opens a page, keeps focus inside it, and Escape returns to the row", async ({ bed }) => {
  await onOverview(bed);
  bed.mock.addArtifact(SEATS.beetle.id, page());
  const row = rowOf(bed, "Desk");
  await row.focus();
  await bed.page.keyboard.press("Enter");
  await expect(reader(bed)).toBeVisible();
  await expect(reader(bed).getByRole("button", { name: "Back" }), "focus moves in").toBeFocused();
  for (let tab = 0; tab < 6; tab++) {
    await bed.page.keyboard.press("Tab");
    // Past the last control focus may leave for the browser, never for the page behind.
    const behind = await bed.page.evaluate(() => Boolean(document.activeElement?.closest(".shell")));
    expect(behind, `tab ${tab + 1} does not reach the page behind`).toBe(false);
  }
  // Keys inside the page's frame are the frame's, so Escape is pressed from the reader.
  await reader(bed).getByRole("button", { name: "Back" }).focus();
  await bed.page.keyboard.press("Escape");
  await expect(reader(bed)).toHaveCount(0);
  await expect(row, "focus returns to where it came from").toBeFocused();
});

test("the same list is at the top of a seat's Views tab", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.addArtifact(SEATS.beetle.id, page());
  if (kindOf(info) === "desktop") {
    const toggle = bed.page.getByRole("button", { name: /side panel/i }).first();
    if (await toggle.isVisible()) await toggle.click();
  }
  await bed.page.getByRole("tab", { name: /^Views/ }).click();
  await expect(rowOf(bed, "Desk")).toBeVisible();
  await rowOf(bed, "Desk").click();
  await expect(reader(bed)).toBeVisible();
  await bed.step("views-reader", { compare: false });
});

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "the toast is for a touch screen"));

  test("the toast for a page opens the page, not the seat that left it", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    // Asks and pages replay on attach, so the first moments are a baseline, not news.
    await bed.page.waitForTimeout(2200);
    bed.mock.addArtifact(SEATS.beetle.id, page());
    await expect(bed.page.getByText("Beetle-Ox left you a page")).toBeVisible();
    await bed.page.getByRole("button", { name: /^Open: Beetle-Ox left you a page/ }).click();
    await expect(reader(bed)).toBeVisible();
    await expect(reader(bed).getByRole("heading", { name: "Desk" })).toBeVisible();
  });
});
