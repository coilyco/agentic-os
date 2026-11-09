// Journey: replay each real Claude Code 2.1.318 capture and ask if it becomes a card.
// One test per screen. Desktop only: a capture is fixed width and does not reflow.
import { card, expect, kindOf, quirk, SEATS, test } from "../harness.ts";
import { captured } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "captures are fixed width"));

const show = async (bed: Parameters<Parameters<typeof test>[2]>[0]["bed"], file: string) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, captured(`claude-2.1.318-${file}`));
};

test("capture: first question of three", async ({ bed }) => {
  await show(bed, "72-01-q1");
  await expect(card(bed.page).getByRole("heading", { name: /accent color/ })).toBeVisible();
  await bed.step("card");
});

test("capture: single question", async ({ bed }) => {
  await show(bed, "72-20-single-q");
  await expect(card(bed.page).getByRole("heading", { name: /accent color/ })).toBeVisible();
});

test("capture: multi-select with its Next row", quirk("COI-3243 bug 2"), async ({ bed }) => {
  await show(bed, "72-03-after-enter-q2");
  await expect(card(bed.page).getByRole("button", { name: /^Next$/ }), "no Next button on the card").toBeVisible();
  await expect(card(bed.page).getByText(/Pick any/), "the card does not know it is a multi-select").toBeVisible();
  await bed.step("card");
});

test("capture: multi-select with the cursor on Next", quirk("COI-3243 bug 3"), async ({ bed }) => {
  await show(bed, "72-05-q2-cursor-on-next");
  await expect(card(bed.page), "the card drops when the cursor reaches Next").toBeVisible();
  await bed.step("card");
});

test("capture: the review screen, which has no footer", quirk("COI-3243 bug 3"), async ({ bed }) => {
  await show(bed, "72-09-after-q3-enter");
  await expect(card(bed.page).getByRole("button", { name: /Submit answers/ }), "the review screen shows no card").toBeVisible();
  await bed.step("card");
});

test("capture: last multi-select at 134 columns reads Submit", quirk("COI-3243 bug 2"), async ({ bed }, info) => {
  // Only the 1920 window gives the terminal the 134 columns this capture was taken at.
  test.skip(info.project.name !== "desktop-1920", "needs a 134 column terminal");
  await show(bed, "134-01-q2");
  await expect(card(bed.page).locator(".actions").getByRole("button", { name: /^Submit$/ }), "no Submit button on the card").toBeVisible();
  await bed.step("card");
});
