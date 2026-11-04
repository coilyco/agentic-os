// COI-3238 item 2: the terminal is too narrow because the side panel stays open.
// Desktop only. A phone's panel is a bottom sheet that starts put away.
import { expect, kindOf, quirk, SEATS, test } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "desktop split layout"));

test("the seat's terminal is wide enough for an 80 column program", quirk("COI-3238 item 2"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await expect.poll(() => bed.cols(SEATS.imp), "columns the seat is sized to, with the panel as it opens").toBeGreaterThanOrEqual(100);
  await bed.step("as-opened");
});

test("the side panel opens put away", quirk("COI-3238 item 2"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.getByRole("tabpanel", { name: /Messages|Views|Browser|Terminal/ }), "the panel is open before anyone asked").toBeHidden();
});

test("one click opens and closes it, and the choice is kept", quirk("COI-3238 item 2"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const toggle = bed.page.getByRole("button", { name: /side panel/i });
  await expect(toggle.first(), "no control to show or hide the panel").toBeVisible();
  await toggle.first().click();
  await bed.step("toggled");
  await bed.page.reload();
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();
  await bed.page.getByRole("tab", { name: /Imp-Dragonfly/ }).first().click();
  await bed.step("after-reload");
});
