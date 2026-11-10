import { describe, expect, it } from "vitest";
import { findColours, hexOfComputed, mixHex } from "./brand";

describe("mixHex", () => {
  it("returns the base at 0 and the top at 1", () => {
    expect(mixHex("#66dd8a", "#0b0c10", 0)).toBe("#0b0c10");
    expect(mixHex("#66dd8a", "#0b0c10", 1)).toBe("#66dd8a");
  });
  it("blends in between", () => expect(mixHex("#ffffff", "#000000", 0.5)).toBe("#808080"));
  it("keeps the base when a colour is not hex", () => expect(mixHex("green", "#0b0c10", 0.5)).toBe("#0b0c10"));
});

describe("findColours", () => {
  it("builds every fill from the accent over the ground, and the border is the accent", () => {
    const colours = findColours("#66dd8a", "#0b0c10");
    expect(colours.activeMatchBorder).toBe("#66dd8a");
    expect(colours.matchBackground).toMatch(/^#[0-9a-f]{6}$/);
    expect(colours.matchBackground).not.toBe(colours.activeMatchBackground);
  });
});

describe("hexOfComputed", () => {
  it("reads a plain colour", () => expect(hexOfComputed("rgb(16, 18, 22)")).toBe("#101216"));
  it("reads a colour with alpha by its channels", () => expect(hexOfComputed("rgba(16, 18, 22, 0.5)")).toBe("#101216"));
  it("reads a derived colour, which computes in the 0 to 1 range", () => expect(hexOfComputed("color(srgb 0.0627451 0.0705882 0.0862745)")).toBe("#101216"));
  it("gives nothing for a colour it cannot read", () => expect(hexOfComputed("")).toBeUndefined());
});
