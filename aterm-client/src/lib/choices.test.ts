import { describe, expect, it } from "vitest";
import { advanceIndex, askAnswer, choiceFromAsk, choiceKey, detectChoice, keysFor, keysForPicks, nextRowIndex } from "./choices";

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

// Captured from Claude Code v2.1.318: a three-question dialog at 72 columns.
const RULE = "────────────────────────────────────────────────────────────────────────";
const tabs = "←  ☐ Accent  ☒ Routes  ☐ Name  ✔ Submit  →";
const multi = (cursorRow: "first" | "next", box = "[ ]") => [
  RULE,
  tabs,
  "",
  "Which routes do you want enabled?",
  "",
  `${cursorRow === "first" ? "❯" : " "} 1. ${box} Qwen`,
  "         Route requests to the Qwen model family.",
  "  2. [ ] Llama",
  "         Route requests to the Llama model family.",
  "  3. [ ] Frontier",
  "         Route requests to frontier Claude models.",
  "  4. [ ] Type something",
  cursorRow === "next" ? "❯    Next" : "     Next",
  RULE,
  "  5. Chat about this",
  "",
  "Enter to select · Tab/Arrow keys to navigate · Esc to cancel",
];
const review = [
  RULE,
  "←  ☒ Accent  ☒ Routes  ☒ Name  ✔ Submit  →",
  "",
  "Review your answers",
  "",
  " ● Which accent color do you want?",
  "   → Teal",
  " ● Which routes do you want enabled?",
  "   → Qwen",
  "",
  "Ready to submit your answers?",
  "",
  "❯ 1. Submit answers",
  "  2. Cancel",
];

describe("a multi-question dialog", () => {
  it("keeps the tab bar out of the question", () => {
    const choice = detectChoice(multi("first"));
    expect(choice?.question).toBe("Which routes do you want enabled?");
    expect(choice?.header).toBe("");
  });

  it("reads Next as a row of its own, not as the description of the row above", () => {
    const choice = detectChoice(multi("first"))!;
    expect(choice.options.map((option) => [option.label, option.description])).toEqual([
      ["Qwen", "Route requests to the Qwen model family."],
      ["Llama", "Route requests to the Llama model family."],
      ["Frontier", "Route requests to frontier Claude models."],
      ["Type something", ""],
      ["Next", ""],
      ["Chat about this", ""],
    ]);
    expect(nextRowIndex(choice)).toBe(4);
  });

  it("reads the checkboxes, so a multi-select is one", () => {
    const choice = detectChoice(multi("first", "[✔]"))!;
    expect(choice.multi).toBe(true);
    expect(choice.options.map((option) => option.checked)).toEqual([true, false, false, false, undefined, undefined]);
  });

  it("treats a checkbox Type something row as the typed one", () => {
    const choice = detectChoice(multi("first"))!;
    expect(choice.options.map((option) => option.freeText)).toEqual([false, false, false, true, false, false]);
  });

  it("still sees the menu while the cursor is on Next", () => {
    const choice = detectChoice(multi("next"))!;
    expect(choice.cursor).toBe(4);
    expect(choice.options).toHaveLength(6);
  });

  it("reads the review screen, which has no footer", () => {
    const choice = detectChoice(review)!;
    expect(choice.options.map((option) => option.label)).toEqual(["Submit answers", "Cancel"]);
    expect(choice.question).toContain("Ready to submit your answers?");
    expect(advanceIndex(choice)).toBe(0);
  });
});

