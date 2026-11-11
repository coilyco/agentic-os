// COI-3485: an image shared from Android's Share menu. The plugin is faked at
// `window.__TAURI__` and answers `take` once, like the real one.
import { expect, SEATS, test } from "../harness.ts";

// A 1x1 PNG, so the host reads it as an image by its bytes.
const PIXEL_B64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";
const PIXEL = Buffer.from(PIXEL_B64, "base64");

/** The app as a share starts it: last host kept, no seat open, `take` answers once. */

async function shared(page: import("@playwright/test").Page, answer: object): Promise<void> {
  await page.addInitScript(
    ({ answer, last }) => {
      localStorage.setItem("aterm.last-open.v1", JSON.stringify(last));
      let spent = false;
      (window as unknown as { __TAURI__: object }).__TAURI__ = {
        core: {
          invoke: async (command: string) => {
            if (command !== "plugin:sharetarget|take") throw new Error(`no ${command}`);
            if (spent) return { images: [], skipped: [], text: null };
            spent = true;
            return answer;
          },
        },
      };
    },
    { answer, last: { host: "local" } },
  );
}

/** The sheet leaves the seat to the person, so the journeys that send pick Imp first. */
async function pickImp(sheet: import("@playwright/test").Locator): Promise<void> {
  await sheet.getByRole("radio", { name: new RegExp(SEATS.imp.name) }).check();
}

