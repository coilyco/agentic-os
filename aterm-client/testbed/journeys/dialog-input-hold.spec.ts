// Journey: a native dialog is open and a message is sent from the composer. The daemon
// half of COI-3243 bug 1 is not testable on a mock. See the testbed reference.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { ACCENT, ClaudeAsk } from "../scenes.ts";

test("a composer message waits for the open dialog", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk([ACCENT]);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await expect(card(bed.page)).toBeVisible();
  const composer = bed.page.locator("#composer-input");
  await composer.fill("hello while you ask");
  const phone = kindOf(test.info()) === "phone";
  // On touch Enter is a newline (COI-3514), and a disabled Send is the hold.
  if (phone) await expect(bed.page.locator("form.composer button.primary")).toHaveText("Waits for answer");
  else await composer.press("Enter");
  // The card covers the terminal, whose redraw differs run to run on WebKit.
  await bed.step("sent-while-open", { mask: [bed.screen()], compare: !phone });
  await bed.page.waitForTimeout(600);
  expect(bed.mock.typed(SEATS.imp.id).join(""), "the message went into the open dialog").not.toContain("hello while you ask");
  await card(bed.page).getByRole("button", { name: /Teal/ }).click();
  await expect.poll(() => dialog.answers).toEqual(["Teal"]);
});
