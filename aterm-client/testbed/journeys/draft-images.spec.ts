// COI-3582: images attached to an unsent message are chips again after a reopen.
import { expect, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { images: true } });

// A 1x1 PNG, enough for the picker to make a chip from.
const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

// A reload rests on the seat overview, so the seat is opened again by its tab (COI-3518).
const reopen = async (bed: Bed) => {
  await bed.page.reload();
  await bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
};

const chips = (bed: Bed) => bed.page.locator("form.composer ul.chips li");
const attach = (bed: Bed, name: string) => bed.page.locator('form.composer input[type="file"]').setInputFiles({ name, mimeType: "image/png", buffer: PIXEL });

// The IndexedDB write is async, so a journey waits for it before it reloads.
const stored = (bed: Bed) =>
  bed.page.evaluate(
    () =>
      new Promise<number>((resolve) => {
        const open = indexedDB.open("aterm.draft-images.v1", 1);
        open.onupgradeneeded = () => open.result.createObjectStore("images", { keyPath: "key" });
        open.onsuccess = () => {
          const all = open.result.transaction("images").objectStore("images").getAll();
          all.onsuccess = () => resolve((all.result as { files: unknown[] }[]).reduce((sum, row) => sum + row.files.length, 0));
        };
      }),
  );

test("an attached image is a chip again after the page is reopened", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await attach(bed, "shot-one.png");
  await expect(chips(bed)).toHaveCount(1);
  await expect.poll(() => stored(bed)).toBe(1);
  await reopen(bed);
  await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
  await expect(chips(bed), "the chip is back with no tap").toHaveCount(1);
  await expect(bed.page.getByRole("status").filter({ hasText: "from your draft came back" })).toBeVisible();
  await bed.step("images-back", { compare: false });
});

test("a removed or sent image does not come back", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await attach(bed, "shot-one.png");
  await attach(bed, "shot-two.png");
  await expect(chips(bed)).toHaveCount(2);
  await expect.poll(() => stored(bed)).toBe(2);
  await reopen(bed);
  await expect(chips(bed)).toHaveCount(2);
  await chips(bed).first().getByRole("button").click();
  await expect.poll(() => stored(bed)).toBe(1);
  await reopen(bed);
  await expect(chips(bed), "the removed image stays removed").toHaveCount(1);
  await bed.page.locator("#composer-input").fill("with the picture");
  await bed.page.locator("form.composer button.primary").click();
  await expect(chips(bed)).toHaveCount(0);
  await expect.poll(() => stored(bed)).toBe(0);
  await reopen(bed);
  await expect(bed.page.locator("#composer-input")).toBeVisible();
  await expect(chips(bed), "a sent image is not a draft").toHaveCount(0);
});

test("an image belongs to its seat", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await attach(bed, "for-imp.png");
  await expect(chips(bed)).toHaveCount(1);
  await bed.switchSeat(SEATS.beetle);
  await expect(chips(bed), "another seat starts with none").toHaveCount(0);
  await bed.switchSeat(SEATS.imp);
  await expect(chips(bed), "and the first seat's image is waiting").toHaveCount(1);
});

test("when the device keeps no storage the box says the images are not kept", async ({ bed }) => {
  await bed.page.addInitScript(() => Object.defineProperty(window, "indexedDB", { value: undefined }));
  await bed.openSeat(SEATS.imp);
  await attach(bed, "shot-one.png");
  await expect(chips(bed)).toHaveCount(1);
  await expect(bed.page.getByText("These images will not be kept if the page closes")).toBeVisible();
});
