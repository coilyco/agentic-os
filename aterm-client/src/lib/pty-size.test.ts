import { describe, expect, it } from "vitest";
import { boxToReport, nextTall, TALL_MAX, TALL_RELEASE_MS, TALL_STEP } from "./pty-size";

const own = { rows: 50, cols: 200 };

describe("boxToReport", () => {
  it("reports what the host measured", () => {
    expect(boxToReport({ rows: 28, cols: 47 }, true, own)).toEqual({ rows: 28, cols: 47 });
    expect(boxToReport({ rows: 28, cols: 47 }, false, own)).toEqual({ rows: 28, cols: 47 });
  });

  it("sends nothing when a panning host cannot be measured, so the PTY's size is never claimed as a box", () => {
    expect(boxToReport(undefined, true, own)).toBeNull();
  });

  it("keeps xterm's own size for a daemon that does not size the PTY", () => {
    expect(boxToReport(undefined, false, own)).toEqual(own);
  });
});

describe("nextTall", () => {
  const seen = { dialog: true, clipped: false, honored: true, idleMs: 0 };

  it("asks for more rows while options sit above the screen, one step at a time", () => {
    expect(nextTall(0, { ...seen, clipped: true })).toBe(TALL_STEP);
    expect(nextTall(TALL_STEP, { ...seen, clipped: true })).toBe(TALL_STEP * 2);
  });

  it("stops at the cap, so a dialog nobody can fit does not grow the PTY forever", () => {
    expect(nextTall(TALL_MAX, { ...seen, clipped: true })).toBe(TALL_MAX);
    expect(nextTall(TALL_MAX - 1, { ...seen, clipped: true })).toBe(TALL_MAX);
  });

  it("does not ask again while the last ask has not been given, so a capped PTY is not chased", () => {
    expect(nextTall(TALL_STEP, { ...seen, clipped: true, honored: false })).toBe(TALL_STEP);
  });

  it("holds the rows while the dialog shows whole, since fewer would clip it again", () => {
    expect(nextTall(TALL_STEP, seen)).toBe(TALL_STEP);
    expect(nextTall(TALL_STEP, { ...seen, idleMs: TALL_RELEASE_MS * 10 })).toBe(TALL_STEP);
  });

  it("lets the rows go once no dialog has been on the screen for a while, and not before", () => {
    const gone = { dialog: false, clipped: false, honored: true };
    expect(nextTall(TALL_STEP, { ...gone, idleMs: TALL_RELEASE_MS - 1 })).toBe(TALL_STEP);
    expect(nextTall(TALL_STEP, { ...gone, idleMs: TALL_RELEASE_MS })).toBe(0);
    expect(nextTall(0, { ...gone, idleMs: TALL_RELEASE_MS })).toBe(0);
  });
});
