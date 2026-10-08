import { describe, expect, it } from "vitest";
import { clampSide, keyedSide, loadSide, MAIN_MIN, SIDE_MIN, storeSide } from "./split";

describe("clampSide", () => {
  it("keeps the side panel readable and leaves the seat room, on a 1280 wide window", () => {
    expect(clampSide(10, 1000)).toBe(SIDE_MIN);
    expect(clampSide(5000, 1000)).toBe(1000 - MAIN_MIN);
    expect(clampSide(512.4, 1000)).toBe(512);
  });

  it("prefers a readable side panel to a roomy seat when the window is too small for both", () => {
    expect(clampSide(300, 500)).toBe(SIDE_MIN);
  });
});

describe("keyedSide", () => {
  it("moves the edge the way the arrow points, bigger with shift", () => {
    expect(keyedSide("ArrowLeft", false, 400, 1280)).toBe(424);
    expect(keyedSide("ArrowRight", false, 400, 1280)).toBe(376);
    expect(keyedSide("ArrowLeft", true, 400, 1280)).toBe(496);
  });

  it("stops at both ends, and Home and End jump to them", () => {
    expect(keyedSide("ArrowRight", false, SIDE_MIN, 1280)).toBe(SIDE_MIN);
    expect(keyedSide("ArrowLeft", false, 1280 - MAIN_MIN, 1280)).toBe(1280 - MAIN_MIN);
    expect(keyedSide("Home", false, 400, 1280)).toBe(1280 - MAIN_MIN);
    expect(keyedSide("End", false, 400, 1280)).toBe(SIDE_MIN);
  });

  it("leaves other keys alone, so Tab still leaves the handle", () => {
    expect(keyedSide("Tab", false, 400, 1280)).toBeNull();
  });
});

describe("stored width", () => {
  it("is remembered, forgotten on reset, and ignored when it is not a usable width", () => {
    const data = new Map<string, string>();
    const storage = { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => void data.set(key, value), removeItem: (key: string) => void data.delete(key) };
    storeSide(512, storage);
    expect(loadSide(storage)).toBe(512);
    storeSide(null, storage);
    expect(loadSide(storage)).toBeNull();
    storeSide(40, storage);
    expect(loadSide(storage)).toBeNull();
    data.set("aterm.side-width.v1", "wide");
    expect(loadSide(storage)).toBeNull();
  });
});
