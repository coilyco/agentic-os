// COI-3800: the person's own messages render as markdown, and an image they sent shows
// inline. The sending device draws its own copy at once, another reads the host's.
import { crc32, deflateSync } from "node:zlib";
import { expect, kindOf, pickView, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true, images: true, thumbs: true } });

const imp = SEATS.imp.id;
const reply = { id: "r0", kind: "reply", text: "Ready.", complete: true, at: "2026-11-27T21:00:00Z" };

/** A solid PNG the size asked, so a thumbnail has a real picture to draw. */
function png(width: number, height: number): Buffer {
  const chunk = (type: string, data: Buffer) => {
    const body = Buffer.concat([Buffer.from(type), data]);
    const out = Buffer.alloc(body.length + 8);
    out.writeUInt32BE(data.length, 0);
    body.copy(out, 4);
    out.writeUInt32BE(crc32(body), body.length + 4);
    return out;
  };
  const head = Buffer.alloc(13);
  head.writeUInt32BE(width, 0);
  head.writeUInt32BE(height, 4);
  head.set([8, 2, 0, 0, 0], 8);
  const row = Buffer.concat([Buffer.from([0]), Buffer.alloc(width * 3, 0xc0)]);
  const pixels = deflateSync(Buffer.concat(Array.from({ length: height }, () => row)));
  return Buffer.concat([Buffer.from("\x89PNG\r\n\x1a\n", "binary"), chunk("IHDR", head), chunk("IDAT", pixels), chunk("IEND", Buffer.alloc(0))]);
}

async function openFocus(bed: Bed, items: unknown[]): Promise<void> {
  bed.mock.setTranscript(imp, [reply, ...items]);
  await bed.openSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await pickView(bed.page, "Focus");
  await expect(bed.page.locator(".transcript")).toContainText("Ready.");
}

const noSideScroll = (bed: Bed) => bed.page.locator(".transcript .feed").evaluate((feed) => feed.scrollWidth <= feed.clientWidth + 1);

test("a sent message with code, a list and a link renders formatted", async ({ bed }) => {
  const words = "Check `aterm --list` first:\n\n- the **stripe**\n- the [runbook](https://example.com/runbook)";
  await openFocus(bed, [{ id: "p1", kind: "prompt", text: words, at: "2026-11-27T21:01:00Z" }]);
  const mine = bed.page.locator('.transcript .turn.me[data-row="p1"]');
  await expect(mine.locator("code")).toHaveText("aterm --list");
  await expect(mine.locator("ul > li")).toHaveCount(2);
  await expect(mine.locator("strong")).toHaveText("stripe");
  await expect(mine.getByRole("link", { name: "runbook" })).toBeVisible();
  await expect(mine, "the raw markdown is gone").not.toContainText("`");
  expect(await noSideScroll(bed), "the message scrolls sideways").toBe(true);
  await bed.step("markdown", { compare: false });
});

test("an image sent from this device shows at once, pending and settled, without asking the host", async ({ bed }) => {
  await openFocus(bed, []);
  const page = bed.page;
  await page.locator('form.composer input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: png(600, 300) });
  await page.locator("#composer-input").fill("this `icon` is wrong");
  await page.locator("form.composer button.primary").click();

  const pending = page.locator('[data-row^="sending-"]');
  await expect(pending.getByRole("img", { name: "Image you sent" })).toBeVisible();
  await expect(pending.locator("code")).toHaveText("icon");
  await bed.step("pending-image", { compare: false });

  const path = `/tmp/aterm/images/${imp}-1/upload-1.png`;
  bed.mock.appendTranscript(imp, [{ id: "p2", kind: "prompt", text: `[Image #1][Attached image: ${path}] this \`icon\` is wrong`, at: new Date().toISOString() }]);
  const settled = page.locator('.transcript .turn.me[data-row="p2"]');
  await expect(settled.getByRole("img", { name: "Image you sent" })).toBeVisible({ timeout: 6000 });
  await expect(pending).toHaveCount(0);
  await expect(settled, "the markers are not words").not.toContainText("Attached image");
  expect(bed.mock.thumbReads, "this device drew its own copy").toHaveLength(0);
  expect(await noSideScroll(bed)).toBe(true);
});

test("another device reads the thumbnail from the host, and a tap opens it full size", async ({ bed }) => {
  const path = `/tmp/aterm/images/${imp}-1/upload-9.png`;
  bed.mock.hostImage(path, png(900, 600));
  await openFocus(bed, [{ id: "p3", kind: "prompt", text: `[Image #1][Attached image: ${path}] from the phone`, at: "2026-11-27T21:02:00Z" }]);
  const mine = bed.page.locator('.transcript .turn.me[data-row="p3"]');
  const thumb = mine.getByRole("img", { name: "Image you sent" });
  await expect(thumb).toBeVisible();
  expect(await thumb.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
  expect(bed.mock.thumbReads).toEqual([{ session: imp, path, full: false }]);
  await bed.step("host-thumb", { compare: false });

  // By keyboard: a tap focuses no button in WebKit, and focus return is for the keyboard.
  await mine.getByRole("button", { name: "Open image full size" }).focus();
  await bed.page.keyboard.press("Enter");
  const viewer = bed.page.getByRole("dialog", { name: "Image you sent" });
  await expect(viewer.getByRole("img")).toBeVisible();
  expect(bed.mock.thumbReads.at(-1)).toEqual({ session: imp, path, full: true });
  await bed.step("full-size", { compare: false });
  await bed.page.keyboard.press("Escape");
  await expect(viewer).toHaveCount(0);
  await expect(mine.getByRole("button", { name: "Open image full size" }), "focus goes back to the picture").toBeFocused();
  await mine.getByRole("button", { name: "Open image full size" }).click();
  await expect(viewer.getByRole("img"), "a tap opens it too").toBeVisible();
});

test("an image the host no longer holds says so in words", async ({ bed }) => {
  await openFocus(bed, [{ id: "p4", kind: "prompt", text: `[Attached image: /tmp/aterm/images/${imp}-1/upload-77.png] gone now`, at: "2026-11-27T21:03:00Z" }]);
  const mine = bed.page.locator('.transcript .turn.me[data-row="p4"]');
  await expect(mine.getByRole("note")).toContainText("no longer holds this image");
  await expect(mine).toContainText("gone now");
  await bed.step("image-gone", { compare: false });
});
