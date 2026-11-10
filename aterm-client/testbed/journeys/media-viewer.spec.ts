// COI-3458: files a seat shares open in a viewer over the seat, as a sequence, with a
// fallback for what the client cannot show. The host's `media` frames are scripted here.
import { readFileSync } from "node:fs";
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { media: true } });

const imp = SEATS.imp.id;
const file = (name: string) => readFileSync(new URL(`../fixtures/media/${name}`, import.meta.url));
const frame = (index: 1 | 2 | 3) => ({ name: index === 1 ? "frame1.png" : `frame${index}.jpg`, mime: index === 1 ? "image/png" : "image/jpeg", kind: "image" as const, bytes: file(index === 1 ? "frame1.png" : `frame${index}.jpg`) });

const chip = (bed: Bed) => bed.page.getByRole("button", { name: /shared \d+ files?/ });
const viewer = (bed: Bed) => bed.page.getByRole("dialog", { name: "Shared by Imp-Dragonfly" });

test("a seat that shared nothing has no chip", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(chip(bed)).toHaveCount(0);
});

test("a shared image says so, opens in a viewer, and the chip is spent once it has been seen", async ({ bed, browserName }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, frame(1));
  await expect(chip(bed)).toContainText("Imp-Dragonfly shared 1 file, 1 new");
  await chip(bed).click();
  await expect(viewer(bed)).toBeVisible();
  const image = viewer(bed).getByRole("img", { name: "frame1.png" });
  await expect(image).toBeVisible();
  expect(await image.evaluate((img: HTMLImageElement) => img.naturalWidth), "the bytes decoded as the image").toBe(320);
  await expect(viewer(bed)).toContainText("Image, 3 KB");
  await bed.step("image", { compare: false });
  await bed.page.getByRole("button", { name: "Close" }).click();
  await expect(viewer(bed)).toHaveCount(0);
  // Safari does not focus a button on a click, so there is nothing to return to.
  if (browserName === "chromium") await expect(chip(bed), "focus returns to where it was").toBeFocused();
  await expect(chip(bed)).not.toContainText("new");
});

test("several frames read as a sequence, and the view follows the newest until the person steps back", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, frame(1));
  bed.mock.shareMedia(imp, frame(2));
  bed.mock.shareMedia(imp, frame(3));
  await expect(chip(bed)).toContainText("shared 3 files, 3 new");
  await chip(bed).click();
  await expect(viewer(bed)).toContainText("3 of 3");
  await expect(viewer(bed).getByRole("button", { name: "Next file" })).toBeDisabled();
  await viewer(bed).getByRole("button", { name: "Previous file" }).click();
  await expect(viewer(bed)).toContainText("2 of 3");
  await expect(viewer(bed).getByRole("img", { name: "frame2.jpg" })).toBeVisible();
  await bed.step("sequence", { compare: false });
  bed.mock.shareMedia(imp, frame(1));
  await expect(viewer(bed), "stepped back, so a new frame does not move the view").toContainText("2 of 4");
  await viewer(bed).getByRole("button", { name: "Next file" }).click();
  await viewer(bed).getByRole("button", { name: "Next file" }).click();
  await expect(viewer(bed)).toContainText("4 of 4");
  bed.mock.shareMedia(imp, frame(2));
  await expect(viewer(bed), "on the newest, so the view follows").toContainText("5 of 5");
});

test("a frame that came out upside down turns in quarters and stays inside the screen", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, frame(1));
  await chip(bed).click();
  const image = viewer(bed).getByRole("img", { name: "frame1.png" });
  await expect(image).toBeVisible();
  const turnedBox = async () => (await image.boundingBox())!;
  await viewer(bed).getByRole("button", { name: "Rotate" }).click();
  const quarter = await turnedBox();
  const stage = (await viewer(bed).locator(".stage").boundingBox())!;
  expect(quarter.x, "inside the stage on the left").toBeGreaterThanOrEqual(stage.x - 1);
  expect(quarter.x + quarter.width, "inside the stage on the right").toBeLessThanOrEqual(stage.x + stage.width + 1);
  expect(quarter.y + quarter.height, "inside the stage at the foot").toBeLessThanOrEqual(stage.y + stage.height + 1);
  await viewer(bed).getByRole("button", { name: "Rotate" }).click();
  await expect(image).toHaveCSS("transform", /matrix\(-1, 0, 0, -1/);
  await bed.step("rotated", { compare: false });
});

