import { describe, expect, it } from "vitest";
import { isBlank, screenRows, tabBarReading } from "./screen";

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

describe("tabBarReading", () => {
  const BAR = "←  ☐ Accent  ☒ Routes  ✔ Submit  →";
  const withCells = (text: string, marked: number[]) => ({
    length: text.length,
    isWrapped: false,
    translateToString: () => text,
    getCell: (x: number) => ({ isBgDefault: () => !marked.includes(x) }),
  });
  const source = (rows: Record<number, ReturnType<typeof withCells>>, baseY = 0) => ({ baseY, getLine: (row: number) => rows[row] });

  it("returns the tab bar's text and the columns whose cells have a background", () => {
    const from = BAR.indexOf("Routes") - 3;
    const reading = tabBarReading(source({ 0: withCells("❯ Use the tool", []), 2: withCells(BAR, [from, from + 1, from + 2]) }), 6);
    expect(reading).toEqual({ text: BAR, highlighted: [from, from + 1, from + 2] });
  });

  it("reads only the live screen, below the scrollback", () => {
    expect(tabBarReading(source({ 0: withCells(BAR, [5]) }, 3), 4)).toBeNull();
  });

  it("is null when no row is a tab bar", () => {
    expect(tabBarReading(source({ 0: withCells("Which accent?", []) }), 3)).toBeNull();
  });
});

describe("tabBarReading on a narrow terminal", () => {
  const BAR = "←  ☐ Accent  ☐ Routes  ☐ Name  ☐ Other  ✔ Submit  →";
  const COLS = 34;
  // xterm hard-wraps a line wider than the terminal and marks the second row as wrapped.
  const rowOf = (text: string, wrapped: boolean, marked: number[]) => ({
    length: COLS,
    isWrapped: wrapped,
    translateToString: (trim: boolean) => (trim ? text.trimEnd() : text.padEnd(COLS)),
    getCell: (x: number) => ({ isBgDefault: () => !marked.includes(x) }),
  });

  it("joins a wrapped tab bar into one line, with columns counted across the rows", () => {
    const mark = BAR.indexOf("✔");
    expect(mark).toBeGreaterThan(COLS);
    const buffer = {
      baseY: 0,
      getLine: (row: number) => (row === 1 ? rowOf(BAR.slice(0, COLS), false, []) : row === 2 ? rowOf(BAR.slice(COLS), true, [mark - COLS]) : undefined),
    };
    expect(tabBarReading(buffer, 4)).toEqual({ text: BAR, highlighted: [mark] });
  });
});
