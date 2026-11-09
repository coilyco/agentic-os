// COI-3342: the count names only seats that need Kai. 20 seats, 5 of them finished a turn
// she has not seen, 2 on a choice card and 1 with a page waiting: she is told 3.
import type { Page } from "@playwright/test";
import { expect, kindOf, pushOutput, SEATS, test } from "../harness.ts";

const filler = (n: number) => ({
  name: `eng-platform-seat-${n}`,
  role: "eng-platform",
  identity: `Seat-${n}`,
  seat: "claude",
  ready: true,
  bracketed_paste: true,
  kai_drafting: false,
  pending: 0,
  scene: "claude-idle",
});
const SEATS_20 = [
  { ...filler(0), name: SEATS.imp.id, role: "frontend-eng", identity: SEATS.imp.name },
  ...Array.from({ length: 19 }, (_, n) => filler(n + 1)),
];
const ask = (n: number) => ({
  type: "ask",
  ask: { id: `ask-${n}`, session: `eng-platform-seat-${n}`, header: "Scope", question: "Merge it?", options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: false, multi: false },
});
const page = (waiting: boolean) => ({
  type: "artifact",
  session: "eng-platform-seat-9",
  artifact: { id: "page-9", url: "https://claude.ai/artifact/abc", title: "Plan", waiting, standing: false, html: "" },
});

const badge = (p: Page) => p.evaluate(() => (window as unknown as { badge: number }).badge);

test.describe("the waiting count", () => {
  test.use({ mockOptions: { sessions: SEATS_20 } });

  test("names the 3 that need her among 20, and the title, badge and chip agree", async ({ bed }, info) => {
    const { page: tab, mock } = bed;
    await tab.addInitScript(() => {
      const w = window as unknown as { badge: number };
      w.badge = 0;
      Object.assign(navigator, { setAppBadge: async (n: number) => void (w.badge = n), clearAppBadge: async () => void (w.badge = 0) });
    });
    await bed.openSeat(SEATS.imp);
    // Five seats write, then go quiet: each finished a turn while she looked elsewhere.
    for (const n of [11, 12, 13, 14, 15]) pushOutput(mock, `eng-platform-seat-${n}`, "Finished the task.\r\n");
    if (kindOf(info) === "desktop") await expect(tab.getByText("done, your turn").first()).toBeVisible({ timeout: 8000 });
    else await tab.waitForTimeout(2500);
    await expect(tab, "finished seats are not waiting").toHaveTitle("aterm");
    expect(await badge(tab)).toBe(0);
    await bed.step("none-waiting", { compare: false });

    mock.push(ask(3));
    mock.push(ask(4));
    mock.push(page(true));
    await expect(tab).toHaveTitle(/^\(3\) /);
    await expect.poll(() => badge(tab)).toBe(3);
    if (kindOf(info) === "phone") await expect(tab.locator(".elsewhere")).toHaveText(/3/);
    await bed.step("three-waiting", { compare: false });

    mock.push({ type: "artifact_update", session: "eng-platform-seat-9", artifact_id: "page-9", waiting: false });
    await expect(tab).toHaveTitle(/^\(2\) /);
    await expect.poll(() => badge(tab)).toBe(2);
  });
});
