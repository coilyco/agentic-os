import { beforeEach, describe, expect, it } from "vitest";
import { loadWrap, storeWrap } from "./wrap";

const shelf = new Map<string, string>();
beforeEach(() => {
  shelf.clear();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: (key: string) => shelf.get(key) ?? null, setItem: (key: string, value: string) => void shelf.set(key, value) },
  });
});

describe("the remembered wrap switch", () => {
  it("is off until a person turns it on for that seat", () => {
    expect(loadWrap("imp")).toBe(false);
    storeWrap("imp", true);
    expect(loadWrap("imp")).toBe(true);
    expect(loadWrap("sprite")).toBe(false);
  });

  it("turns off again, leaving the other seats as they were", () => {
    storeWrap("imp", true);
    storeWrap("sprite", true);
    storeWrap("imp", false);
    expect([loadWrap("imp"), loadWrap("sprite")]).toEqual([false, true]);
  });

  it("reads a stored value that is not a list as off, so a hand-edited one cannot throw", () => {
    for (const bad of ["", "{", "7", "null", '{"imp":true}']) {
      shelf.set("aterm.terminal-wrap.v1", bad);
      expect(loadWrap("imp"), bad).toBe(false);
    }
  });

  it("keeps only the newest 64 seats", () => {
    for (let seat = 0; seat < 70; seat++) storeWrap(`seat-${seat}`, true);
    expect([loadWrap("seat-0"), loadWrap("seat-5"), loadWrap("seat-6"), loadWrap("seat-69")]).toEqual([false, false, true, true]);
  });

  it("survives storage that throws", () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: { getItem: () => { throw new Error("off"); }, setItem: () => { throw new Error("off"); } },
    });
    expect(() => storeWrap("imp", true)).not.toThrow();
    expect(loadWrap("imp")).toBe(false);
  });
});
