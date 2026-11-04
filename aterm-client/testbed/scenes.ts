// What a seat's screen shows, drawn at the size asked for, as a harness redraws on
// resize. Dialogs follow real v2.1.318 captures. See the testbed reference.
import { readFileSync } from "node:fs";
import type { SessionSpec } from "./mock-daemon.ts";

export interface Scene {
  render(cols: number, rows: number): string[];
  /** Typed bytes from the client. True when the screen changed and must be redrawn. */
  input?(data: string): boolean;
}

/** Word-wrap to `width`, later rows indented by `hang`. Leading spaces stay. */
export function wrap(text: string, width: number, hang = 0): string[] {
  const lead = text.length - text.trimStart().length;
  const out: string[] = [];
  let line = " ".repeat(lead);
  let empty = true;
  const room = () => (out.length === 0 ? width : width - hang);
  for (const word of text.trimStart().split(" ")) {
    if (!empty && line.length + 1 + word.length > room()) {
      out.push(line);
      line = "";
      empty = true;
    }
    // A token longer than the row breaks mid-word, as a terminal does.
    let rest = word;
    while (line.length + (empty ? 0 : 1) + rest.length > room() && rest.length > room() - line.length) {
      if (!empty) {
        out.push(line);
        line = "";
        empty = true;
        continue;
      }
      out.push(rest.slice(0, room()));
      rest = rest.slice(room());
    }
    line = empty ? line + rest : `${line} ${rest}`;
    empty = false;
  }
  out.push(line);
  return out.map((row, index) => (index === 0 ? row : " ".repeat(hang) + row));
}

const rule = (cols: number, label = ""): string => (label ? "─".repeat(Math.max(3, cols - label.length - 3)) + ` ${label} ─` : "─".repeat(cols));

/** The last `rows` rows, since a full screen scrolls its top away. */
const bottom = (lines: string[], rows: number): string[] => lines.slice(Math.max(0, lines.length - rows));

/** Pad above so the screen is `rows` tall and the harness box sits on the last row. */
const anchored = (lines: string[], rows: number): string[] => [...Array(Math.max(0, rows - lines.length)).fill(""), ...bottom(lines, rows)];

// What a working seat has said. Long paths and a long command are here on purpose:
// a phone wraps them, and wrapping is what the journeys look at.
const TRANSCRIPT: string[] = [
  "❯ Fix the Next button on the multi-question dialog, then run the gate.",
  "",
  "⏺ I'll start by reading how the card finds its menu.",
  "",
  "⏺ Read(aterm-client/src/lib/choices.ts)",
  "  ⎿  Read 214 lines",
  "",
  "⏺ Read(aterm-client/src/components/ChoiceCard.svelte)",
  "  ⎿  Read 180 lines",
  "",
  "⏺ The card moves a menu's own Next row into its footer, but only when the row sits below the options at the same indent. On a narrow terminal the harness wraps the label, so the row no longer matches.",
  "",
  "⏺ Bash(just aterm-client-gate)",
  "  ⎿  svelte-check found 0 errors and 0 warnings",
  "     Test Files  58 passed (58)",
  "          Tests  412 passed (412)",
  "     vite v8.3.0 building for production... ✓ built in 2.41s",
  "",
  "⏺ The gate is green. Changes:",
  "  - choices.ts: read a wrapped Next label as one row",
  "  - ChoiceCard.svelte: keep the advance button in the footer",
  "  - docs: choices.md notes the wrapped row",
];

export function claudeSession(options: { working: boolean; transcript?: string[]; label?: string; fresh?: boolean }): Scene {
  const { working, label = "seat" } = options;
  const transcript = options.transcript ?? TRANSCRIPT;
  return {
    render(cols, rows) {
      const body: string[] = [];
      for (const paragraph of transcript) body.push(...(paragraph === "" ? [""] : wrap(paragraph, cols, paragraph.startsWith("⏺") ? 2 : paragraph.startsWith("  ") ? 5 : 0)));
      if (working) body.push("", `✻ Churning… (esc to interrupt)`);
      const box = [rule(cols, label), "❯ ", rule(cols), "", "  ⏵⏵ accept edits on (shift+tab to cycle)"];
      const above = bottom(body, Math.max(0, rows - box.length));
      // A short conversation leaves blank rows under the box, as a fresh screen does.
      return options.fresh ? [...above, ...box] : anchored([...above, ...box], rows);
    },
  };
}

export interface Question {
  header: string;
  question: string;
  options: { label: string; description?: string }[];
  multi?: boolean;
}

type Phase = "asking" | "review" | "done" | "chat" | "cancelled";
type Row = { kind: "option" | "type" | "next"; index: number };

/** The prompt Claude Code keeps on screen above a dialog. */
const PROMPT = "❯ Use the AskUserQuestion tool exactly once and nothing else. After the answers, reply only done.";

