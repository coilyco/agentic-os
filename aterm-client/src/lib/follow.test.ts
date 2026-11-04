import { describe, expect, it } from "vitest";
import { AT_END, jumpLabel, linesAbove, scrolled, wrote } from "./follow";

describe("linesAbove", () => {
  it("is zero at the end and counts lines below the top of the view", () => {
    expect(linesAbove({ baseY: 500, viewportY: 500 })).toBe(0);
    expect(linesAbove({ baseY: 500, viewportY: 120 })).toBe(380);
  });

  it("never goes negative", () => {
    expect(linesAbove({ baseY: 0, viewportY: 3 })).toBe(0);
  });
});

describe("scrolled", () => {
  it("is away, with nothing new, the moment the view leaves the end", () => {
    expect(scrolled(AT_END, 12)).toEqual({ away: true, unseen: 0 });
  });

  it("keeps the count while the view stays above the end", () => {
    expect(scrolled({ away: true, unseen: 7 }, 30)).toEqual({ away: true, unseen: 7 });
  });

  it("starts over on returning to the end", () => {
    expect(scrolled({ away: true, unseen: 7 }, 0)).toEqual(AT_END);
  });
});

describe("wrote", () => {
  it("counts lines pushed past a view that is above the end", () => {
    expect(wrote({ away: true, unseen: 2 }, 40, 43)).toEqual({ away: true, unseen: 5 });
  });

  it("counts them when the scrollback is full too, since the view's top moves up instead", () => {
    // baseY holds at the cap and viewportY falls by one per line, so the gap still grows.
    expect(wrote({ away: true, unseen: 0 }, 900, 905)).toEqual({ away: true, unseen: 5 });
  });

  it("ignores output while the view is at the end, and a redraw in place", () => {
    expect(wrote(AT_END, 0, 0)).toEqual(AT_END);
    expect(wrote({ away: true, unseen: 4 }, 40, 40)).toEqual({ away: true, unseen: 4 });
  });
});

describe("jumpLabel", () => {
  it("says only where it goes when nothing is new", () => {
    expect(jumpLabel(0)).toEqual({ count: "", spoken: "Jump to latest output" });
  });

  it("counts the new lines, for the eye and for a screen reader", () => {
    expect(jumpLabel(1)).toEqual({ count: "1", spoken: "Jump to latest output, 1 new line" });
    expect(jumpLabel(12)).toEqual({ count: "12", spoken: "Jump to latest output, 12 new lines" });
  });

  it("caps a long count", () => {
    expect(jumpLabel(4200).count).toBe("999+");
  });
});
