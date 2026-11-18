// Journey: a first message is typed while the seat waits on Enter (COI-3680).
// The screen is a 2.1.327 capture from a made-up API key's run, laid out again per width.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { reflowed } from "../scenes.ts";

test("a first message waits on the Enter-only screen, and Continue answers it", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, reflowed("claude-2.1.327-100-security-notes"));
  await expect(card(bed.page).getByRole("heading", { name: /Security notes/ })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: "Continue" })).toBeVisible();
  const composer = bed.page.locator("#composer-input");
  await composer.fill("hello once I am signed in");
  const phone = kindOf(test.info()) === "phone";
  // On touch Enter is a newline (COI-3514), and a disabled Send is the hold.
  if (phone) await expect(bed.page.locator("form.composer button.primary")).toHaveText("Waits for answer");
  else await composer.press("Enter");
  await expect(bed.page.locator("p.hold", { hasText: "waiting on Enter" })).toBeVisible();
  await bed.step("message-waiting", { mask: [bed.screen()], compare: false });
  await bed.page.waitForTimeout(600);
  expect(bed.mock.typed(SEATS.imp.id).join(""), "the message went in on the screen that waits on Enter").not.toContain("hello once I am signed in");
  await card(bed.page).getByRole("button", { name: "Continue" }).click();
  await expect.poll(() => bed.mock.typed(SEATS.imp.id).join("")).toBe("\r");
  await expect(composer, "the words stay in the box for the prompt").toHaveValue("hello once I am signed in");
});
