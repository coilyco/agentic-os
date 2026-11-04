import { describe, expect, it } from "vitest";
import { MAX_IMAGES, chipLabel, formatSize, pastedImages, screenImages, typedPath, withImagePaths } from "./images";

function file(name: string, type: string, size = 100): File {
  return new File([new Uint8Array(size)], name, { type });
}

const channel = { maxBytes: 1024 * 1024 };

describe("formatSize", () => {
  it("reads in the unit a person weighs a screenshot in", () => {
    expect(formatSize(0)).toBe("0 B");
    expect(formatSize(900)).toBe("900 B");
    expect(formatSize(240 * 1024)).toBe("240 KB");
    expect(formatSize(1.5 * 1024 * 1024)).toBe("1.5 MB");
  });

  it("never rounds a small file down to 0 KB", () => {
    expect(formatSize(1100)).toBe("1 KB");
  });
});

describe("chipLabel", () => {
  it("counts from one and carries the size, which is what a screen reader hears", () => {
    expect(chipLabel(0, 240 * 1024)).toBe("Image 1, 240 KB");
    expect(chipLabel(2, 900)).toBe("Image 3, 900 B");
  });
});

describe("pastedImages", () => {
  const clip = (types: string[], files: File[]) => ({ types, files }) as unknown as DataTransfer;

  it("takes a screenshot, which carries a file and no text", () => {
    expect(pastedImages(clip(["Files"], [file("shot.png", "image/png")]))).toHaveLength(1);
  });

  it("takes a copied browser image, which carries html and a file but no plain text", () => {
    expect(pastedImages(clip(["text/html", "Files"], [file("a.png", "image/png")]))).toHaveLength(1);
  });

  it("leaves a spreadsheet cell as text, though it carries a picture of itself", () => {
    expect(pastedImages(clip(["text/plain", "text/html", "Files"], [file("cell.png", "image/png")]))).toEqual([]);
  });

  it("ignores a pasted file that is not an image, and a paste with no data", () => {
    expect(pastedImages(clip(["Files"], [file("notes.pdf", "application/pdf")]))).toEqual([]);
    expect(pastedImages(null)).toEqual([]);
  });
});

describe("screenImages", () => {
  it("takes PNG, JPEG, GIF and WebP", () => {
    const files = ["png", "jpeg", "gif", "webp"].map((kind) => file(`a.${kind}`, `image/${kind}`));
    expect(screenImages(files, 0, channel)).toEqual({ accepted: files, refusal: null });
  });

  it("says nothing for an empty paste", () => {
    expect(screenImages([], 0, channel)).toEqual({ accepted: [], refusal: null });
  });

  it("says the host cannot take images, rather than dropping them, when it has no channel", () => {
    const out = screenImages([file("a.png", "image/png")], 0, undefined);
    expect(out.accepted).toEqual([]);
    expect(out.refusal).toBe("This host cannot take images, so nothing was added. It needs a newer aterm daemon.");
  });

  it("refuses a file that is not an image by name, and keeps the good one", () => {
    const good = file("ok.png", "image/png");
    const out = screenImages([file("notes.pdf", "application/pdf"), good], 0, channel);
    expect(out.accepted).toEqual([good]);
    expect(out.refusal).toBe("notes.pdf is not an image the seat can read. PNG, JPEG, GIF and WebP work.");
  });

  it("refuses an SVG, which can carry script and which no harness reads as a picture", () => {
    expect(screenImages([file("a.svg", "image/svg+xml")], 0, channel).accepted).toEqual([]);
  });

  it("names the size and the limit for an image over the host's limit", () => {
    const out = screenImages([file("big.png", "image/png", 2 * 1024 * 1024)], 0, channel);
    expect(out.accepted).toEqual([]);
    expect(out.refusal).toBe("big.png is 2.0 MB, over the 1.0 MB this host takes.");
  });

  it("stops at the most one message carries, counting chips already waiting", () => {
    const files = Array.from({ length: 3 }, (_, i) => file(`${i}.png`, "image/png"));
    const out = screenImages(files, MAX_IMAGES - 1, channel);
    expect(out.accepted).toEqual([files[0]]);
    expect(out.refusal).toBe(`A message carries ${MAX_IMAGES} images at most. Send these, then add more.`);
  });

  it("words only the first refusal", () => {
    const out = screenImages([file("a.pdf", "application/pdf"), file("b.pdf", "application/pdf")], 0, channel);
    expect(out.refusal).toBe("a.pdf is not an image the seat can read. PNG, JPEG, GIF and WebP work.");
  });

  it("does not trust an empty name", () => {
    expect(screenImages([file("", "application/pdf")], 0, channel).refusal).toMatch(/^That file is not an image/);
  });
});

describe("typedPath", () => {
  it("leaves a plain path alone", () => {
    expect(typedPath("/Users/kai/.aterm/images/a1b2.png")).toBe("/Users/kai/.aterm/images/a1b2.png");
  });

  it("escapes the space in a macOS state directory the way a terminal drop does", () => {
    expect(typedPath("/Users/kai/Library/Application Support/aterm/a.png")).toBe("/Users/kai/Library/Application\\ Support/aterm/a.png");
  });

  it("escapes shell syntax, so a path never reads as a second command", () => {
    expect(typedPath("/t/a(1)&b;c.png")).toBe("/t/a\\(1\\)\\&b\\;c.png");
  });
});

describe("withImagePaths", () => {
  it("puts the paths ahead of the words", () => {
    expect(withImagePaths("what is wrong here", ["/t/a.png", "/t/b.png"])).toBe("/t/a.png /t/b.png what is wrong here");
  });

  it("sends the paths alone when there are no words", () => {
    expect(withImagePaths("", ["/t/a.png"])).toBe("/t/a.png");
  });

  it("changes nothing for a text-only message", () => {
    expect(withImagePaths("hello", [])).toBe("hello");
  });
});
