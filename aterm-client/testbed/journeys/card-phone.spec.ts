// COI-3320 items 1 and 8: on a phone the card leaves the output readable and can fold.
import { card, expect, kindOf, quirk, SEATS, test } from "../harness.ts";

const long = "A description long enough to wrap to a second line on a phone, as Claude Code's often do.";
const ask = {
  type: "ask",
  ask: {
    id: "phone-1",
    session: SEATS.imp.id,
    header: "Accent",
    question: "Which accent color should the client use for the active seat?",
    options: ["Purple", "Teal", "Black", "Gold"].map((label) => ({ label, description: long })),
    allow_other: true,
    multi: false,
  },
};
const heightOf = async (locator: ReturnType<typeof card>) => (await locator.boundingBox())!.height;

test("the card leaves the seat's output a readable slice", quirk("COI-3320 item 1"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "phone card");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  const screen = (await bed.page.locator(".screen").first().boundingBox())!;
  expect(await heightOf(card(bed.page)), "the card takes more than 62% of the terminal area").toBeLessThanOrEqual(screen.height * 0.62);
  await bed.step("capped");
});

test("Hide question folds the card to a bar, and Show question brings it back", quirk("COI-3320 item 1"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "phone card");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await card(bed.page).getByRole("button", { name: "Hide question" }).click();
  await expect(card(bed.page).getByRole("button", { name: /Purple/ })).toBeHidden();
  expect(await heightOf(card(bed.page)), "a folded card is not a bar").toBeLessThanOrEqual(110);
  await expect(card(bed.page).getByRole("heading", { name: /accent color/ }), "the question is still there as one line").toBeVisible();
  await bed.step("folded");
  await card(bed.page).getByRole("button", { name: "Show question" }).click();
  await expect(card(bed.page).getByRole("button", { name: /Purple/ })).toBeVisible();
});

test("the hint in the text box fits the box", quirk("COI-3320 item 8"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "phone card");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  const field = bed.page.locator("#choice-text");
  await expect(field).toBeFocused();
  const fits = await field.evaluate((el: HTMLInputElement) => {
    const probe = document.createElement("span");
    probe.textContent = el.placeholder;
    probe.style.cssText = `font: ${getComputedStyle(el).font}; position: absolute; visibility: hidden; white-space: nowrap`;
    document.body.append(probe);
    const wanted = probe.getBoundingClientRect().width;
    probe.remove();
    return { wanted: Math.round(wanted), room: Math.round(el.clientWidth - 24) };
  });
  expect(fits.wanted, `the hint needs ${fits.wanted}px and the box has ${fits.room}px`).toBeLessThanOrEqual(fits.room);
  await expect(field, "the box it opened is below the fold").toBeInViewport();
  await bed.step("text-box");
});

test("a desktop card has no fold button", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "desktop card");
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Hide question/ })).toHaveCount(0);
});
