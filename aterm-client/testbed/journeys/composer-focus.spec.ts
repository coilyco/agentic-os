// COI-3500: with the keyboard up, a tap on Send, Add image or an image's remove button
// must not take focus from the field, or the keyboard closes and the first tap is lost.
import { expect, kindOf, quirk, SEATS, test } from "../harness.ts";

test.use({ mockOptions: { images: true } });

const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

type Bed = Parameters<Parameters<typeof test>[2]>[0]["bed"];

async function typing(bed: Bed): Promise<void> {
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await field.focus();
  await field.fill("run the gate again");
  const size = bed.page.viewportSize()!;
  await bed.page.setViewportSize({ width: size.width, height: size.height - Math.round(size.height * 0.4) });
  await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
  await expect(field).toBeFocused();
}

const focused = (bed: Bed) => bed.page.evaluate(() => document.activeElement?.id ?? document.activeElement?.tagName ?? "none");

test("one tap on Send sends and the field keeps focus", quirk("COI-3500"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "an on-screen keyboard is a phone thing");
  await typing(bed);
  await bed.page.locator("form.composer button.primary").tap();
  await expect.poll(() => bed.mock.typed(SEATS.imp.id).join("")).toContain("run the gate again");
  expect(await focused(bed), "Send took focus, so the keyboard closes").toBe("composer-input");
});

test("one tap on Add image leaves focus in the field", quirk("COI-3500"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "an on-screen keyboard is a phone thing");
  await typing(bed);
  const chooser = bed.page.waitForEvent("filechooser");
  await bed.page.getByRole("button", { name: "Add image" }).tap();
  await chooser;
  expect(await focused(bed), "Add image took focus, so the keyboard closes").toBe("composer-input");
});

test("one tap on an image's remove button leaves focus in the field", quirk("COI-3500"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "an on-screen keyboard is a phone thing");
  await typing(bed);
  const chooser = bed.page.waitForEvent("filechooser");
  await bed.page.getByRole("button", { name: "Add image" }).tap();
  await (await chooser).setFiles({ name: "a.png", mimeType: "image/png", buffer: PIXEL });
  const remove = bed.page.getByRole("button", { name: /Remove image 1/ });
  await expect(remove).toBeVisible();
  await bed.page.locator("#composer-input").focus();
  await remove.tap();
  await expect(remove).toHaveCount(0);
  expect(await focused(bed), "Remove took focus, so the keyboard closes").toBe("composer-input");
});
