import { describe, expect, it } from "vitest";
import { harnessInputRows } from "./harness";

const rule = "─".repeat(40);
const text = (n: number) => Array.from({ length: n }, (_, i) => `session text ${i + 1}`);

describe("harnessInputRows", () => {
  it("finds Claude Code's ruled input box and the status lines under it", () => {
    const screen = [...text(10), rule, "❯ ", rule, "  ⏵⏵ accept edits on (shift+tab to cycle)", "  ? for shortcuts"];
    expect(harnessInputRows(screen)).toBe(5);
  });

  it("finds the box as Claude Code draws it today: a labelled rule, a blank row, then one status line", () => {
    const label = "─".repeat(60) + " seat ─";
    const screen = ["✢ Recombining… (58m 6s · ↓ 233.4k tokens)", "  ⎿  Tip: Use /clear to start fresh", "     622218 tokens", label, "❯ ", "─".repeat(70), "", "  ⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent"];
    expect(harnessInputRows(screen)).toBe(5);
  });

  it("finds a rounded box with the prompt inside it", () => {
    const screen = [...text(10), "╭" + "─".repeat(38) + "╮", "│ > half a thought      │", "╰" + "─".repeat(38) + "╯", "  ? for shortcuts"];
    expect(harnessInputRows(screen)).toBe(4);
  });

  it("counts blank rows under a box that is not at the foot of the screen", () => {
    const screen = [...text(5), rule, "❯ ", rule, "", ""];
    expect(harnessInputRows(screen)).toBe(5);
  });

  it("shows everything when the bottom is a permission dialog, since a seat is waiting on it", () => {
    const screen = [...text(6), rule, " Do you want to proceed?", " ❯ 1. Yes", "   2. No, and tell Claude what to do differently", rule];
    expect(harnessInputRows(screen)).toBe(0);
  });

  it("shows everything when a select menu is on screen", () => {
    const screen = [...text(4), rule, "❯ 1. First", "  2. Second", rule, "  Enter to select · Esc to cancel"];
    expect(harnessInputRows(screen)).toBe(0);
  });

  it("shows everything for a plain shell prompt, and for plain output", () => {
    expect(harnessInputRows([...text(10), "❯ "])).toBe(0);
    expect(harnessInputRows(text(10))).toBe(0);
    expect(harnessInputRows([])).toBe(0);
  });

  it("shows everything when a rule is not followed by a single prompt row", () => {
    expect(harnessInputRows([...text(5), rule, "some output", rule])).toBe(0);
    expect(harnessInputRows([...text(5), rule, "❯ ", "more output", rule])).toBe(0);
  });

  it("does not take a long tail of other output for a status line", () => {
    const screen = [...text(5), rule, "❯ ", rule, "a", "b", "c", "d"];
    expect(harnessInputRows(screen)).toBe(0);
  });
});
