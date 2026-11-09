import { describe, expect, it } from "vitest";
import { findColours, mixHex } from "./brand";

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
