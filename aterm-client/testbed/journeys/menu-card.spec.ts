// COI-3788: the daemon names the resume picker, and every screen size gets a native card.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { recorded } from "../scenes.ts";

test("the resume picker is a card that moves, resumes and closes with its own keys", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, recorded("resume-picker"));
  bed.mock.patchSession(SEATS.imp.id, { tui: "resume_picker" });
  const menu = card(bed.page);
  await expect(menu.getByRole("heading", { name: "Resume a conversation" })).toBeVisible();
  // A 100-column capture does not reflow, so a phone wraps the highlight off the top.
  const whole = kindOf(info) === "desktop";
  if (whole) await expect(menu.getByText("eng-platform-wise-beetle")).toBeVisible();
  else await expect(menu.getByRole("button", { name: "Resume", exact: true })).toBeDisabled();
  // The DOM checks are the test. CI has no Linux baseline to compare against.
  await bed.step("resume-picker", { compare: false });
  const keys = [["Highlight the row below", "\x1b[B"], ["Highlight the row above", "\x1b[A"], ...(whole ? ([["Resume", "\r"]] as const) : []), ["Close", "\x1b"]] as const;
  for (const [button, key] of keys) {
    await menu.getByRole("button", { name: button, exact: true }).click();
    await expect.poll(() => bed.mock.typed(SEATS.imp.id).at(-1), button).toBe(key);
  }
});

test("a seat the daemon reads as idle draws no card, even with a menu footer on screen", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "desktop", "the capture is fixed width");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, recorded("q1"));
  bed.mock.patchSession(SEATS.imp.id, { tui: "idle" });
  await expect(card(bed.page)).toBeHidden();
  bed.mock.patchSession(SEATS.imp.id, { tui: "choice" });
  await expect(card(bed.page)).toBeVisible();
});
