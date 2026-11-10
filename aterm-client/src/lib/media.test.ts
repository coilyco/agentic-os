import { describe, expect, it } from "vitest";
import { addToShelf, decodeMedia, describe as words, MAX_SHELF, mediaRefusal, removeFromShelf, stepped, turned, unseenCount, type MediaItem } from "./media";

const item = (id: string, extra: Partial<MediaItem> = {}): MediaItem => ({ id, session: "s", name: `${id}.jpg`, mime: "image/jpeg", kind: "image", size: 1000, ...extra });

describe("an item off the wire", () => {
  it("reads the fields and the time, as a string or a number", () => {
    expect(decodeMedia("s", { id: "m1", name: "a.png", mime: "image/png", kind: "image", size: 2803, at: "2026-11-10T21:00:00Z" })).toEqual({ id: "m1", session: "s", name: "a.png", mime: "image/png", kind: "image", size: 2803, at: Date.parse("2026-11-10T21:00:00Z") });
    expect(decodeMedia("s", { id: "m1", name: "a", kind: "file", at: 5 })?.at).toBe(5);
  });

  it("drops anything without an id, a name or a known kind, so an odd frame never breaks the shelf", () => {
    for (const bad of [null, "x", {}, { id: "", name: "a", kind: "image" }, { id: "1", kind: "image" }, { id: "1", name: "a", kind: "pdf" }]) expect(decodeMedia("s", bad), JSON.stringify(bad)).toBeNull();
  });

  it("fills a missing mime and size with plain defaults, and an empty name with words", () => {
    expect(decodeMedia("s", { id: "1", name: "", kind: "file" })).toMatchObject({ mime: "application/octet-stream", size: 0, name: "Unnamed file" });
  });
});

describe("the shelf of what a seat shared", () => {
  it("keeps items in the order they came, and replaces a repeat of an id in place", () => {
    const shelf = addToShelf(addToShelf([], item("a")), item("b"));
    expect(addToShelf(shelf, item("a", { name: "renamed.jpg" })).map((each) => each.name)).toEqual(["renamed.jpg", "b.jpg"]);
  });

  it("keeps only the newest 32", () => {
    let shelf: MediaItem[] = [];
    for (let index = 0; index < 40; index++) shelf = addToShelf(shelf, item(`m${index}`));
    expect(shelf).toHaveLength(MAX_SHELF);
    expect(shelf[0]!.id).toBe("m8");
    expect(shelf.at(-1)!.id).toBe("m39");
  });

  it("removes an item the host closed", () => {
    expect(removeFromShelf([item("a"), item("b")], "a").map((each) => each.id)).toEqual(["b"]);
  });

  it("counts what came after the last one seen, or everything when none was", () => {
    const shelf = [item("a"), item("b"), item("c")];
    expect(unseenCount(shelf, undefined)).toBe(3);
    expect(unseenCount(shelf, "a")).toBe(2);
    expect(unseenCount(shelf, "c")).toBe(0);
    expect(unseenCount(shelf, "gone"), "a seen item the host evicted counts all").toBe(3);
    expect(unseenCount([], undefined)).toBe(0);
  });
});

describe("moving through a sequence", () => {
  it("steps one at a time and stops at the ends", () => {
    expect(stepped(0, 3, -1)).toBe(0);
    expect(stepped(0, 3, 1)).toBe(1);
    expect(stepped(2, 3, 1)).toBe(2);
    expect(stepped(0, 1, 1)).toBe(0);
  });

  it("turns in quarters and wraps, so upside down takes two", () => {
    expect(turned(0)).toBe(90);
    expect(turned(turned(0))).toBe(180);
    expect(turned(270)).toBe(0);
    expect(turned(0, -1)).toBe(270);
  });
});

describe("words", () => {
  it("names what an item is and how big", () => {
    expect(words(item("a", { size: 117_000 }))).toBe("Image, 117 KB");
    expect(words(item("v", { kind: "video", size: 2_500_000 }))).toBe("Video, 2.5 MB");
    expect(words(item("f", { kind: "file", size: 0 }))).toBe("File");
  });

  it("says each refusal in a sentence, and a reason it does not know in a plain one", () => {
    for (const reason of ["media_unknown", "too_large", "locked"]) expect(mediaRefusal(reason)).not.toMatch(/_/);
    expect(mediaRefusal("something_new")).toBe("The host would not send that file.");
    expect(mediaRefusal("constructor")).toBe("The host would not send that file.");
  });
});