describe("keysForPicks", () => {
  const frame = () => detectChoice(multi("first", "[✔]"))!;

  it("toggles only what differs, then moves on to Next", () => {
    // Qwen stays checked, Llama is added, then the cursor walks down to Next.
    expect(keysFor_(frame(), [0, 1])).toEqual(["\x1b[B", "\r", "\x1b[B\x1b[B\x1b[B", "\r"]);
  });

  it("unchecks a row the person put down", () => {
    expect(keysFor_(frame(), [])).toEqual(["\r", "\x1b[B\x1b[B\x1b[B\x1b[B", "\r"]);
  });

  it("types into the Type something row and leaves it, since Enter would uncheck it", () => {
    expect(keysFor_(frame(), [0, 3], "Mistral")).toEqual(["\x1b[B\x1b[B\x1b[B", "Mistral", "\x1b[B", "\r"]);
  });
});

function keysFor_(choice: Parameters<typeof keysForPicks>[0], picks: number[], text = ""): string[] {
  return keysForPicks(choice, picks, text);
}

describe("a narrow terminal", () => {
  // Captured from Claude Code v2.1.318 at 44 columns: the footer wraps.
  const narrow = [
    "────────────────────────────────────────────",
    "←  ☒ Accent  ☐ Routes  ☐ Name  ✔ Submit  →",
    "",
    "Which routes do you want enabled?",
    "",
    "❯ 1. [ ] Qwen",
    "         Qwen model family",
    "  2. [ ] Llama",
    "         Llama model family",
    "  3. [ ] Type something",
    "     Next",
    "────────────────────────────────────────────",
    "  4. Chat about this",
    "",
    "Enter to select · Tab/Arrow keys to navigate",
    "· Esc to cancel",
  ];

  it("reads Esc to cancel from the wrapped footer, so Cancel stays", () => {
    expect(detectChoice(narrow)?.cancellable).toBe(true);
  });
});

describe("the review screen's summary", () => {
  it("keeps each answer on its own line", () => {
    const choice = detectChoice(review)!;
    expect(choice.question).toContain("● Which accent color do you want?\n→ Teal\n● Which routes do you want enabled?\n→ Qwen");
  });
});

describe("a very narrow terminal", () => {
  // Captured from v2.1.318 at 34 columns, cursor on Submit: a four-row footer.
  const veryNarrow = [
    "──────────────────────────────────",
    "←  ☒ Accent  ☐ Routes  ✔ Submit  →",
    "",
    "Which routes do you want enabled?",
    "",
    "  1. [ ] Qwen",
    "         Routes requests to the",
    "         Qwen model family, which",
    "         is strong at multilingual",
    "         and coding tasks.",
    "  2. [ ] Type something",
    "❯    Submit",
    "──────────────────────────────────",
    "  3. Chat about this",
    "",
    "Enter to select · Tab/Arrow keys",
    "to navigate · ctrl+g to edit in VS",
    "Code · Esc to cancel",
  ];

  it("still reads Esc to cancel, and the wrapped descriptions stay with their rows", () => {
    const choice = detectChoice(veryNarrow)!;
    expect(choice.cancellable).toBe(true);
    expect(choice.cursor).toBe(2);
    expect(choice.options[0]?.description).toBe("Routes requests to the Qwen model family, which is strong at multilingual and coding tasks.");
  });
});

describe("choiceKey", () => {
  const menu = (question: string, labels = ["Purple", "Teal"]) => ({
    header: "Accent",
    question,
    options: labels.map((label) => ({ label, description: "", freeText: false })),
    cursor: 0,
    cancellable: true,
    multi: false,
  });

  it("is the same while only a counter in the dialog's text ticks", () => {
    expect(choiceKey(menu("Which accent? (continuing in 59s)"))).toBe(choiceKey(menu("Which accent? (continuing in 58s)")));
  });

  it("changes for a different question, and for different options", () => {
    expect(choiceKey(menu("Which accent?"))).not.toBe(choiceKey(menu("Which route?")));
    expect(choiceKey(menu("Which accent?"))).not.toBe(choiceKey(menu("Which accent?", ["Purple", "Gold"])));
  });

  it("ignores where the cursor is", () => {
    expect(choiceKey({ ...menu("Which accent?"), cursor: 1 })).toBe(choiceKey(menu("Which accent?")));
  });
});
