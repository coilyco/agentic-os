import { describe, expect, it } from "vitest";
import { advanceIndex, askAnswer, choiceFromAsk, detectChoice, keysFor } from "./choices";

// Captured from Claude Code's workspace trust prompt in an aterm daemon session.
const trust = [
  "",
  "─────────────────────────────────────────────────────────────────",
  " Accessing workspace:",
  "",
  " /private/tmp/scratchpad/claude-fixture",
  "",
  " Quick safety check: Is this a project you created or one you trust? (Like your own code, a",
  " well-known open source project, or work from your team). If not, take a moment to review",
  " what's in this folder first.",
  "",
  " Claude Code'll be able to read, edit, and execute files here.",
  "",
  " Security guide",
  "",
  " ❯ No, exit",
  "   Yes, I trust this folder",
  "",
  " Enter to confirm · Esc to cancel",
  "",
  "",
];

// Captured from Claude Code v2.1.282 showing an AskUserQuestion menu.
const ask = [
  "❯ Use the AskUserQuestion tool once to ask me which accent color the aterm client should",
  "  use. Options: Purple (matches the brand mark), Teal (matches the sysadmin seats), Black",
  "─────────────────────────────────────────────────────────────────",
  " ☐ Accent",
  "",
  "Which accent color should the aterm client use?",
  "",
  "❯ 1. Purple",
  "     Matches the brand mark.",
  "  2. Teal",
  "     Matches the sysadmin seats.",
  "  3. Black",
  "     The quietest of the three.",
  "  4. Type something.",
  "─────────────────────────────────────────────────────────────────",
  "  5. Chat about this",
  "",
  "Enter to select · ↑/↓ to navigate · Esc to cancel",
  "",
];

describe("detectChoice", () => {
  it("reads Claude Code's AskUserQuestion menu", () => {
    const choice = detectChoice(ask);
    expect(choice?.header).toBe("Accent");
    expect(choice?.question).toBe("Which accent color should the aterm client use?");
    expect(choice?.options.map((option) => [option.label, option.description, option.freeText])).toEqual([
      ["Purple", "Matches the brand mark.", false],
      ["Teal", "Matches the sysadmin seats.", false],
      ["Black", "The quietest of the three.", false],
      ["Type something.", "", true],
      ["Chat about this", "", false],
    ]);
    expect(choice?.cursor).toBe(0);
  });

  it("reads Claude Code's trust prompt", () => {
    const choice = detectChoice(trust);
    expect(choice?.options.map((option) => option.label)).toEqual(["No, exit", "Yes, I trust this folder"]);
    expect(choice?.cursor).toBe(0);
    expect(choice?.cancellable).toBe(true);
    expect(choice?.question.split("\n\n")).toEqual([
      "Accessing workspace:",
      "/private/tmp/scratchpad/claude-fixture",
      "Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source project, or work from your team). If not, take a moment to review what's in this folder first.",
      "Claude Code'll be able to read, edit, and execute files here.",
      "Security guide",
    ]);
  });

  it("strips numbering and keeps option descriptions", () => {
    const choice = detectChoice([
      "Which color should the accent be?",
      "",
      "  1. Purple",
      "       Matches the brand mark.",
      "❯ 2. Teal",
      "       Matches the sysadmin seats.",
      "  3. Black",
      "",
      "Enter to select · ↑/↓ to navigate · Esc to cancel",
    ]);
    expect(choice?.options.map((option) => [option.label, option.description])).toEqual([
      ["Purple", "Matches the brand mark."],
      ["Teal", "Matches the sysadmin seats."],
      ["Black", ""],
    ]);
    expect(choice?.cursor).toBe(1);
    expect(choice?.question).toBe("Which color should the accent be?");
  });

  it("keeps a scrolled menu's marker out of the question", () => {
    const choice = detectChoice([" ☐ Accent", "", "Which accent?", "", "↑ 2. Teal", "❯ 3. Black", "  4. Type something.", "", "Enter to select · Esc to cancel"]);
    expect(choice?.question).toBe("Which accent?");
    expect(choice?.options.map((option) => option.label)).toEqual(["Black", "Type something."]);
  });

  it("ignores a shell prompt that uses the same glyph", () => {
    expect(detectChoice(["~/projects ❯ ls", "README.md", "~/projects ❯ "])).toBeNull();
  });

  it("needs a footer, so a stray list is not a menu", () => {
    expect(detectChoice(["❯ one", "  two"])).toBeNull();
  });
});

describe("keysFor", () => {
  it("moves down to the target and confirms", () => {
    const choice = detectChoice(trust)!;
    expect(keysFor(choice, 1)).toEqual(["\x1b[B", "\r"]);
    expect(keysFor(choice, 0)).toEqual(["\r"]);
  });

  it("types a free-text answer on its row before confirming", () => {
    const choice = detectChoice(ask)!;
    expect(keysFor(choice, 3, "Magenta")).toEqual(["\x1b[B\x1b[B\x1b[B", "Magenta", "\r"]);
  });
});

describe("choiceFromAsk", () => {
  it("adds a typed row for allow_other and keeps multi", () => {
    const choice = choiceFromAsk({
      id: "a", session: "s", header: "Sweep", question: "Which routes?",
      options: [{ label: "Qwen", description: "fast" }, { label: "Llama", description: "" }],
      allowOther: true, multi: true,
    });
    expect(choice.options.map((option) => [option.label, option.freeText])).toEqual([["Qwen", false], ["Llama", false], ["Type something.", true]]);
    expect(choice.multi).toBe(true);
    expect(choice.cancellable).toBe(true);
  });
});

describe("askAnswer", () => {
  const ask = {
    id: "a", session: "s", header: "", question: "Which?",
    options: [{ label: "Qwen", description: "" }, { label: "Llama", description: "" }, { label: "Hosted", description: "" }],
    allowOther: true, multi: true,
  };

  it("keeps the ask's own picks and sends Other as text only", () => {
    expect(askAnswer(ask, [0, 3], "Mistral")).toEqual({ picks: [0], text: "Mistral" });
  });

  it("sends a lone typed answer with no picks", () => {
    expect(askAnswer(ask, [3], "Mistral")).toEqual({ picks: [], text: "Mistral" });
  });

  it("drops text the ask did not allow", () => {
    expect(askAnswer({ ...ask, allowOther: false }, [1], "stray")).toEqual({ picks: [1] });
  });
});

describe("advanceIndex", () => {
  const option = (label: string, freeText = false) => ({ label, description: "", freeText });
  const menu = (labels: string[], multi = false) => ({ header: "", question: "", options: labels.map((label) => option(label)), cursor: 0, cancellable: true, multi });

  it("finds a harness's Next row wherever it sits", () => {
    expect(advanceIndex(menu(["[ ] Purple", "[ ] Teal", "Next", "Chat about this"]))).toBe(2);
    expect(advanceIndex(menu(["Submit answers", "Cancel"]))).toBe(0);
  });

  it("leaves ordinary options and multi cards alone", () => {
    expect(advanceIndex(menu(["Purple", "Teal"]))).toBe(-1);
    expect(advanceIndex(menu(["Next step", "Done later"]))).toBe(-1);
    expect(advanceIndex(menu(["Teal", "Next"], true))).toBe(-1);
  });
});
