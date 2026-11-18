// Journey: a menu stays on screen after its card was answered, then a message is sent
// (COI-3689). The capture ignores keys, so the menu stays. Desktop only.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { captured } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "captures are fixed width"));

test("a message never types into a menu the card was answered for", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, captured("claude-2.1.318-72-03-after-enter-q2"));
  await card(bed.page).getByRole("button", { name: /Qwen/ }).click();
  await card(bed.page).getByRole("button", { name: /^Next$/ }).click();
  await expect(card(bed.page)).toBeHidden();
  // The card's own keys go in 150ms apart, and are not the message.
  await bed.page.waitForTimeout(1200);
  const before = bed.mock.typed(SEATS.imp.id).join("");
  const composer = bed.page.locator("#composer-input");
  await composer.fill("a message for after the menu");
  await composer.press("Enter");
  await bed.step("held-behind-the-menu", { mask: [bed.screen()], compare: false });
  await bed.page.waitForTimeout(600);
  expect(bed.mock.typed(SEATS.imp.id).join(""), "the message went into the open menu").toBe(before);
  await expect(composer, "the words stay in the box").toHaveValue("a message for after the menu");
  await expect(card(bed.page), "a menu that did not take the keys brings its card back").toBeVisible({ timeout: 6000 });
});
