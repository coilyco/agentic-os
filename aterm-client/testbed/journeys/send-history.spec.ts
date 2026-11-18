// COI-3691: up arrow on the first line recalls past sends, down walks back to the draft,
// a phone's History control does the same, and a send is kept the moment Send is pressed.
import { card, expect, kindOf, reopenSeat, SEATS, test, type Bed } from "../harness.ts";
import { ACCENT, ClaudeAsk } from "../scenes.ts";

test.use({ mockOptions: { images: true } });

const field = (bed: Bed) => bed.page.locator("#composer-input");
const sendButton = (bed: Bed) => bed.page.locator("form.composer button.primary");

async function send(bed: Bed, words: string): Promise<void> {
  await field(bed).fill(words);
  await sendButton(bed).click();
  await expect(field(bed)).toHaveValue("");
}

test("up recalls older sends, down walks back, and past the newest the draft returns", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await send(bed, "first send");
  await send(bed, "second send");
  await field(bed).fill("half typed");
  await field(bed).press("ArrowUp");
  await expect(field(bed)).toHaveValue("second send");
  await field(bed).press("ArrowUp");
  await expect(field(bed)).toHaveValue("first send");
  await field(bed).press("ArrowUp");
  await expect(field(bed), "past the oldest it stays on the oldest").toHaveValue("first send");
  await field(bed).press("ArrowDown");
  await expect(field(bed)).toHaveValue("second send");
  await field(bed).press("ArrowDown");
  await expect(field(bed), "the draft was lost").toHaveValue("half typed");
  await field(bed).press("ArrowDown");
  await expect(field(bed)).toHaveValue("half typed");
});

test("up inside a draft of several lines still moves the caret", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await send(bed, "an earlier send");
  await field(bed).fill("line one\nline two");
  await field(bed).press("ArrowUp");
  await expect(field(bed), "up on the second line recalled a send").toHaveValue("line one\nline two");
  await field(bed).press("ArrowUp");
  await expect(field(bed), "up on the first line did not recall").toHaveValue("an earlier send");
});

test("the sends are still there after the app is reopened", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await send(bed, "survives a restart");
  await reopenSeat(bed.page, SEATS.imp);
  await field(bed).press("ArrowUp");
  await expect(field(bed)).toHaveValue("survives a restart");
});

test("a send held back by a dialog was still recorded when Send was pressed", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "desktop", "on a phone a held Send is disabled, so there is no press");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, new ClaudeAsk([ACCENT]));
  await expect(card(bed.page)).toBeVisible();
  await field(bed).fill("swallowed by the dialog");
  await field(bed).press("Enter");
  await expect(bed.page.locator("p.hold")).toContainText("A question is waiting");
  await field(bed).fill("");
  await field(bed).press("ArrowUp");
  await expect(field(bed)).toHaveValue("swallowed by the dialog");
});

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone has no arrow keys"));

  test("the History control lists this seat's sends, puts one in the field, and gets the draft back", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    const control = bed.page.getByRole("button", { name: "Recent sends" });
    await expect(control, "no sends yet, so no control").toHaveCount(0);
    await send(bed, "first send");
    await send(bed, "second send");
    await expect(control).toBeVisible();
    await field(bed).fill("my draft");
    await control.click();
    const list = bed.page.getByRole("list", { name: "Recent sends" });
    await expect(list.getByRole("button")).toHaveCount(3);
    await expect(list.getByRole("button").first()).toContainText("second send");
    await bed.step("history", { compare: false });
    await list.getByRole("button", { name: /first send/ }).click();
    await expect(field(bed)).toHaveValue("first send");
    await expect(list).toHaveCount(0);

    await control.click();
    await expect(bed.page.getByRole("button", { name: "Back to your draft" })).toBeVisible();
    await bed.page.getByRole("button", { name: "Back to your draft" }).click();
    await expect(field(bed)).toHaveValue("my draft");

    await control.click();
    await bed.page.getByRole("button", { name: "Clear recent sends" }).click();
    await expect(control).toHaveCount(0);
  });

  test("the field is still wide enough to type in beside Add image and History", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    await send(bed, "one send");
    const box = await field(bed).boundingBox();
    expect(box!.width, "the field was squeezed").toBeGreaterThanOrEqual(96);
  });
});
