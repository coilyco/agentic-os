import { describe, expect, it } from "vitest";
import { coastRows, takeRows } from "./touch-scroll";

describe("takeRows", () => {
  it("holds a drag shorter than a row for the next move", () => {
    const first = takeRows(0, 9, 20);
    expect(first).toEqual({ rows: 0, carry: 9 });
    expect(takeRows(first.carry, 12, 20)).toEqual({ rows: 1, carry: 1 });
  });
  it("counts rows up and down", () => {
    expect(takeRows(0, 65, 20).rows).toBe(3);
    expect(takeRows(0, -65, 20).rows).toBe(-3);
  });
  it("does nothing without a row height", () => expect(takeRows(5, 100, 0)).toEqual({ rows: 0, carry: 0 }));
});

describe("coastRows", () => {
  it("does not coast on a slow lift", () => expect(coastRows(0.2, 20)).toBe(0));
  it("coasts further on a faster flick, in the flick's direction", () => {
    expect(coastRows(1, 20)).toBeGreaterThan(5);
    expect(coastRows(2, 20)).toBeGreaterThan(coastRows(1, 20));
    expect(coastRows(-1, 20)).toBeLessThan(0);
  });
});
