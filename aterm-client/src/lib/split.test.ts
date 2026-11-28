import { describe, expect, it } from "vitest";
import { clampSide, keyedSide, loadSide, loadSideOpen, MAIN_MIN, SIDE_MIN, storeSide, storeSideOpen } from "./split";

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

describe("stored open state", () => {
  const stub = () => {
    const data = new Map<string, string>();
    return { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => void data.set(key, value), removeItem: (key: string) => void data.delete(key) };
  };

  it("starts put away, since an empty panel costs the terminal 360px", () => {
    expect(loadSideOpen("seat-a", stub())).toBe(false);
  });

  it("keeps what a person chose for each seat, either way", () => {
    const storage = stub();
    storeSideOpen("seat-a", true, storage);
    storeSideOpen("seat-b", false, storage);
    expect(loadSideOpen("seat-a", storage)).toBe(true);
    expect(loadSideOpen("seat-b", storage)).toBe(false);
    storeSideOpen("seat-a", false, storage);
    expect(loadSideOpen("seat-a", storage)).toBe(false);
  });

  it("gives a seat with no choice of its own the device-wide one kept before COI-3820", () => {
    const storage = stub();
    storage.setItem("aterm.side-open.v1", "open");
    expect(loadSideOpen("seat-a", storage)).toBe(true);
    storeSideOpen("seat-a", false, storage);
    expect(loadSideOpen("seat-a", storage), "its own choice wins").toBe(false);
    expect(loadSideOpen("seat-b", storage)).toBe(true);
  });

  it("keeps the newest 100 seats", () => {
    const storage = stub();
    for (let i = 0; i < 105; i++) storeSideOpen(`seat-${i}`, true, storage);
    expect(loadSideOpen("seat-0", storage), "the oldest is forgotten").toBe(false);
    expect(loadSideOpen("seat-104", storage)).toBe(true);
  });

  it("falls back to put away when storage is off or holds something else", () => {
    expect(loadSideOpen("seat-a", { getItem: () => { throw new Error("off"); } })).toBe(false);
    expect(loadSideOpen("seat-a", { getItem: () => "maybe" })).toBe(false);
  });
});
