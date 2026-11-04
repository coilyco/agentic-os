// Journey: a seat calls ask_choice, a card shows, a person answers it. The durable path.
import { card, expect, SEATS, test } from "../harness.ts";

const ask = {
  type: "ask",
  ask: {
    id: "ask-1",
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

test("answer a choice card", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await bed.step("card-shown");
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).click();
  await expect.poll(() => bed.mock.answers).toEqual([{ ask_id: "ask-1", picks: [1] }]);
  await expect(card(bed.page)).toBeHidden();
  await bed.step("answered");
});

test("answer a choice card with typed text", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await bed.step("text-box-open");
  await bed.page.getByLabel("Your own answer").fill("Behind a button, with a count on it");
  await card(bed.page).getByRole("button", { name: "Answer" }).click();
  await expect.poll(() => bed.mock.answers.at(0)?.text).toBe("Behind a button, with a count on it");
  await expect(card(bed.page)).toBeHidden();
});
