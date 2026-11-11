// COI-3346: switch a role off in Settings and its quiet seats leave the list. A seat that
// asks, is pinned or is open stays, a note counts the hidden, a reload keeps the choice.
import type { Page } from "@playwright/test";
import { expect, kindOf, SEATS, test, listedSeats } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

const seat = (name: string, role: string, identity: string) =>
  ({ name: `${role}-${name}`, role, identity, seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, scene: "claude-idle" }) as unknown as SessionSpec;
const engineers = Array.from({ length: 12 }, (_, n) => seat(`e${String(n + 1).padStart(2, "0")}`, "eng-platform", `Eng-${n + 1}`));
const SESSIONS = [
  { ...seat("imp", "frontend-eng", SEATS.imp.name), name: SEATS.imp.id },
  ...engineers,
  seat("d1", "prod-director", "Dir-1"),
  seat("d2", "prod-director", "Dir-2"),
];
const ask = {
  type: "ask",
  ask: { id: "ask-1", session: "eng-platform-e05", header: "Scope", question: "Merge it?", options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: false, multi: false },
};

const ids = (page: Page) => listedSeats(page);
const settings = (page: Page) => page.getByRole("region", { name: "Settings" });
const role = (page: Page) => settings(page).getByRole("switch", { name: /Platform Engineer/ });
// A phone's list is a sheet over the seat, and Settings is its own page: Back returns.
const openList = async (page: Page, phone: boolean) => {
  if (phone) await page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ }).click();
};
const openSettings = async (page: Page, phone: boolean) => {
  if (phone) {
    if (!(await page.getByText("Hosts and settings").isVisible())) await openList(page, phone);
    await page.getByText("Hosts and settings").click();
    await page.getByRole("button", { name: "Settings", exact: true }).click();
  } else {
    await page.getByRole("navigation", { name: "Hosts and sessions" }).getByRole("button", { name: "Settings", exact: true }).click();
  }
};
const backToList = async (page: Page, phone: boolean) => {
  if (!phone) return;
  await settings(page).getByRole("button", { name: "Back" }).click();
  await openList(page, phone);
};
const hiddenEngineers = (all: string[]) => engineers.filter((each) => !all.includes(each.name));

test.describe("hide a role", () => {
  test.use({ mockOptions: { sessions: SESSIONS } });

  test("the engineers leave the list, the one holding a question stays, and a note counts the rest", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    bed.mock.push(ask);
    await openSettings(bed.page, phone);
    await role(bed.page).uncheck();
    await backToList(bed.page, phone);
    await expect(bed.page.getByRole("status").filter({ hasText: "11 seats are hidden." })).toBeVisible();
    const left = await ids(bed.page);
    expect(hiddenEngineers(left).length, "eleven engineers are out of the list").toBe(11);
    expect(left, "the engineer asking stays").toContain("eng-platform-e05");
    expect(left, "the directors stay").toEqual(expect.arrayContaining(["prod-director-d1", "prod-director-d2"]));
    await bed.step("hidden", { compare: false });
  });

  test("a reload keeps the choice, and Show them brings every engineer back", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await openSettings(bed.page, phone);
    await role(bed.page).uncheck();
    await backToList(bed.page, phone);
    await expect(bed.page.getByRole("status").filter({ hasText: "12 seats are hidden." })).toBeVisible();
    await bed.page.reload();
    await expect(bed.page.getByRole("heading", { name: "Seats", level: 1 })).toBeVisible();
    await expect(bed.page.getByRole("status").filter({ hasText: "12 seats are hidden." }).first(), "still hidden after a reload").toBeVisible();
    await bed.page.getByRole("button", { name: "Show them" }).first().click();
    await expect.poll(async () => hiddenEngineers(await ids(bed.page)).length).toBe(0);
    // Settings holds the switch, and it follows the button.
    await bed.page.getByRole("button", { name: "Settings", exact: true }).first().click();
    await expect(role(bed.page), "the switch is back on").toBeChecked();
  });

  test("a pinned seat of a hidden role stays in the list", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await openList(bed.page, phone);
    await bed.page.locator(".seat-row").filter({ hasText: "Eng-3" }).getByRole("button", { name: "Pin to the top" }).click();
    await openSettings(bed.page, phone);
    await role(bed.page).uncheck();
    await backToList(bed.page, phone);
    await expect(bed.page.getByRole("status").filter({ hasText: "11 seats are hidden." })).toBeVisible();
    expect(await ids(bed.page)).toContain("eng-platform-e03");
  });
});