test.describe("a host that takes images", () => {
  test.use({ mockOptions: { images: true } });

  test("a share opens the sheet, names what was left out, and sends the picture and words to the seat", async ({ bed }) => {
    await shared(bed.page, {
      images: [{ name: "Screenshot_1.png", mime: "image/png", data: PIXEL_B64 }, { name: "photo.heic", mime: "image/heic", data: "AAAA" }],
      skipped: [{ name: "clip.mp4", reason: "not_image" }],
      text: "this broke after the update",
    });
    await bed.page.goto("/");
    const sheet = bed.page.getByRole("dialog", { name: "Send to a seat" });
    await expect(sheet).toBeVisible();
    // Only the PNG is a picture the seat reads, and each thing left out says so in words.
    await expect(sheet.getByRole("button", { name: "Remove image 1" })).toBeVisible();
    await expect(sheet.getByRole("button", { name: "Remove image 2" })).toHaveCount(0);
    await expect(sheet.getByText("photo.heic is not an image the seat can read")).toBeVisible();
    await expect(sheet.getByText("clip.mp4 is not an image the seat can read")).toBeVisible();
    await expect(sheet.getByLabel("Words to go with it, if any")).toHaveValue("this broke after the update");
    // No seat is on screen, so Send waits on a choice and then names it.
    await pickImp(sheet);
    const send = sheet.getByRole("button", { name: `Send to ${SEATS.imp.name}` });
    await expect(send).toBeEnabled();
    await expect(send, "Send is on screen without scrolling the sheet").toBeInViewport({ ratio: 1 });
    await bed.step("sheet", { compare: false });

    await send.click();
    await expect(sheet).toBeHidden();
    expect(bed.mock.uploads).toHaveLength(1);
    expect(bed.mock.uploads[0]!.bytes.equals(PIXEL), "the host received the picture's own bytes").toBe(true);
    await expect.poll(() => bed.mock.typed(SEATS.imp.id).join("")).toContain("this broke after the update");
    expect(bed.mock.typed(SEATS.imp.id).join("")).toContain("/tmp/aterm/images/");
  });

  test("a refused upload says why, sends nothing, keeps the picture and words, and sends once allowed", async ({ bed }) => {
    await shared(bed.page, { images: [{ name: "Screenshot_1.png", mime: "image/png", data: PIXEL_B64 }], skipped: [], text: "keep these words" });
    await bed.page.goto("/");
    const sheet = bed.page.getByRole("dialog", { name: "Send to a seat" });
    await pickImp(sheet);
    const send = sheet.getByRole("button", { name: `Send to ${SEATS.imp.name}` });
    await expect(send).toBeEnabled();
    bed.mock.refuseUploads("too_large");
    await send.click();
    await expect(sheet.getByRole("alert")).toContainText("Not sent. The host took that as over 3 MB.");
    await expect(sheet.getByRole("alert")).toContainText("Your screenshot and words stay here.");
    await expect(sheet.getByRole("alert"), "the refusal is on screen, not below the fold").toBeInViewport({ ratio: 1 });
    await expect(sheet.getByLabel("Words to go with it, if any")).toHaveValue("keep these words");
    await expect(sheet.getByRole("button", { name: "Remove image 1" })).toBeVisible();
    expect(bed.mock.typed(SEATS.imp.id), "nothing reached the seat").toEqual([]);
    await bed.step("refused", { compare: false });
    bed.mock.refuseUploads(null);
    await expect(send).toBeEnabled();
    await send.click();
    await expect(sheet).toBeHidden();
    expect(bed.mock.uploads).toHaveLength(1);
  });

  test("Discard drops the share and puts nothing into the seat", async ({ bed }) => {
    await shared(bed.page, { images: [{ name: "Screenshot_1.png", mime: "image/png", data: PIXEL_B64 }], skipped: [], text: null });
    await bed.page.goto("/");
    const sheet = bed.page.getByRole("dialog", { name: "Send to a seat" });
    await pickImp(sheet);
    await expect(sheet.getByRole("button", { name: `Send to ${SEATS.imp.name}` })).toBeEnabled();
    await sheet.getByRole("button", { name: "Discard" }).click();
    await expect(sheet).toBeHidden();
    expect(bed.mock.uploads).toHaveLength(0);
  });

  test("with no seat chosen Send says so and stays off, and the keyboard reaches the choice", async ({ bed }) => {
    // No remembered seat, so more than one live seat leaves the choice to the person.
    await bed.page.addInitScript(() => {
      localStorage.setItem("aterm.last-open.v1", JSON.stringify({ host: "local" }));
      let spent = false;
      (window as unknown as { __TAURI__: object }).__TAURI__ = {
        core: {
          invoke: async () => {
            if (spent) return { images: [], skipped: [], text: null };
            spent = true;
            return { images: [{ name: "Screenshot_1.png", mime: "image/png", data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==" }], skipped: [], text: null };
          },
        },
      };
    });
    await bed.page.goto("/");
    const sheet = bed.page.getByRole("dialog", { name: "Send to a seat" });
    await expect(sheet).toBeVisible();
    const send = sheet.getByRole("button", { name: "Pick a seat" });
    await expect(send).toBeDisabled();
    const seat = sheet.getByRole("radio", { name: new RegExp(SEATS.beetle.name) });
    await seat.focus();
    await bed.page.keyboard.press("Space");
    await expect(seat).toBeChecked();
    await expect(sheet.getByRole("button", { name: `Send to ${SEATS.beetle.name}` })).toBeEnabled();
    await bed.step("pick-a-seat", { compare: false });
  });
});

test.describe("a host that cannot take images", () => {
  test("the sheet says so and Send stays off", async ({ bed }) => {
    await shared(bed.page, { images: [{ name: "Screenshot_1.png", mime: "image/png", data: PIXEL_B64 }], skipped: [], text: null });
    await bed.page.goto("/");
    const sheet = bed.page.getByRole("dialog", { name: "Send to a seat" });
    await expect(sheet.getByRole("alert").first()).toContainText("This host cannot take images");
    await expect(sheet.getByRole("button", { name: /^(Send to|Pick a seat)/ })).toBeDisabled();
    await bed.step("host-cannot", { compare: false });
  });
});

test.describe("no share waiting", () => {
  test.use({ mockOptions: { images: true } });

  test("a launch with nothing shared opens no sheet", async ({ bed }) => {
    await bed.page.addInitScript(() => {
      (window as unknown as { __TAURI__: object }).__TAURI__ = { core: { invoke: async () => ({ images: [], skipped: [], text: null }) } };
    });
    await bed.openSeat(SEATS.imp);
    await expect(bed.page.getByRole("dialog", { name: "Send to a seat" })).toHaveCount(0);
  });
});
