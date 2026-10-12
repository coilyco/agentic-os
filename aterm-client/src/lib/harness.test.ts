import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { harnessInputRows } from "./harness";

/** A screen read off a live seat with aterm status, trailing blanks trimmed. */
const captured = (name: string): string[] => readFileSync(new URL(`./fixtures/${name}.txt`, import.meta.url), "utf8").split("\n");

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

  it("finds opencode's left-rail box, with its hint row and the path row under it", () => {
    // A real opencode 1.18 start screen. Its box has no rules.
    const screen = [
      "                               █▀▀█ █▀▀█ █▀▀█ █▀▀▄ █▀▀▀ █▀▀█ █▀▀█ █▀▀█",
      "",
      "             ┃",
      '             ┃  Ask anything… "Fix broken tests"',
      "             ┃",
      "             ┃  Build · Claude Sonnet 4.6 (US) Amazon Bedrock",
      "             ╹" + "▀".repeat(60),
      "                                                             tab agents  ctrl+p commands",
      "",
      "  /path/to/project:main                                                         1.18.35",
    ];
    expect(harnessInputRows(screen)).toBe(8);
  });

  it("finds opencode's box from a real screen captured from a seat, idle and at work", () => {
    expect(harnessInputRows(captured("opencode-home"))).toBe(7);
    // The Update Complete dialog above the box stays on screen. Only the box goes.
    const working = captured("opencode-working");
    expect(harnessInputRows(working)).toBe(7);
    expect(working.slice(0, working.length - 7).some((row) => row.includes("Update Complete"))).toBe(true);
  });

  it("finds codex's prompt row, its status row and its hint row, from a real seat", () => {
    expect(harnessInputRows(captured("codex-working"))).toBe(5);
  });

  it("finds goose's gauge and send row, from the screen pinned in aterm/testdata/goose-startup.json", () => {
    const screen = [
      "    __( O)>  ● new session · ollama qwen3.8:27b",
      "   \\____)    20261008_4 · /private/tmp/aterm-goose-fixture",
      "     L L     goose is ready",
      "  ⏳ loading extensions in background...",
      "  ╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌ 0% 0/128k",
      "",
      "> Enter to send · Ctrl+J newline",
    ];
    expect(harnessInputRows(screen)).toBe(3);
    // A line already typed, or a shell prompt, is not goose's empty box.
    expect(harnessInputRows([...screen.slice(0, 5), "", "> half a thought"])).toBe(0);
    expect(harnessInputRows([...text(4), "> "])).toBe(0);
  });

  it("shows everything when a rail dialog or a codex prompt is not the plain box", () => {
    // Constructed from the harnesses' approval words, not captured. See architecture.md.
    const rail = ["  ┃  Permission required", "  ┃  Allow once   Allow always   Reject", "  ╹" + "▀".repeat(30), "  enter confirm  esc dismiss"];
    expect(harnessInputRows([...text(5), ...rail])).toBe(0);
    const model = ["  ┃  Allow bash: ls /etc/hosts", "  ┃  Build · Some Model", "  ╹" + "▀".repeat(30), "  tab agents  ctrl+p commands"];
    expect(harnessInputRows([...text(5), ...model])).toBe(0);
    const codex = ["  Would you like to run the following command?", "", "› 1. Yes, proceed (y)", "  2. No, and tell Codex what to do differently (esc)", "", "  Press enter to confirm or esc to cancel"];
    expect(harnessInputRows([...text(5), ...codex])).toBe(0);
    expect(harnessInputRows([...text(5), "› quoted earlier message", "", "  other output"])).toBe(0);
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
