import { beforeEach, describe, expect, it } from "vitest";
import { defaultView, loadView, loadViewDefault, storeView, storeViewDefault } from "./seat-view";

const shelf = new Map<string, string>();
beforeEach(() => {
  shelf.clear();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: (key: string) => shelf.get(key) ?? null, setItem: (key: string, value: string) => void shelf.set(key, value) },
  });
});

describe("the view a seat opens to", () => {
  it("is Focus on a phone and the screen on a desktop, until a person picks", () => {
    expect(loadView("imp", true)).toEqual({ level: "focus", raw: false });
    expect(loadView("imp", false)).toEqual({ level: "verbose", raw: true });
    expect(defaultView(true).raw).toBe(false);
  });

  it("keeps a pick for that seat only", () => {
    storeView("imp", { level: "default", raw: false });
    expect(loadView("imp", true)).toEqual({ level: "default", raw: false });
    expect(loadView("sprite", true)).toEqual({ level: "focus", raw: false });
  });

  it("ignores a stored value that is not a view, so a hand-edited one cannot break the seat", () => {
    for (const bad of ["", "{", "7", '[["imp",{"level":"huge"}]]', '[["imp"]]', '{"imp":1}']) {
      shelf.set("aterm.seat-view.v1", bad);
      expect(loadView("imp", true), bad).toEqual({ level: "focus", raw: false });
    }
  });

  it("keeps only the newest 64 seats and survives storage that throws", () => {
    for (let seat = 0; seat < 70; seat++) storeView(`seat-${seat}`, { level: "default", raw: false });
    expect(loadView("seat-0", true).level).toBe("focus");
    expect(loadView("seat-69", true).level).toBe("default");
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: { getItem: () => { throw new Error("off"); }, setItem: () => { throw new Error("off"); } } });
    expect(() => storeView("imp", { level: "default", raw: false })).not.toThrow();
    expect(loadView("imp", true).level).toBe("focus");
  });
});

describe("the default a person sets for seats with no view of their own", () => {
  it("is Auto until chosen, and a hand-edited value reads as Auto", () => {
    expect(loadViewDefault()).toBe("auto");
    for (const bad of ["", "huge", "{", "7"]) {
      shelf.set("aterm.seat-view-default.v1", bad);
      expect(loadViewDefault(), bad).toBe("auto");
    }
  });

  it("changes where a seat opens on a phone and a desktop, and Auto puts it back", () => {
    storeViewDefault("default");
    expect(loadView("imp", true)).toEqual({ level: "default", raw: false });
    expect(loadView("imp", false)).toEqual({ level: "default", raw: false });
    storeViewDefault("auto");
    expect(loadView("imp", true)).toEqual({ level: "focus", raw: false });
    expect(loadView("imp", false)).toEqual({ level: "verbose", raw: true });
  });

  it("never overrides a view picked for that seat", () => {
    storeView("imp", { level: "focus", raw: false });
    storeViewDefault("verbose");
    expect(loadView("imp", false)).toEqual({ level: "focus", raw: false });
    expect(loadView("sprite", false)).toEqual({ level: "verbose", raw: false });
  });
});
