import { describe, expect, it } from "vitest";
import { isBlank, screenRows } from "./screen";

const buffer = (lines: string[], baseY: number) => ({
  baseY,
  getLine: (row: number) => (lines[row] === undefined ? undefined : { translateToString: () => lines[row]! }),
});

describe("screenRows", () => {
  it("reads the live screen below the scrollback", () => {
    const lines = ["old 1", "old 2", "old 3", "❯ 1. Yes", "  2. No", "Enter to select"];
    expect(screenRows(buffer(lines, 3), 3)).toEqual(["❯ 1. Yes", "  2. No", "Enter to select"]);
  });

  it("pads rows the buffer has not filled", () => {
    expect(screenRows(buffer(["one"], 0), 3)).toEqual(["one", "", ""]);
  });
});

describe("isBlank", () => {
  it("is blank until a row holds text, so a starting seat's empty screen can be told from a drawn one", () => {
    expect(isBlank(["", "  ", ""])).toBe(true);
    expect(isBlank(["", "Do you trust the files in this folder?"])).toBe(false);
  });
});