/** Claude Code's AskUserQuestion dialog, laid out as the 2.1.318 captures show it. */
export class ClaudeAsk implements Scene {
  phase: Phase = "asking";
  step = 0;
  cursor = 0;
  private picked: Set<number>[];
  private typed: string[];
  /** What each question answered with, in the words Claude would read back. */
  answers: string[];

  constructor(private questions: Question[]) {
    this.picked = questions.map(() => new Set());
    this.typed = questions.map(() => "");
    this.answers = questions.map(() => "");
  }

  private get q(): Question {
    return this.questions[this.step]!;
  }

  private get last(): boolean {
    return this.step === this.questions.length - 1;
  }

  /** Rows of the menu: options, Type something, then Next (Submit last) if multi. */
  private get menu(): Row[] {
    const rows: Row[] = this.q.options.map((_, index) => ({ kind: "option", index }));
    rows.push({ kind: "type", index: this.q.options.length });
    if (this.q.multi) rows.push({ kind: "next", index: this.q.options.length + 1 });
    return rows;
  }

  private tabBar(): string {
    const tabs = this.questions.map((each, index) => `${this.answers[index] || index < this.step ? "☒" : "☐"} ${each.header}`);
    return `←  ${tabs.join("  ")}  ✔ Submit  →`;
  }

  render(cols: number, rows: number): string[] {
    if (this.phase === "done" || this.phase === "chat" || this.phase === "cancelled") return this.after(cols, rows);
    const above = ["", ...wrap(PROMPT, cols, 2)];
    const out: string[] = [rule(cols), this.questions.length > 1 ? this.tabBar() : ` ☐ ${this.q.header}`, ""];
    if (this.phase === "review") {
      out.push("Review your answers", "");
      this.questions.forEach((each, index) => out.push(...wrap(` ● ${each.question}`, cols, 3), ...wrap(`   → ${this.answers[index]}`, cols, 5)));
      out.push("", "Ready to submit your answers?", "", `${this.cursor === 0 ? "❯" : " "} 1. Submit answers`, `${this.cursor === 1 ? "❯" : " "} 2. Cancel`);
      return anchored([...above, ...out], rows);
    }
    out.push(...wrap(this.q.question, cols), "");
    const multi = this.q.multi === true;
    // A multi-select's checkbox pushes a description in with it.
    const body = multi ? 9 : 5;
    this.menu.forEach((row, at) => {
      const pointer = at === this.cursor ? "❯ " : "  ";
      if (row.kind === "option") {
        const box = multi ? (this.picked[this.step]!.has(row.index) ? "[✔] " : "[ ] ") : "";
        out.push(...wrap(`${pointer}${row.index + 1}. ${box}${this.q.options[row.index]!.label}`, cols, 5));
        const description = this.q.options[row.index]!.description;
        if (description) out.push(...wrap(`${" ".repeat(body)}${description}`, cols, body));
      } else if (row.kind === "type") {
        const text = this.typed[this.step]!;
        const box = multi ? (this.picked[this.step]!.has(row.index) ? "[✔] " : "[ ] ") : "";
        out.push(...wrap(`${pointer}${row.index + 1}. ${box}${text || (multi ? "Type something" : "Type something.")}`, cols, 5));
      } else {
        out.push(`${pointer}   ${this.last ? "Submit" : "Next"}`);
      }
    });
    const hint = this.questions.length > 1 ? "Enter to select · Tab/Arrow keys to navigate" : "Enter to select · ↑/↓ to navigate";
    const editor = multi && this.menu[this.cursor] && this.menu[this.cursor]!.kind !== "option" ? " · ctrl+g to edit in VS Code" : "";
    out.push(rule(cols), `  ${this.menu.length + 1}. Chat about this`, "", ...wrap(`${hint}${editor} · Esc to cancel`, cols, 0));
    return anchored([...above, ...out], rows);
  }

  private after(cols: number, rows: number): string[] {
    const said =
      this.phase === "chat"
        ? ["⏺ You chose to chat about this instead."]
        : this.phase === "cancelled"
          ? ["⏺ The question was dismissed."]
          : ["⏺ User answered:", ...this.questions.map((each, index) => `  ⎿  ${each.question} → ${this.answers[index]}`)];
    return claudeSession({ working: true, transcript: [PROMPT, "", ...said] }).render(cols, rows);
  }

