// COI-3346: switch a role off and its quiet seats leave the list. A seat that asks, is
// pinned or is open stays, a note counts the hidden, and a reload keeps the choice.
import type { Page } from "@playwright/test";
import { expect, kindOf, SEATS, test } from "../harness.ts";
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

const ids = (page: Page) => page.locator('[id^="tab-session-"]').evaluateAll((tabs) => tabs.map((tab) => tab.getAttribute("id")!.replace("tab-session-", "")));
const openMenu = async (page: Page, phone: boolean) => {
  if (phone) {
    await page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
    await page.getByText("Alerts and hosts").click();
  }
};
const hiddenEngineers = (all: string[]) => engineers.filter((each) => !all.includes(each.name));

test.describe("hide a role", () => {
  test.use({ mockOptions: { sessions: SESSIONS } });

  test("the engineers leave the list, the one holding a question stays, and a note counts the rest", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    bed.mock.push(ask);
    await openMenu(bed.page, phone);
    await expect(bed.page.getByRole("tab", { name: /Eng-1,/ }).first()).toBeVisible();
    await bed.page.getByRole("switch", { name: /Platform Engineer/ }).uncheck();
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
    await openMenu(bed.page, phone);
    await bed.page.getByRole("switch", { name: /Platform Engineer/ }).uncheck();
    await expect(bed.page.getByRole("status").filter({ hasText: "12 seats are hidden." })).toBeVisible();
    await bed.page.reload();
    await expect(bed.page.locator(".xterm-rows")).toBeVisible();
    await openMenu(bed.page, phone);
    await expect(bed.page.getByRole("status").filter({ hasText: "12 seats are hidden." }), "still hidden after a reload").toBeVisible();
    await expect(bed.page.getByRole("switch", { name: /Platform Engineer/ })).not.toBeChecked();
    await bed.page.getByRole("button", { name: "Show them" }).click();
    await expect.poll(async () => hiddenEngineers(await ids(bed.page)).length).toBe(0);
    await expect(bed.page.getByRole("switch", { name: /Platform Engineer/ })).toBeChecked();
  });

  test("a pinned seat of a hidden role stays in the list", async ({ bed }, info) => {
    const phone = kindOf(info) === "phone";
    await bed.openSeat(SEATS.imp);
    await openMenu(bed.page, phone);
    await bed.page.locator(".seat-row").getByRole("button", { name: "Pin Eng-3" }).click();
    await bed.page.getByRole("switch", { name: /Platform Engineer/ }).uncheck();
    await expect(bed.page.getByRole("status").filter({ hasText: "11 seats are hidden." })).toBeVisible();
    expect(await ids(bed.page)).toContain("eng-platform-e03");
  });
});
