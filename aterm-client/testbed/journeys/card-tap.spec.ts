// COI-3334: on a phone one tap on a choice-card option selects it.
// Kai needed two on her Pixel. These pass on emulated phones, so they guard the tap
// and do not reproduce her report.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { ACCENT, ClaudeAsk } from "../scenes.ts";

const ask = {
  type: "ask",
  ask: {
    id: "tap-1",
    session: SEATS.imp.id,
    header: "Scope",
    question: "Should the seat list keep its alerts pinned, or put them behind one button?",
    options: [
      { label: "Pinned", description: "Always visible, takes space." },
      { label: "Behind a button", description: "One tap away." },
    ],
    allow_other: true,
    multi: false,
  },
};

test("one tap on a daemon card's option answers it", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).tap();
  await expect.poll(() => bed.mock.answers, "the first tap did not answer").toEqual([{ ask_id: "tap-1", picks: [1] }]);
  await expect(card(bed.page)).toBeHidden();
});

test("one tap on a native question's option answers it", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk([ACCENT]);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await expect(card(bed.page)).toBeVisible();
  await card(bed.page).getByRole("button", { name: /Teal/ }).tap();
  await expect.poll(() => dialog.phase, "the first tap did not answer").toBe("done");
  expect(dialog.answers).toEqual(["Teal"]);
});

test("one click on a desktop card's option still answers it", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "pointer");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).click();
  await expect.poll(() => bed.mock.answers).toEqual([{ ask_id: "tap-1", picks: [1] }]);
});

// A card often arrives while the person types: composer focused, keyboard up.
test("one tap answers a card while the keyboard is up", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "an on-screen keyboard is a phone thing");
  await bed.openSeat(SEATS.imp);
  const size = bed.page.viewportSize()!;
  await bed.page.locator("#composer-input").focus();
  await bed.page.setViewportSize({ width: size.width, height: size.height - Math.round(size.height * 0.4) });
  await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await bed.step("keyboard-up-card", { compare: false });
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).tap();
  await expect.poll(() => bed.mock.answers, "the first tap did not answer").toEqual([{ ask_id: "tap-1", picks: [1] }]);
});
