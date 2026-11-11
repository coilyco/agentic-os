// COI-3519: the open seat's top bar carries its activity line under its name on a screen
// wide enough for it. A phone's bar keeps its one row, so the terminal keeps every row.
import { expect, kindOf, SEATS, test, seatEntry } from "../harness.ts";
import type { SessionSpec } from "../mock-daemon.ts";

const LONG = "Checking whether a card's text box keeps focus on a phone";
const seat = (name: string, identity: string, extra: Record<string, unknown>) =>
  ({ name, role: "eng-platform", identity, seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, scene: "claude-idle", ...extra }) as unknown as SessionSpec;
const FRESH = seat("eng-platform-fresh-ox", "Fresh-Ox", { activity: LONG, activity_age_seconds: 40 });
const STALE = seat("eng-platform-stale-ox", "Stale-Ox", { activity: "Waiting for the daemon to come back", activity_age_seconds: 3 * 3600 });
const NONE = seat("eng-platform-none-ox", "None-Ox", {});
// Its line was set two minutes ago and its state began half a minute ago.
const MOVED = seat("eng-platform-moved-ox", "Moved-Ox", { activity: "Running the gate", activity_age_seconds: 120, state_seconds: 30 });

const open = async (bed: import("../harness.ts").Bed, name: string) => {
  await bed.page.goto("/");
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();
  await seatEntry(bed.page, name).click();
  await expect(bed.page.getByRole("heading", { name, level: 1 })).toBeAttached();
};

test.describe("the activity line in the top bar", () => {
  test.use({ mockOptions: { sessions: [FRESH, STALE, NONE, MOVED, { ...seat(SEATS.imp.id, SEATS.imp.name, { role: "frontend-eng" }) }] } });

  test("a fresh line sits under the name on one line, whole on hover, and follows the seat", async ({ bed }, info) => {
    await open(bed, "Fresh-Ox");
    const line = bed.page.locator("header .name-block .activity");
    if (kindOf(info) !== "desktop") {
      await expect(line, "a phone's bar keeps its one row").toHaveCount(0);
      await expect(bed.page.locator(".switcher .activity")).toHaveCount(0);
      return;
    }
    await expect(line.locator(".line")).toHaveText(LONG);
    await expect(line).toHaveAttribute("title", LONG);
    await expect(line).toHaveAttribute("data-stale", "false");
    expect((await line.boundingBox())!.height, "one line").toBeLessThan(22);
    const name = await bed.page.getByRole("heading", { name: "Fresh-Ox", level: 1 }).boundingBox();
    expect((await line.boundingBox())!.y, "under the name").toBeGreaterThan(name!.y + name!.height - 2);
    await bed.step("fresh", { compare: false });
    bed.mock.addSession({ ...FRESH, activity: "Running the gate", activity_age_seconds: 1 } as SessionSpec);
    await expect(line.locator(".line"), "updates with no reload").toHaveText("Running the gate");
  });

  test("an old line says how old, and a seat with none leaves no space", async ({ bed }, info) => {
    test.skip(kindOf(info) !== "desktop", "the line is for a wide bar");
    await open(bed, "Stale-Ox");
    const stale = bed.page.locator("header .name-block .activity");
    await expect(stale).toHaveAttribute("data-stale", "true");
    await expect(stale.locator(".age")).toHaveText("3h ago");
    await expect(stale).toHaveAttribute("title", /\(set 3h ago\)/);
    await open(bed, "Moved-Ox");
    const moved = bed.page.locator("header .name-block .activity");
    await expect(moved, "a line from before the present state is not current").toHaveAttribute("data-stale", "true");
    await expect(moved.locator(".age")).toHaveText("2m ago");
    await open(bed, "None-Ox");
    await expect(bed.page.locator("header .name-block .activity"), "no line, no box").toHaveCount(0);
    const h1 = await bed.page.getByRole("heading", { name: "None-Ox", level: 1 }).boundingBox();
    const block = await bed.page.locator("header .name-block").boundingBox();
    expect(block!.height, "the block is as tall as the name alone").toBeLessThanOrEqual(h1!.height + 1);
    await bed.step("none", { compare: false });
  });
});
