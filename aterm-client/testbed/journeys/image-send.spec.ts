// COI-3496: the words can reach a busy seat before its attachment forms.
// So the words name the path, and the seat can open the file either way.
import { expect, SEATS, test } from "../harness.ts";
import { claudeSession } from "../scenes.ts";

test.use({ mockOptions: { images: true } });

const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
const PASTE = /^\x1b\[200~(.*)\x1b\[201~$/s;
const inner = (frame: string) => PASTE.exec(frame)?.[1] ?? null;

for (const working of [false, true]) {
  test(`${working ? "a busy" : "an idle"} seat gets the words with the image's path in them, even if its attachment never forms`, async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.setScene(SEATS.imp.id, claudeSession({ working }));
    const page = bed.page;
    await page.locator('form.composer input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: PIXEL });
    await expect(page.getByRole("button", { name: "Remove image 1" })).toBeVisible();
    await page.locator("#composer-input").fill("found some missing icons");
    await page.locator("form.composer button.primary").click();

    await expect.poll(() => bed.mock.typed(SEATS.imp.id).at(-1)).toBe("\r");
    expect(bed.mock.uploads, "the host took the picture's own bytes").toHaveLength(1);
    expect(bed.mock.uploads[0]!.bytes.equals(PIXEL)).toBe(true);

    const frames = bed.mock.typed(SEATS.imp.id);
    const path = inner(frames[0]!);
    expect(path, "the first paste is the lone path, which is what attaches").toMatch(/^\/tmp\/aterm\/images\/.+\.png$/);
    // A late attachment is as if the lone paste never came. The rest names the file.
    const rest = frames.slice(1).map((frame) => inner(frame) ?? frame);
    expect(rest, "the words are one paste that names the path and then says what the person wrote").toContain(`[Attached image: ${path}] found some missing icons`);
    expect(frames.at(-1)).toBe("\r");
  });
}
