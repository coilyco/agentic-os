// COI-3258: a seat's page waits in a card, and a click loads it in that seat's browser.
import { expect, SEATS, test } from "../harness.ts";

const URL = "https://accounts.example.com/login?state=SECRET";
const card = (bed: { page: import("@playwright/test").Page }) => bed.page.getByRole("region", { name: "A seat opened a page" });
const driven = (bed: { mock: { browserCalls: { type: string }[] } }) => bed.mock.browserCalls.filter((call) => call.type === "browser_control" || call.type === "browser_navigate");

test("a seat's page waits in a card and moves nothing", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  await expect.poll(() => bed.mock.browserCalls.some((call) => call.type === "subscribe" && call.channel === "open_url"), "the client says it can show a page").toBe(true);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: URL });
  await expect(card(bed)).toBeVisible();
  await expect(card(bed)).toContainText("Beetle-Ox opened a page");
  await expect(card(bed)).toContainText("accounts.example.com/login?…");
  await expect(card(bed), "a login's state is never shown").not.toContainText("SECRET");
  await expect(bed.page.getByRole("heading", { name: SEATS.griffin.name, level: 1 })).toBeAttached();
  expect(driven(bed), "nothing took control before a click").toEqual([]);
  await bed.step("card", { compare: false });
});

test("Show here switches to that seat, shows its browser, takes control and loads the page", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: URL });
  await card(bed).getByRole("button", { name: /Show here/ }).click();
  await expect(card(bed)).toBeHidden();
  await expect(bed.page.getByRole("heading", { name: SEATS.beetle.name, level: 1 })).toBeAttached();
  await expect(bed.page.getByRole("tab", { name: "Browser" }), "the seat's browser tab is the one showing").toHaveAttribute("aria-selected", "true");
  await expect.poll(() => driven(bed), "control is taken, then the page is loaded").toEqual([
    { type: "browser_control", session: SEATS.beetle.id, take: true },
    { type: "browser_navigate", session: SEATS.beetle.id, url: URL },
  ]);
  await expect(bed.page.getByLabel("Page address")).toHaveValue(URL);
  await bed.step("loaded", { compare: false });
});

test("Not now clears the card and touches nothing", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: URL });
  await card(bed).getByRole("button", { name: "Not now" }).click();
  await expect(card(bed)).toBeHidden();
  expect(driven(bed)).toEqual([]);
});

test("a second link from the same seat is one card, and two seats count behind the newest", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: URL });
  bed.mock.push({ type: "open_url", session: SEATS.beetle.id, url: URL });
  await expect(card(bed)).toContainText("Beetle-Ox opened a page");
  await expect(card(bed)).not.toContainText("more");
  bed.mock.push({ type: "open_url", session: SEATS.turtle.id, url: "https://docs.example.com/" });
  await expect(card(bed)).toContainText("Turtle-Ox opened a page, and 1 more is waiting");
});
