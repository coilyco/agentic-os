// COI-3253: a phone pins the seat's last reply under the message box. A tap reads it all.
import { expect, kindOf, SEATS, test } from "../harness.ts";

const FIRST = "I put an access sysadmin on it and the gate is green now.";
const WHOLE = `${FIRST}\n\n${Array.from({ length: 30 }, (_, i) => `Step ${i + 1}: the thing that changed and why it matters.`).join("\n")}\nEND-OF-THE-REPLY`;

const phoneOnly = (info: import("@playwright/test").TestInfo) => kindOf(info) === "phone";

test("the last reply sits under the message box and a tap reads all of it", async ({ bed }, info) => {
  test.skip(!phoneOnly(info), "a phone pins it, a desktop does not");
  await bed.openSeat(SEATS.imp);
  bed.mock.setLastReply(SEATS.imp.id, { text: WHOLE, ageSeconds: 200 });
  const pin = bed.page.getByRole("button", { name: /Last reply from Imp-Dragonfly, 3m ago/ });
  await expect(pin, "pinned once the seat has a reply").toBeVisible();
  await expect(pin).toContainText(FIRST);

  const box = bed.page.getByRole("textbox", { name: /message/i }).first();
  const [pinBox, textBox] = [await pin.boundingBox(), await box.boundingBox()];
  expect(pinBox!.y, "directly below the message box").toBeGreaterThanOrEqual(textBox!.y + textBox!.height - 2);
  expect(pinBox!.height, "a 44px target or more").toBeGreaterThanOrEqual(44);
  await bed.step("pinned", { compare: false });

  await pin.click();
  const sheet = bed.page.getByRole("dialog", { name: /Last reply from Imp-Dragonfly/ });
  await expect(sheet).toBeVisible();
  const text = sheet.getByRole("region", { name: /Full text of the last reply/ });
  await expect(text, "the whole reply, fetched").toContainText("Step 30:");
  expect(await text.evaluate((el) => el.parentElement!.scrollHeight > el.parentElement!.clientHeight), "a long reply scrolls inside the sheet").toBe(true);
  const sheetBox = (await sheet.boundingBox())!;
  const viewport = bed.page.viewportSize()!;
  expect(sheetBox.y + sheetBox.height, "stays on screen").toBeLessThanOrEqual(viewport.height + 1);
  await bed.step("open", { compare: false });

  await bed.page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
  await expect(pin, "back on the pin").toBeFocused();
});

test("the pin gives way to the keyboard, so the terminal keeps its last lines", async ({ bed }, info) => {
  test.skip(!phoneOnly(info), "a phone pins it");
  await bed.openSeat(SEATS.imp);
  bed.mock.setLastReply(SEATS.imp.id, { text: WHOLE, ageSeconds: 30 });
  const pin = bed.page.getByRole("button", { name: /Last reply from Imp-Dragonfly/ });
  await expect(pin).toBeVisible();
  await bed.page.getByRole("textbox", { name: /message/i }).first().focus();
  await expect(pin, "hidden while the message box has focus").toBeHidden();
  await bed.page.getByRole("textbox", { name: /message/i }).first().blur();
  await expect(pin, "back when it does not").toBeVisible();
});

test("a cut reply says so, and a seat with no reply pins nothing", async ({ bed }, info) => {
  test.skip(!phoneOnly(info), "a phone pins it");
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.getByRole("button", { name: /Last reply from/ }), "no reply yet, no pin").toHaveCount(0);
  bed.mock.setLastReply(SEATS.imp.id, { text: `${FIRST}\nMore.`, ageSeconds: 5, truncated: true });
  await bed.page.getByRole("button", { name: /Last reply from/ }).click();
  await expect(bed.page.getByText("The host sent only the first 8 KiB of this reply.")).toBeVisible();
});

test("a desktop does not pin the reply", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "desktop", "a desktop pane");
  await bed.openSeat(SEATS.imp);
  bed.mock.setLastReply(SEATS.imp.id, { text: WHOLE, ageSeconds: 30 });
  await bed.page.waitForTimeout(300);
  await expect(bed.page.getByRole("button", { name: /Last reply from/ })).toHaveCount(0);
});
