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

test("inside an app without the save plugin, it says so and offers none", async ({ bed }) => {
  await bed.page.addInitScript(() => {
    (window as unknown as { __TAURI__: unknown }).__TAURI__ = { core: { invoke: async () => undefined } };
  });
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, { name: "print-log.gcode", mime: "text/plain", kind: "file", bytes: Buffer.from("G28\n") });
  await chip(bed).click();
  await expect(viewer(bed)).toContainText("This app cannot save files yet. Open Treehouse in a browser on this device to download it.");
  await expect(viewer(bed).getByRole("button", { name: "Download" }), "no button that does nothing").toHaveCount(0);
  await expect(viewer(bed)).not.toContainText("Download it to open it elsewhere.");
  await bed.step("in-app-file", { compare: false });
});

/** The app's bridge with the savefile plugin. It can refuse the Nth write. */
async function appWithSave(bed: Bed, refuseWrite = 0): Promise<void> {
  await bed.page.addInitScript((refuse) => {
    const w = window as unknown as { __TAURI__: unknown; saves: { commands: string[]; bytes: number; name: string } };
    w.saves = { commands: [], bytes: 0, name: "" };
    let writes = 0;
    w.__TAURI__ = {
      core: {
        invoke: async (command: string, args?: { name?: string; data?: string }) => {
          if (!command.startsWith("plugin:savefile|")) return null;
          const verb = command.replace("plugin:savefile|", "");
          w.saves.commands.push(verb);
          if (verb === "available") return { place: "your Downloads folder" };
          if (verb === "begin") return ((w.saves.name = args?.name ?? ""), { id: "1", name: "print-log (1).gcode" });
          if (verb === "write") {
            if (++writes === refuse) throw { code: "unwritable", message: "No space left on device" };
            w.saves.bytes += atob(args?.data ?? "").length;
            return null;
          }
          if (verb === "finish") return { name: "print-log (1).gcode", place: "your Downloads folder" };
          return null;
        },
      },
    };
  }, refuseWrite);
}
const saves = (bed: Bed) => bed.page.evaluate(() => (window as unknown as { saves: { commands: string[]; bytes: number; name: string } }).saves);

test("inside the app with the save plugin, Download writes the file to Downloads in pieces and says where", async ({ bed }) => {
  await appWithSave(bed);
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, { name: "print-log.gcode", mime: "text/plain", kind: "file", bytes: Buffer.alloc(1_000_000, 7) });
  await chip(bed).click();
  await expect(viewer(bed)).toContainText("Download it to open it elsewhere.");
  await expect(viewer(bed)).not.toContainText("cannot save files");
  await viewer(bed).getByRole("button", { name: "Download" }).click();
  await expect(viewer(bed)).toContainText("Saved print-log (1).gcode to your Downloads folder.");
  const done = await saves(bed);
  expect(done.name).toBe("print-log.gcode");
  expect(done.bytes, "every byte went over").toBe(1_000_000);
  expect(done.commands.slice(0, 2)).toEqual(["available", "begin"]);
  expect(done.commands.at(-1)).toBe("finish");
  await bed.step("in-app-saved", { compare: false });
});

test("when the app cannot write the file the viewer says why, and it can be tried again", async ({ bed }) => {
  await appWithSave(bed, 1);
  await bed.openSeat(SEATS.imp);
  bed.mock.shareMedia(imp, { name: "print-log.gcode", mime: "text/plain", kind: "file", bytes: Buffer.alloc(1_000_000, 7) });
  await chip(bed).click();
  await viewer(bed).getByRole("button", { name: "Download" }).click();
  await expect(viewer(bed)).toContainText("print-log.gcode (1 MB) was not saved: No space left on device. Nothing was left in Downloads.");
  expect((await saves(bed)).commands.at(-1), "what it opened was cancelled").toBe("cancel");
  await expect(viewer(bed).getByRole("button", { name: "Download" })).toBeEnabled();
  await bed.step("in-app-save-refused", { compare: false });
});
