// Journey: write to a seat from a phone with its keyboard up. The keyboard is emulated by
// shrinking the viewport, so a real iOS keyboard's quirks are the Simulator follow-up.
import { expect, kindOf, SEATS, test } from "../harness.ts";

test("type to a seat with the keyboard up", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "phone", "an on-screen keyboard is a phone thing");
  await bed.openSeat(SEATS.imp);
  const size = bed.page.viewportSize()!;
  const keyboard = Math.round(size.height * 0.4);
  const composer = bed.page.locator("#composer-input");
  await composer.focus();
  const before = bed.mock.boxOf(SEATS.imp.id)!.rows;
  await bed.page.setViewportSize({ width: size.width, height: size.height - keyboard });
  // The terminal refits to the smaller viewport and the mock redraws. Wait for that.
  await expect.poll(() => bed.mock.boxOf(SEATS.imp.id)!.rows).not.toBe(before);
  // A late font or a slow frame can resize once more, so the shot waits for a long quiet.
  await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
  // With focus in the composer the tab strip goes away, so the seat has room.
  await composer.focus();
  await expect(bed.page.getByRole("tab", { name: "Messages" }), "the tab strip stays under the keyboard").toBeHidden();
  await bed.step("keyboard-up");
  await composer.fill("run the gate again and tell me what changed");
  await bed.step("typed");
  const box = (await composer.boundingBox())!;
  expect(box.y + box.height, "the composer sits below the keyboard's edge").toBeLessThanOrEqual(size.height - keyboard);
  const terminal = (await bed.screen().boundingBox())!;
  expect(terminal.height, "pixels of the seat's text left above the composer").toBeGreaterThanOrEqual(120);
  await composer.press("Enter");
  await expect.poll(() => bed.mock.typed(SEATS.imp.id).join("")).toContain("run the gate again");
});