test("a video plays from its own controls", async ({ bed, browserName }) => {
  await bed.openSeat(SEATS.imp);
  const webm = browserName === "chromium";
  bed.mock.shareMedia(imp, { name: webm ? "clip.webm" : "clip.mp4", mime: webm ? "video/webm" : "video/mp4", kind: "video", bytes: file(webm ? "clip.webm" : "clip.mp4") });
  await chip(bed).click();
  const video = viewer(bed).locator("video");
  await expect(video).toBeVisible();
  await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.duration), "the clip's metadata loaded").toBeGreaterThan(0.5);
  await expect(video).toHaveAttribute("controls", "");
  await expect(viewer(bed).getByRole("button", { name: "Rotate" }), "no rotate for a video").toHaveCount(0);
  await bed.step("video", { compare: false });
});

test("a file it cannot show gets a card and a Download, read in chunks and saved under its name", async ({ bed, browserName }) => {
  test.skip(browserName !== "chromium", "downloads are read through Chromium here");
  await bed.openSeat(SEATS.imp);
  const big = Buffer.alloc(4_500_000, 7);
  bed.mock.shareMedia(imp, { name: "print-log.gcode", mime: "text/plain", kind: "file", bytes: big });
  await chip(bed).click();
  await expect(viewer(bed)).toContainText("Treehouse cannot show this kind of file.");
  await expect(viewer(bed)).toContainText("File, 4.5 MB");
  expect(bed.mock.mediaReads, "nothing is read until it is asked for").toEqual([]);
  const download = bed.page.waitForEvent("download");
  await viewer(bed).getByRole("button", { name: "Download" }).click();
  const saved = await download;
  expect(saved.suggestedFilename()).toBe("print-log.gcode");
  expect(bed.mock.mediaReads.map((read) => read.offset), "read in 2 MB chunks").toEqual([0, 2_000_000, 4_000_000]);
  await expect(viewer(bed)).toContainText("Asked this device to save print-log.gcode");
});

test("a file the host no longer has says so in words and can be asked for again", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, { ...frame(1), unreadable: true });
  await chip(bed).click();
  await expect(viewer(bed).getByRole("alert")).toContainText("The host no longer has that file.");
  await expect(viewer(bed).getByRole("button", { name: "Try again" })).toBeVisible();
  await bed.step("gone", { compare: false });
});

test("works by keyboard: arrows step, Escape closes, and Tab stays inside the viewer", async ({ bed, browserName }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, frame(1));
  bed.mock.shareMedia(imp, frame(2));
  await chip(bed).click();
  await expect(viewer(bed)).toContainText("2 of 2");
  await bed.page.keyboard.press("ArrowLeft");
  await expect(viewer(bed)).toContainText("1 of 2");
  // Safari leaves buttons out of the Tab order unless its setting says otherwise.
  for (let press = 0; press < (browserName === "chromium" ? 8 : 0); press++) {
    await bed.page.keyboard.press("Tab");
    expect(await bed.page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]')), `Tab ${press + 1} stays in the viewer`).toBe(true);
  }
  await bed.page.keyboard.press("Escape");
  await expect(viewer(bed)).toHaveCount(0);
});

test("its controls are 44px targets that fit the screen", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, frame(1));
  await chip(bed).click();
  const width = bed.page.viewportSize()!.width;
  for (const name of ["Previous file", "Rotate", "Download", "Next file", "Close"]) {
    const box = (await viewer(bed).getByRole("button", { name }).boundingBox())!;
    expect(box.height, `${name} is 44px tall`).toBeGreaterThanOrEqual(44);
    expect(box.x + box.width, `${name} stays on screen`).toBeLessThanOrEqual(width);
  }
});
