import { beforeEach, describe, expect, it } from "vitest";
import { loadWrap, loadWrapDefault, loadWrapPick, storeWrap, storeWrapDefault, wrapsByDefault } from "./wrap";

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

describe("a person's own pick for a seat", () => {
  it("is nothing until the button is pressed, then on or off for that seat only", () => {
    expect(loadWrapPick("imp")).toBeUndefined();
    storeWrap("imp", true);
    storeWrap("sprite", false);
    expect([loadWrapPick("imp"), loadWrapPick("sprite"), loadWrapPick("beetle")]).toEqual([true, false, undefined]);
  });

  it("changes with each press, so the last one wins", () => {
    storeWrap("imp", false);
    storeWrap("imp", true);
    expect(loadWrapPick("imp")).toBe(true);
    storeWrap("imp", false);
    expect(loadWrapPick("imp")).toBe(false);
  });
});

describe("the device default", () => {
  it("is Auto until chosen, and a hand-edited value reads as Auto", () => {
    expect(loadWrapDefault()).toBe("auto");
    for (const bad of ["", "maybe", "{", "7"]) {
      shelf.set("aterm.terminal-wrap-default.v1", bad);
      expect(loadWrapDefault(), bad).toBe("auto");
    }
    storeWrapDefault("on");
    expect(loadWrapDefault()).toBe("on");
    storeWrapDefault("off");
    expect(loadWrapDefault()).toBe("off");
  });

  it("wraps on a phone or narrow window under Auto, never on a desktop, and On and Off ignore the screen", () => {
    expect([wrapsByDefault("auto", true), wrapsByDefault("auto", false)]).toEqual([true, false]);
    expect([wrapsByDefault("on", true), wrapsByDefault("on", false)]).toEqual([true, true]);
    expect([wrapsByDefault("off", true), wrapsByDefault("off", false)]).toEqual([false, false]);
  });
});
