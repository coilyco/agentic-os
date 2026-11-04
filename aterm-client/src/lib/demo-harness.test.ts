import { describe, expect, it } from "vitest";
import { detectChoice } from "./choices";
import { harnessInputRows } from "./harness";
import { CLAUDE_BOX, CLAUDE_DIALOG, drawHarness, type Frame } from "./demo-harness";

// The drive's test bed, so its screens must stay what the detectors read.
const screen = (frame: Frame, rows = 0, cols = 70) =>
  drawHarness(["› check the hosts page"], frame, rows, cols).replace("\x1b[H\x1b[2J", "").replace(/\x1b\[[0-9;]*m/g, "").split("\r\n");

describe("demo harness screens", () => {
  it("draws a Claude Code box the crop hides, with no menu in it", () => {
    expect(harnessInputRows(screen(CLAUDE_BOX))).toBe(5);
    expect(detectChoice(screen(CLAUDE_BOX))).toBeNull();
  });

  it("draws a permission menu the crop leaves showing and a card can read", () => {
    expect(harnessInputRows(screen(CLAUDE_DIALOG))).toBe(0);
    expect(detectChoice(screen(CLAUDE_DIALOG))?.options.map((option) => option.label)).toEqual(["Yes", "Yes, and don't ask again this session", "No, and tell Claude what to do differently"]);
  });

  it("pins the frame to the bottom row of a taller PTY, as a harness repaints on resize", () => {
    const tall = screen(CLAUDE_BOX, 40);
    expect(tall).toHaveLength(40);
    expect(harnessInputRows(tall)).toBe(5);
  });

  it("draws its rules to a phone's width, so no rule wraps into a second row", () => {
    const narrow = screen(CLAUDE_BOX, 30, 35);
    expect(harnessInputRows(narrow)).toBe(5);
    expect(narrow.every((row) => row.length <= 35)).toBe(true);
  });
});
