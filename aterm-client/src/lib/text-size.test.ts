import { beforeEach, describe, expect, it } from "vitest";
import { DEFAULT_TEXT_SIZE, loadTextSize, stepTextSize, storeTextSize, TEXT_SIZES } from "./text-size";

const shelf = new Map<string, string>();
beforeEach(() => {
  shelf.clear();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: (key: string) => shelf.get(key) ?? null, setItem: (key: string, value: string) => void shelf.set(key, value) },
  });
});

describe("the remembered text size", () => {
  it("is today's 14 until a person picks one", () => {
    expect(DEFAULT_TEXT_SIZE).toBe(14);
    expect(loadTextSize()).toBe(14);
  });

  it("comes back as stored", () => {
    storeTextSize(11);
    expect(loadTextSize()).toBe(11);
  });

  it("ignores a stored value that is not a step, so a stale or hand-edited one cannot size the terminal", () => {
    for (const bad of ["", "0", "abc", "15", "-14", "99", "NaN"]) {
      shelf.set("aterm.terminal-text-size.v1", bad);
      expect(loadTextSize(), bad).toBe(14);
    }
  });

  it("falls back to 14 when storage throws", () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: { getItem: () => { throw new Error("off"); }, setItem: () => { throw new Error("off"); } },
    });
    expect(loadTextSize()).toBe(14);
    expect(() => storeTextSize(12)).not.toThrow();
  });
});

describe("stepTextSize", () => {
  it("moves one step and stops at both ends", () => {
    expect(stepTextSize(14, -1)).toBe(13);
    expect(stepTextSize(14, 1)).toBe(16);
    expect(stepTextSize(TEXT_SIZES[0], -1)).toBe(TEXT_SIZES[0]);
    expect(stepTextSize(TEXT_SIZES.at(-1)!, 1)).toBe(TEXT_SIZES.at(-1));
  });

  it("starts from the nearest step when the size is not one", () => {
    expect(stepTextSize(15, -1)).toBe(14);
    expect(stepTextSize(15, 1)).toBe(16);
  });
});
