// COI-3488: a fully idle seat shows Close on its row and in its own view. A seat that
// works, waits on background work, asks, holds a message or is silent about it does not.
import type { Page } from "@playwright/test";
import { expect, kindOf, SEATS, test, seatEntry } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

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
  ref: `${n.padEnd(8, "0")}`.slice(0, 8),
  background: 0,
  ...extra,
});
const { background: _none, ...impSeat } = seat("imp", SEATS.imp.name, { role: "frontend-eng" });
const imp = { ...impSeat, name: SEATS.imp.id };
const SESSIONS = ([
  imp,
  seat("calm0001", "Calm-Ox"),
  seat("busy0001", "Busy-Ox", { state: "busy" }),
  seat("ci000001", "Ci-Ox", { background: 1 }),
  (({ background: _b, ...rest }) => rest)(seat("silent01", "Silent-Ox")),
  seat("queued01", "Queued-Ox", { pending: 1 }),
  seat("asks0001", "Asks-Ox"),
  seat("draft001", "Draft-Ox", { refuse_close: "Draft-Ox holds Kai's unsent draft. Close it with --force." }),
  seat("push0001", "Push-Ox", { unpushed: true }),
] as unknown) as SessionSpec[];

const ask = {
  type: "ask",
  ask: { id: "ask-1", session: "eng-platform-asks0001", header: "Scope", question: "Merge it?", options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: false, multi: false },
};

const openSeats = async (page: Page, phone: boolean) => {
  if (phone) await page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
};
const closeButtons = (page: Page) => page.getByRole("button", { name: /^Close [A-Z][a-z]+-Ox$/ });

test.describe("close a fully idle seat", () => {
  test.use({ mockOptions: { sessions: SESSIONS } });

  test("Close shows on the seats that are fully idle and no others", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.push(ask);
    await openSeats(bed.page, kindOf(info) === "phone");
    await expect(bed.page.getByRole("tab", { name: /Asks-Ox/ }).first()).toBeVisible();
    await expect(closeButtons(bed.page).filter({ hasText: "Close" })).toHaveCount(3);
    for (const name of ["Calm-Ox", "Draft-Ox", "Push-Ox"]) await expect(bed.page.getByRole("button", { name: `Close ${name}` })).toBeVisible();
    for (const name of ["Busy-Ox", "Ci-Ox", "Silent-Ox", "Queued-Ox", "Asks-Ox"]) await expect(bed.page.getByRole("button", { name: `Close ${name}` }), `${name} is not fully idle`).toHaveCount(0);
    await bed.step("rows", { compare: false });
  });

  test("a tap asks first, Keep leaves the seat, Close it ends it by name and ref and says how to bring it back", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await openSeats(bed.page, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Close Calm-Ox" }).click();
    await expect(bed.page.getByRole("group", { name: "Close Calm-Ox?" })).toBeVisible();
    expect(bed.mock.closes, "one tap closes nothing").toEqual([]);
    await bed.step("asking", { compare: false });
    await bed.page.getByRole("button", { name: "Keep" }).click();
    await expect(bed.page.getByRole("button", { name: "Close Calm-Ox" })).toBeVisible();
    await bed.page.getByRole("button", { name: "Close Calm-Ox" }).click();
    await bed.page.getByRole("button", { name: "Close it" }).click();
    await expect.poll(() => bed.mock.closes).toEqual(["eng-platform-calm0001@calm0001"]);
    await expect(bed.page.getByRole("tab", { name: /Calm-Ox/ })).toHaveCount(0);
    await expect(bed.page.getByRole("status").filter({ hasText: /Closed Calm-Ox\. Reopen it to pick its conversation up again, or start Platform Engineer fresh from Start\./ })).toBeVisible();
    await bed.step("closed", { compare: false });
  });

  test("the daemon's refusal is said and the seat stays", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await openSeats(bed.page, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Close Draft-Ox" }).click();
    await bed.page.getByRole("button", { name: "Close it" }).click();
    await expect(bed.page.getByRole("status").filter({ hasText: /Not closed\. Draft-Ox holds Kai's unsent draft/ })).toBeVisible();
    await expect(bed.page.getByRole("tab", { name: /Draft-Ox/ }).first()).toBeVisible();
    await bed.step("refused", { compare: false });
  });

  test("commits on no remote are said first, and Close anyway is the person's second decision", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await openSeats(bed.page, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Close Push-Ox" }).click();
    await bed.page.getByRole("button", { name: "Close it" }).click();
    await expect(bed.page.getByRole("status").filter({ hasText: /Not closed\. Push-Ox holds commits on no remote\./ })).toBeVisible();
    expect(bed.mock.forced, "the first close was not forced").toEqual([]);
    await expect(bed.page.getByRole("tab", { name: /Push-Ox/ }).first()).toBeVisible();
    await bed.step("unpushed", { compare: false });
    await bed.page.getByRole("button", { name: "Close anyway" }).click();
    await expect.poll(() => bed.mock.forced).toEqual(["eng-platform-push0001@push0001"]);
    await expect(bed.page.getByRole("tab", { name: /Push-Ox/ })).toHaveCount(0);
  });

  test("Reopen brings the closed seat back under a fresh name and opens it", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await openSeats(bed.page, kindOf(info) === "phone");
    await bed.page.getByRole("button", { name: "Close Calm-Ox" }).click();
    await bed.page.getByRole("button", { name: "Close it" }).click();
    await bed.page.getByRole("button", { name: "Reopen" }).click();
    await expect.poll(() => bed.mock.reopens).toEqual(["eng-platform-calm0001@calm0001"]);
    await expect(bed.page.getByRole("heading", { name: "Calm-Ox", level: 1 })).toBeAttached();
    await bed.step("reopened", { compare: false });
  });

  test("in its own view a fully idle seat has the same Close", async ({ bed }, info) => {
    await bed.page.goto("/");
    await bed.page.getByRole("tab", { name: /this Mac/ }).click();
    await seatEntry(bed.page, /Calm-Ox/).click();
    await expect(bed.page.getByRole("heading", { name: "Calm-Ox", level: 1 })).toBeAttached();
    const own = bed.page.locator("header").getByRole("button", { name: "Close Calm-Ox" });
    await expect(own).toBeVisible();
    await bed.step("own-view", { compare: false });
    await own.click();
    await bed.page.getByRole("button", { name: "Close it" }).click();
    await expect.poll(() => bed.mock.closes).toEqual(["eng-platform-calm0001@calm0001"]);
  });
});
