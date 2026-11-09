// Journey: the attach-image button sits left of the field on Send's row, a 44px target
// centred with both (COI-3411). On a desktop the field is tall, so a bottom-aligned row
// left the buttons in its lower corners.
import type { Locator } from "@playwright/test";
import { expect, SEATS, test } from "../harness.ts";

test.use({ mockOptions: { images: true } });

// A 1x1 PNG, enough for the picker to make a chip from.
const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

const centre = async (item: Locator) => {
  const box = (await item.boundingBox())!;
  return { left: box.x, right: box.x + box.width, mid: box.y + box.height / 2, width: box.width, height: box.height };
};

test("the attach button is a 44px target centred with the field and Send", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const page = bed.page;
  const input = page.locator("#composer-input");
  const attach = page.getByRole("button", { name: "Add image" });
  const send = page.locator("form.composer button.primary");
  await input.fill("a short note");

  const check = async (state: string) => {
    const [a, f, s] = [await centre(attach), await centre(input), await centre(send)];
    expect(a.width, `${state}: attach width`).toBeGreaterThanOrEqual(44);
    expect(a.height, `${state}: attach height`).toBeGreaterThanOrEqual(44);
    expect(a.right, `${state}: attach is left of the field`).toBeLessThanOrEqual(f.left);
    expect(s.left, `${state}: Send is right of the field`).toBeGreaterThanOrEqual(f.right);
    expect(Math.abs(a.mid - f.mid), `${state}: attach centre against the field's`).toBeLessThanOrEqual(1);
    expect(Math.abs(s.mid - f.mid), `${state}: Send centre against the field's`).toBeLessThanOrEqual(1);
  };

  await check("one line");
  await bed.step("one-line", { mask: [bed.screen()] });

  await input.fill("first line\nsecond line\nthird line\nfourth line");
  await check("four lines");

  // A chip is a row of its own above the field, and the buttons keep their places.
  await page.locator('form.composer input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: PIXEL });
  const chip = page.getByRole("button", { name: "Remove image 1" });
  await expect(chip).toBeVisible();
  const chipBox = (await chip.boundingBox())!;
  const fieldBox = (await input.boundingBox())!;
  expect(chipBox.y + chipBox.height, "the chip row sits above the field row").toBeLessThanOrEqual(fieldBox.y);
  await check("with a chip");
  await bed.step("with-chip", { mask: [bed.screen()] });
});
