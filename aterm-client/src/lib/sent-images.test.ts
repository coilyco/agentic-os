import { afterEach, describe, expect, it, vi } from "vitest";
import { imagePrefix } from "./images";
import { forgetSent, hostThumb, ImageReadRefused, localCopy, parseSent, rememberSent, sniffType } from "./sent-images";

const A = "/tmp/aterm-501/images/frontend-eng-imp-1700000000/9f393b42a47a664e944f3c97.png";
const B = "/tmp/aterm-501/images/frontend-eng-imp-1700000000/bc1b489c1ca571d84222a915.jpg";

afterEach(() => forgetSent());

describe("parseSent", () => {
  it("takes the paths out of a claude prompt and keeps the words", () => {
    // The shape a claude transcript holds, read from live seat files on 2026-11-27.
    expect(parseSent(`[Image #7][Attached image: ${A}] focus screen is broken?`)).toEqual({ words: "focus screen is broken?", paths: [A] });
    expect(parseSent(`[Image #14] [Image #15]${imagePrefix([A, B])} look at \`both\``)).toEqual({ words: "look at `both`", paths: [A, B] });
  });

  it("reads what the composer writes, whatever it writes", () => {
    expect(parseSent(`${imagePrefix([A])} hi`).paths).toEqual([A]);
    expect(parseSent(imagePrefix([A, B]))).toEqual({ words: "", paths: [A, B] });
  });

  it("leaves markdown and its indentation as the person typed it", () => {
    const words = "Fix these:\n\n- `one`\n  - nested\n- [two](https://example.com)\n\n    code";
    expect(parseSent(`[Image #1]${imagePrefix([A])} ${words}`).words).toBe(words);
    expect(parseSent(words)).toEqual({ words, paths: [] });
  });

  it("drops opencode markers and claude's paste wrapper", () => {
    expect(parseSent(`[Image 1] <pasted_content id="85ad">${imagePrefix([A])} long words</pasted_content id="85ad">`)).toEqual({ words: "long words", paths: [A] });
  });

  it("reads a leading bare path from a seat with no bracketed paste, never one among the words", () => {
    expect(parseSent(`${A} what is this`)).toEqual({ words: "what is this", paths: [A] });
    expect(parseSent(`open ${A} please`).paths).toEqual([]);
  });

  it("names each path once", () => {
    expect(parseSent(`${A} ${imagePrefix([A])} hi`).paths).toEqual([A]);
  });
});

describe("sniffType", () => {
  it("reads the type from the bytes", () => {
    expect(sniffType(Uint8Array.from([0x89, 0x50, 0x4e, 0x47, 13, 10]))).toBe("image/png");
    expect(sniffType(Uint8Array.from([0xff, 0xd8, 0xff, 0xe0]))).toBe("image/jpeg");
    expect(sniffType(new TextEncoder().encode("GIF89a"))).toBe("image/gif");
    expect(sniffType(new TextEncoder().encode("RIFF\0\0\0\0WEBPVP8 "))).toBe("image/webp");
    expect(sniffType(new TextEncoder().encode("<html>"))).toBe("application/octet-stream");
  });
});

describe("this device's copies", () => {
  it("keeps the newest 32", () => {
    for (let index = 0; index < 40; index++) rememberSent(`/p/${index}.png`, new Blob([String(index)]));
    expect(localCopy("/p/0.png")).toBeUndefined();
    expect(localCopy("/p/8.png")).toBeDefined();
    expect(localCopy("/p/39.png")).toBeDefined();
  });
});

describe("hostThumb", () => {
  it("asks once per path and keeps a refusal that will not change", async () => {
    const read = vi.fn(async (_session: string, path: string) => {
      if (path === A) return new Blob(["x"]);
      throw new ImageReadRefused("image_gone");
    });
    await hostThumb(read, "seat", A);
    await hostThumb(read, "seat", A);
    await expect(hostThumb(read, "seat", B)).rejects.toThrow("no longer holds");
    await expect(hostThumb(read, "seat", B)).rejects.toThrow();
    expect(read).toHaveBeenCalledTimes(2);
  });

  it("asks again after a failure that may pass", async () => {
    const read = vi.fn().mockRejectedValueOnce(new Error("dropped")).mockResolvedValue(new Blob(["x"]));
    await expect(hostThumb(read, "seat", A)).rejects.toThrow("dropped");
    await Promise.resolve();
    await expect(hostThumb(read, "seat", A)).resolves.toBeInstanceOf(Blob);
    expect(read).toHaveBeenCalledTimes(2);
  });
});
