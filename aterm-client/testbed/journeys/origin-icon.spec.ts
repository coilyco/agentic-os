// COI-3504: a small icon says where a seat started. Unknown shows none.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

const seat = (n: string, identity: string, extra: Record<string, unknown> = {}) =>
  ({ name: `eng-platform-${n}`, role: "eng-platform", identity, seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, scene: "claude-idle", ...extra }) as unknown as SessionSpec;
const SESSIONS = [
  { ...seat("imp", SEATS.imp.name, { role: "frontend-eng", origin: "window" }), name: SEATS.imp.id },
  seat("win00001", "Window-Ox", { origin: "window" }),
  seat("head0001", "Headless-Ox", { origin: "headless" }),
  seat("unknown1", "Unknown-Ox"),
];
const WINDOW = "started in a terminal window";
const NONE = "started with no window";

test.describe("where a seat started", () => {
  test.use({ mockOptions: { sessions: SESSIONS } });

  test("each row carries its icon with a label, and a seat with no field carries none", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    if (kindOf(info) === "phone") await bed.page.getByRole("button", { name: /Seats, starting a seat, hosts and alerts/ }).click();
    const row = (name: string) => bed.page.getByRole("tab", { name: new RegExp(name) }).first();
    await expect(row("Window-Ox")).toHaveAccessibleName(new RegExp(WINDOW));
    await expect(row("Headless-Ox")).toHaveAccessibleName(new RegExp(NONE));
    await expect(row("Window-Ox").locator("svg.origin")).toHaveAttribute("data-origin", "window");
    await expect(row("Headless-Ox").locator("svg.origin")).toHaveAttribute("data-origin", "headless");
    await expect(row("Unknown-Ox").locator("svg.origin"), "unknown shows no icon").toHaveCount(0);
    await expect(row("Unknown-Ox")).not.toHaveAccessibleName(/started/);
    await bed.step("rows", { compare: false });
  });

  test("the open seat's header carries the icon too, and it does not move when it is opened from elsewhere", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    const shown = kindOf(info) === "phone" ? bed.page.locator(".switcher svg.origin") : bed.page.locator("header svg.origin");
    await expect(shown).toHaveAttribute("data-origin", "window");
    await bed.switchSeat({ id: "eng-platform-head0001", name: "Headless-Ox" } as unknown as Parameters<typeof bed.switchSeat>[0]);
    await expect(shown).toHaveAttribute("data-origin", "headless");
    await expect(shown).toHaveAccessibleName(NONE);
    await bed.step("header", { compare: false });
  });
});