  input(data: string): boolean {
    if (this.phase === "done" || this.phase === "chat" || this.phase === "cancelled") return false;
    let changed = false;
    for (const key of data.match(/\x1b\[[ABCD]|\x1b|\r|\n|\x7f|\t|[^]/gu) ?? []) changed = this.key(key) || changed;
    return changed;
  }

  private key(key: string): boolean {
    if (key === "\x1b") {
      this.phase = "cancelled";
      return true;
    }
    if (this.phase === "review") {
      if (key === "\x1b[A") this.cursor = 0;
      else if (key === "\x1b[B") this.cursor = 1;
      else if (key === "\r") this.phase = this.cursor === 0 ? "done" : "cancelled";
      else return false;
      return true;
    }
    const menu = this.menu;
    const total = menu.length + 1;
    const here = this.cursor < menu.length ? menu[this.cursor]! : undefined;
    if (key === "\x1b[A") this.cursor = (this.cursor + total - 1) % total;
    else if (key === "\x1b[B") this.cursor = (this.cursor + 1) % total;
    else if (key === " " && here?.kind === "option" && this.q.multi) this.toggle(here.index);
    else if (key === "\r") this.choose(here);
    // Typed text lands only on the Type something row. In a multi-select it checks it.
    else if (here?.kind === "type" && key === "\x7f") this.typed[this.step] = this.typed[this.step]!.slice(0, -1);
    else if (here?.kind === "type" && key >= " " && key.length === 1) {
      this.typed[this.step] += key;
      if (this.q.multi) this.picked[this.step]!.add(here.index);
    } else return false;
    return true;
  }

  private toggle(index: number): void {
    const set = this.picked[this.step]!;
    if (!set.delete(index)) set.add(index);
  }

  private choose(here: Row | undefined): void {
    if (!here) {
      this.phase = "chat";
      return;
    }
    if (here.kind === "option" && !this.q.multi) this.answers[this.step] = this.q.options[here.index]!.label;
    else if (here.kind === "option") {
      this.toggle(here.index);
      return;
    } else if (here.kind === "type" && this.q.multi) {
      // Enter on the typed row of a multi-select flips its box, as captured.
      this.toggle(here.index);
      return;
    } else if (here.kind === "type") {
      if (!this.typed[this.step]) return;
      this.answers[this.step] = this.typed[this.step]!;
    } else {
      const labels = [...this.picked[this.step]!].sort().map((index) => (index < this.q.options.length ? this.q.options[index]!.label : this.typed[this.step]!));
      this.answers[this.step] = labels.join(", ");
    }
    this.advance();
  }

  private advance(): void {
    this.cursor = 0;
    if (this.step + 1 < this.questions.length) this.step += 1;
    else if (this.questions.length > 1) this.phase = "review";
    else this.phase = "done";
  }
}

/** The single question of the 72-20 capture: four presets, Type something, Chat. */
export const ACCENT: Question = {
  header: "Accent",
  question: "Which accent color do you want?",
  options: [
    { label: "Purple", description: "Deep violet tone with a bold, creative feel." },
    { label: "Teal", description: "Cool blue-green tone that reads calm and modern." },
    { label: "Black", description: "High-contrast monochrome, minimal and sharp." },
    { label: "Gold", description: "Warm metallic yellow for a premium look." },
  ],
};

/** The three questions of fixtures/captures/*72-01-q1: single, multi-select, single. */
export const MULTI: Question[] = [
  ACCENT,
  {
    header: "Routes",
    question: "Which routes do you want enabled?",
    multi: true,
    options: [
      { label: "Qwen", description: "Route requests to the Qwen model family." },
      { label: "Llama", description: "Route requests to the Llama model family." },
      { label: "Frontier", description: "Route requests to frontier Claude models." },
    ],
  },
  {
    header: "Name",
    question: "Which name do you want?",
    options: [
      { label: "Fern", description: "Soft, green, and natural-sounding." },
      { label: "Moss", description: "Short and earthy with a grounded tone." },
      { label: "Pine", description: "Crisp and evergreen, easy to remember." },
      { label: "Birch", description: "Light and clean with a simple sound." },
    ],
  },
];

/** A real capture, replayed as it was taken at its own width. It does not reflow. */
export function captured(name: string): Scene {
  const text = readFileSync(new URL(`./fixtures/captures/${name}.txt`, import.meta.url), "utf8");
  const lines = text.replace(/\n+$/, "").split("\n");
  return { render: (_cols, rows) => bottom(lines, rows) };
}

/** A scene by its fixture name. */
export function sceneFor(name: string, _spec: SessionSpec): Scene {
  switch (name) {
    case "claude-working":
      return claudeSession({ working: true });
    case "claude-fresh":
      return claudeSession({ working: false, fresh: true, transcript: ["❯ hello", "", "⏺ Hello. What should I work on?"] });
    case "claude-idle":
      return claudeSession({ working: false });
    case "claude-ask-single":
      return new ClaudeAsk([ACCENT]);
    case "claude-ask-multi":
      return new ClaudeAsk(MULTI);
    default:
      throw new Error(`no scene named ${name}`);
  }
}
