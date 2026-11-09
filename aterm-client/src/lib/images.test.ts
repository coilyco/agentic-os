import { describe, expect, it } from "vitest";
import { IMAGE_UPLOAD_MAX_BYTES, MAX_IMAGES, attachNote, chipLabel, fitAttempts, formatSize, imagePrefix, imageRefusal, pastedImages, pasteSteps, screenImages, typedPath, withImagePaths } from "./images";

function file(name: string, type: string, size = 100): File {
  return new File([new Uint8Array(size)], name, { type });
}

const channel = { maxBytes: 1024 * 1024 };

describe("formatSize", () => {
  it("reads in the unit a person weighs a screenshot in", () => {
    expect(formatSize(0)).toBe("0 B");
    expect(formatSize(900)).toBe("900 B");
    expect(formatSize(240_000)).toBe("240 KB");
    expect(formatSize(1_500_000)).toBe("1.5 MB");
    expect(formatSize(3_000_000)).toBe("3 MB");
  });

  it("never rounds a small file down to 0 KB", () => {
    expect(formatSize(1100)).toBe("1 KB");
  });
});

describe("chipLabel", () => {
  it("counts from one and carries the size, which is what a screen reader hears", () => {
    expect(chipLabel(0, 240_000)).toBe("Image 1, 240 KB");
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

  it("takes a still image over the limit, to be shrunk, since a retina screenshot often is", () => {
    const big = file("big.png", "image/png", 2 * 1024 * 1024);
    expect(screenImages([big], 0, channel)).toEqual({ accepted: [big], refusal: null });
  });

  it("refuses a GIF over the limit by name, since shrinking it would flatten its motion", () => {
    const out = screenImages([file("loop.gif", "image/gif", 2 * 1024 * 1024)], 0, channel);
    expect(out.accepted).toEqual([]);
    expect(out.refusal).toBe("loop.gif is 2.1 MB, over the 1 MB this host takes, and a GIF cannot be shrunk without losing its motion.");
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

describe("imageRefusal", () => {
  it("puts each reason the daemon names in words", () => {
    expect(imageRefusal("not_an_image")).toBe("The host did not read that as a PNG, JPEG, GIF or WebP image.");
    expect(imageRefusal("too_large")).toBe("The host took that as over 3 MB.");
    expect(imageRefusal("too_many")).toMatch(/16 images/);
    expect(imageRefusal("session_unknown")).toMatch(/not attached to this seat/);
  });

  it("leaves every other reason to the lock and the generic error", () => {
    expect(imageRefusal("passkey_required")).toBeNull();
    expect(imageRefusal("session_descendant")).toBeNull();
    expect(imageRefusal(undefined)).toBeNull();
  });

  it("states the limit the frame sets", () => {
    expect(IMAGE_UPLOAD_MAX_BYTES).toBe(3_000_000);
  });
});

describe("pasteSteps", () => {
  const open = "<";
  const close = ">";

  it("pastes each path on its own, then one paste of the words that names every path", () => {
    expect(pasteSteps(["/t/a.png", "/t/b.png"], "what is this", open, close)).toEqual([
      "</t/a.png>",
      "</t/b.png>",
      "<[Attached image 1 of 2: /t/a.png] [Attached image 2 of 2: /t/b.png] what is this>",
    ]);
  });

  it("still names the path for an image sent with no words, so a late attachment is not lost", () => {
    expect(pasteSteps(["/t/a.png"], "", open, close)).toEqual(["</t/a.png>", "<[Attached image: /t/a.png]>"]);
  });

  it("is one paste for a text-only message, as before", () => {
    expect(pasteSteps([], "hello", open, close)).toEqual(["<hello>"]);
  });
});

describe("fitAttempts", () => {
  it("shrinks a PNG as a smaller PNG first, then as a JPEG", () => {
    expect(fitAttempts("image/png")).toEqual([
      { scale: 0.75, type: "image/png" },
      { scale: 0.5, type: "image/png" },
      { scale: 1, type: "image/jpeg" },
      { scale: 0.75, type: "image/jpeg" },
      { scale: 0.5, type: "image/jpeg" },
    ]);
  });

  it("goes straight to JPEG for a photograph", () => {
    expect(fitAttempts("image/jpeg").every((attempt) => attempt.type === "image/jpeg")).toBe(true);
    expect(fitAttempts("image/webp")[0]).toEqual({ scale: 1, type: "image/jpeg" });
  });
});

describe("attachNote", () => {
  it("tells the person when a harness reads the path as text", () => {
    expect(attachNote("goose")).toBe("goose does not attach images, so it got the file's path to open instead.");
  });

  it("says nothing for a harness that attaches", () => {
    for (const harness of ["claude", "codex", "opencode"]) expect(attachNote(harness)).toBeNull();
  });
});

describe("imagePrefix", () => {
  it("is nothing without an image", () => {
    expect(imagePrefix([])).toBe("");
  });

  it("names one image plainly and several by their place, each with its absolute path in brackets", () => {
    expect(imagePrefix(["/tmp/a b/x.png"])).toBe("[Attached image: /tmp/a b/x.png]");
    expect(imagePrefix(["/t/a.png", "/t/b.png"])).toBe("[Attached image 1 of 2: /t/a.png] [Attached image 2 of 2: /t/b.png]");
  });
});
