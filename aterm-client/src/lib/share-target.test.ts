import { describe, expect, it, vi } from "vitest";
import { parseShared, skipWords, takeShared } from "./share-target";

const PNG = { name: "Screenshot_1.png", mime: "image/png", data: btoa("\x89PNG\x01\x02") };

describe("what the plugin returns", () => {
  it("is nothing when nothing waited, so a launch opens no sheet", () => {
    expect(parseShared({ images: [], skipped: [], text: null })).toBeNull();
    expect(parseShared(null)).toBeNull();
    expect(parseShared("garbage")).toBeNull();
  });

  it("decodes an image into a file with the bytes, name and type it was sent with", async () => {
    const shared = parseShared({ images: [PNG], skipped: [], text: "look at this" });
    expect(shared?.text).toBe("look at this");
    const file = shared?.files[0];
    expect(file?.name).toBe("Screenshot_1.png");
    expect(file?.type).toBe("image/png");
    expect(Array.from(new Uint8Array(await file!.arrayBuffer()))).toEqual([0x89, 0x50, 0x4e, 0x47, 1, 2]);
  });

  it("says in words what it left out, for every reason the plugin has", () => {
    const shared = parseShared({
      images: [],
      skipped: [
        { name: "clip.mp4", reason: "not_image" },
        { name: "big.png", reason: "too_big" },
        { name: "gone.png", reason: "unreadable" },
        { name: "fifth.png", reason: "too_many" },
        { name: "odd.png", reason: "from_the_future" },
      ],
    });
    expect(shared?.files).toEqual([]);
    expect(shared?.notes).toEqual([
      skipWords("clip.mp4", "not_image"),
      skipWords("big.png", "too_big"),
      skipWords("gone.png", "unreadable"),
      skipWords("fifth.png", "too_many"),
      // An unknown reason is read as the cautious one, never dropped.
      skipWords("odd.png", "unreadable"),
    ]);
    expect(shared?.notes[1]).toContain("25 MB");
  });

  it("keeps an image type the seat cannot read out of the files and says so", () => {
    const shared = parseShared({ images: [{ name: "photo.heic", mime: "image/heic", data: btoa("x") }, PNG], skipped: [] });
    expect(shared?.files.map((file) => file.name)).toEqual(["Screenshot_1.png"]);
    expect(shared?.notes).toEqual([skipWords("photo.heic", "not_image")]);
  });

  it("turns an item it cannot decode into a note instead of throwing", () => {
    const shared = parseShared({ images: [{ name: "bad.png", mime: "image/png", data: "%%%not base64%%%" }, { name: "empty.png", mime: "image/png", data: "" }], skipped: [] });
    expect(shared?.files).toEqual([]);
    expect(shared?.notes).toEqual([skipWords("bad.png", "unreadable"), skipWords("empty.png", "unreadable")]);
  });

  it("opens a sheet for words alone", () => {
    expect(parseShared({ images: [], skipped: [], text: "  just words " })).toEqual({ files: [], text: "just words", notes: [] });
  });
});

describe("takeShared", () => {
  it("asks the plugin's take and decodes the answer", async () => {
    const invoke = vi.fn().mockResolvedValue({ images: [PNG], skipped: [], text: null });
    const shared = await takeShared(invoke);
    expect(invoke).toHaveBeenCalledWith("plugin:sharetarget|take");
    expect(shared?.files).toHaveLength(1);
  });

  it("is null in a browser, and when the call fails, which an app without the plugin also does", async () => {
    expect(await takeShared(null)).toBeNull();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(await takeShared(vi.fn().mockRejectedValue({ code: "other", message: "boom" }))).toBeNull();
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});
