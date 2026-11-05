// COI-3259: a dialog taller than the seat's screen. Real Claude Code 2.1.318 at 34 by 20.
// The top scrolls away for good and no scrollback keeps it. See the choices reference.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { captured } from "../scenes.ts";

// Landscape phones are short on rows by design and have their own journeys.
test.beforeEach(({}, info) => test.skip(kindOf(info) === "landscape", "landscape is its own journey"));

const HIGHLIGHT_OFF = "claude-2.1.318-34x20-multi-highlight-off-screen";
const TOP_OFF = "claude-2.1.318-34x20-multi-top-off-screen";

const show = async (bed: Parameters<Parameters<typeof test>[2]>[0]["bed"], file: string) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, captured(file));
};

test("the highlight is above the screen: a card says so instead of leaving Claude's raw screen", async ({ bed }) => {
  await show(bed, HIGHLIGHT_OFF);
  await expect(card(bed.page), "no card, so the person is left with Claude's own screen").toBeVisible();
  await expect(card(bed.page).getByText(/first 2 options are off the top/), "the card does not say options are off the screen").toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Move highlight down/ })).toBeVisible();
  await bed.step("highlight-off-screen");
});

test("moving the highlight down is one arrow typed into the seat", async ({ bed }) => {
  await show(bed, HIGHLIGHT_OFF);
  await card(bed.page).getByRole("button", { name: /Move highlight down/ }).click();
  await expect.poll(() => bed.mock.typed(SEATS.imp.id)).toEqual(["\x1b[B"]);
});

test("the first options are off the screen: the card names how many and offers the rest", async ({ bed }) => {
  await show(bed, TOP_OFF);
  await expect(card(bed.page).getByText(/first 2 options are off the top/), "the card is missing options and does not say so").toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Frontier/ })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Qwen/ }), "an option nobody can read was invented").toHaveCount(0);
  await bed.step("top-off-screen");
});

test("ticking a visible option moves from the highlight and not from a guess", async ({ bed }) => {
  await show(bed, TOP_OFF);
  await expect(card(bed.page).getByRole("button", { name: /Frontier/ })).toHaveAttribute("aria-pressed", "false");
  await card(bed.page).getByRole("button", { name: /Frontier/ }).click();
  await card(bed.page).getByRole("button", { name: /^Submit/ }).click();
  // The highlight is on Frontier: tick it in place, then Submit is two rows down.
  await expect.poll(() => bed.mock.typed(SEATS.imp.id)).toEqual(["\r", "\x1b[B\x1b[B", "\r"]);
});

test("a single-select whose highlight is above the screen answers by the number Claude printed", async ({ bed }) => {
  await show(bed, "claude-2.1.318-34x20-single-highlight-off-screen");
  await expect(card(bed.page).getByText(/first option is/), "no card, or no word that an option is off the screen").toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Something else/ }), "typing needs the highlight on the row").toBeDisabled();
  await card(bed.page).getByRole("button", { name: /Black/ }).click();
  // Black is printed as 3. Claude Code answers on the digit wherever the highlight is.
  await expect.poll(() => bed.mock.typed(SEATS.imp.id)).toEqual(["3"]);
  await bed.step("single-highlight-off-screen");
});
