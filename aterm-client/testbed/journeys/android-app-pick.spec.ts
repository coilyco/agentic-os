// COI-3739: the app's picker has no type filter, so Android opens its file browser and
// not the Photo Picker. The app is faked at `window.__TAURI__`. Lane: image-pick-app.
import { expect, SEATS, test } from "../harness.ts";

test.use({ mockOptions: { images: true } });

const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
const PICKER = 'form.composer input[type="file"]';

async function inApp(page: import("@playwright/test").Page): Promise<void> {
  await page.addInitScript(() => {
    (window as unknown as { __TAURI__: object }).__TAURI__ = {
      core: {
        invoke: async (command: string) => {
          if (command === "plugin:sharetarget|take") return { images: [], skipped: [], text: null };
          throw new Error(`no ${command}`);
        },
      },
    };
  });
}

test.describe("the Android app", () => {
  test.beforeEach(({}, info) => {
    test.skip(!/Android/.test(String(info.project.use.userAgent)), "the app's WebView says Android");
  });

  test("Add image opens a picker with no type filter, and a picked PNG reaches the seat", async ({ bed }) => {
    await inApp(bed.page);
    await bed.openSeat(SEATS.imp);
    const page = bed.page;
    expect(await page.locator(PICKER).getAttribute("accept"), "no filter, so Android opens its file browser and not the Photo Picker").toBeNull();

    await page.locator(PICKER).setInputFiles({ name: "Screenshot_1.png", mimeType: "image/png", buffer: PIXEL });
    await expect(page.getByRole("button", { name: "Remove image 1" })).toBeVisible();
    await page.locator("form.composer button.primary").click();
    await expect.poll(() => bed.mock.uploads.length).toBe(1);
    expect(bed.mock.uploads[0]!.bytes.equals(PIXEL)).toBe(true);
  });

  test("a file that is not an image is refused in words, and nothing is uploaded", async ({ bed }) => {
    await inApp(bed.page);
    await bed.openSeat(SEATS.imp);
    const page = bed.page;
    await page.locator(PICKER).setInputFiles({ name: "notes.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4") });
    await expect(page.locator("form.composer")).toContainText("notes.pdf is not an image the seat can read. PNG, JPEG, GIF and WebP work.");
    await expect(page.getByRole("button", { name: "Remove image 1" })).toHaveCount(0);
    expect(bed.mock.uploads).toHaveLength(0);
  });
});

test("a browser keeps the image filter on Add image", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  expect(await bed.page.locator(PICKER).getAttribute("accept")).toBe("image/png,image/jpeg,image/gif,image/webp");
});
