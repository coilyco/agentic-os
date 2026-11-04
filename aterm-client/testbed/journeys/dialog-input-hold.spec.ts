// Journey: a native dialog is open and a message is sent from the composer. The daemon
// half of COI-3243 bug 1 is not testable on a mock. See the testbed reference.
import { card, expect, quirk, SEATS, test } from "../harness.ts";
import { ACCENT, ClaudeAsk } from "../scenes.ts";

test("a composer message waits for the open dialog", quirk("COI-3243 bug 1 (client half)"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk([ACCENT]);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await expect(card(bed.page)).toBeVisible();
  const composer = bed.page.locator("#composer-input");
  await composer.fill("hello while you ask");
  await composer.press("Enter");
  await bed.step("sent-while-open");
  await bed.page.waitForTimeout(600);
  expect(bed.mock.typed(SEATS.imp.id).join(""), "the message went into the open dialog").not.toContain("hello while you ask");
  await card(bed.page).getByRole("button", { name: /Teal/ }).click();
  await expect.poll(() => dialog.answers).toEqual(["Teal"]);
});
