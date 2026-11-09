// COI-3514: on a touch device the keyboard's return key makes a newline and Send sends.
// A desktop keeps Enter to send and Shift+Enter for a newline.
import { expect, kindOf, quirk, SEATS, test } from "../harness.ts";

type Bed = Parameters<Parameters<typeof test>[2]>[0]["bed"];
const sent = (bed: Bed) => bed.mock.typed(SEATS.imp.id).join("");

test("on a phone Enter makes a newline and Send sends the lines", quirk("COI-3514"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await expect(field, "the return key still shows a send arrow").toHaveAttribute("enterkeyhint", "enter");
  await field.focus();
  await bed.page.keyboard.type("line one");
  const one = (await field.boundingBox())!.height;
  await bed.page.keyboard.press("Enter");
  await bed.page.keyboard.type("line two");
  await bed.page.keyboard.press("Enter");
  await bed.page.keyboard.type("line three");
  expect(sent(bed), "Enter sent a half-written message").toBe("");
  await expect(field).toHaveValue("line one\nline two\nline three");
  expect((await field.boundingBox())!.height, "the box did not grow with its lines").toBeGreaterThan(one);
  await bed.page.locator("form.composer button.primary").tap();
  await expect.poll(() => sent(bed)).toContain("line one");
  expect(sent(bed)).toContain("line three");
  await expect(field).toHaveValue("");
});

test("on a phone dictation lands whole and Send sends it", quirk("COI-3514"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await field.focus();
  for (const chunk of ["tell the seat ", "to run the gate ", "and report"]) await bed.page.keyboard.insertText(chunk);
  await expect(field).toHaveValue("tell the seat to run the gate and report");
  await bed.page.locator("form.composer button.primary").tap();
  await expect.poll(() => sent(bed)).toContain("tell the seat to run the gate and report");
});

test("on a desktop Enter sends and Shift+Enter makes a newline", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "pointer");
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await expect(field).toHaveAttribute("enterkeyhint", "send");
  await field.focus();
  await bed.page.keyboard.type("first");
  await bed.page.keyboard.press("Shift+Enter");
  await bed.page.keyboard.type("second");
  await expect(field).toHaveValue("first\nsecond");
  expect(sent(bed)).toBe("");
  await bed.page.keyboard.press("Enter");
  await expect.poll(() => sent(bed)).toContain("first");
  expect(sent(bed)).toContain("second");
});
