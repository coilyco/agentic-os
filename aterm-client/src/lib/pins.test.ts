import { describe, expect, it } from "vitest";
import { flipPin, isPinned, NO_PINS, parsePins } from "./pins";

describe("parsePins", () => {
  it("reads both lists", () => {
    expect(parsePins('{"pinned":["a"],"unpinned":["b"]}')).toEqual({ pinned: ["a"], unpinned: ["b"] });
  });

  it("reads nothing, junk, or a wrong shape as no pins", () => {
    expect(parsePins(null)).toEqual(NO_PINS);
    expect(parsePins("not json")).toEqual(NO_PINS);
    expect(parsePins('{"pinned":"a","unpinned":[1,"b"]}')).toEqual({ pinned: [], unpinned: ["b"] });
  });
});

describe("isPinned", () => {
  it("follows the role's default until the person decides", () => {
    expect(isPinned("a", NO_PINS, true)).toBe(true);
    expect(isPinned("a", NO_PINS, false)).toBe(false);
  });

  it("lets the person's pin and unpin win over the default", () => {
    expect(isPinned("a", { pinned: ["a"], unpinned: [] }, false)).toBe(true);
    expect(isPinned("a", { pinned: [], unpinned: ["a"] }, true)).toBe(false);
  });
});

describe("flipPin", () => {
  it("pins an unpinned seat and keeps it out of the unpinned list", () => {
    expect(flipPin("a", false, { pinned: [], unpinned: ["a"] })).toEqual({ pinned: ["a"], unpinned: [] });
  });

  it("unpins a pinned seat, and records it so a default does not return", () => {
    expect(flipPin("a", true, { pinned: ["a", "b"], unpinned: [] })).toEqual({ pinned: ["b"], unpinned: ["a"] });
  });

  it("is the same after flipping twice", () => {
    const once = flipPin("a", false, NO_PINS);
    expect(isPinned("a", flipPin("a", true, once), false)).toBe(false);
  });
});
