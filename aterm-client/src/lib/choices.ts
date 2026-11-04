// Reads a harness's own select menu off the screen so the client can offer
// native buttons for it. Shape and safeguards: the choices reference.
import type { Ask } from "./protocol";

export interface ChoiceOption {
  label: string;
  description: string;
  /** Claude Code's "Type something." row takes a typed answer instead. */
  freeText: boolean;
  /** A multi-select's checkbox state. Undefined on a row that has no checkbox. */
  checked?: boolean;
}

export interface Choice {
  header: string;
  question: string;
  options: ChoiceOption[];
  cursor: number;
  cancellable: boolean;
  /** Several options may be picked, then submitted together. */
  multi: boolean;
}

const CURSOR = "❯";
const FOOTER = /\b(Enter|Esc) to\b/;
const RULE = /^[\s│|]*[─━╭╰┌└]{3,}/;
const CHIP = /^[☐☒✔✓■□]\s+/;
// A multi-question dialog opens with a tab bar that does not say which tab is current.
const TAB_BAR = /^←\s+[☐☒✔✓■□]\s/;
const SCROLL_MARK = /^[↑↓]\s/;
const FREE_TEXT = /^type something\.?$/i;
const CHECKBOX = /^\[([ ✔✓xX])\]\s+/;
// Next and Submit are rows of their own, with no number, at the label column.
const ACTION_ROW = /^(next|submit)$/i;
const QUESTION_LINES = 8;

function strip(line: string): string {
  return line.replace(/^[\s│|]*?(?=\S)/, (lead) => lead.replace(/[│|]/g, " "));
}

/** The last select menu on screen, or null when none is showing. */
export function detectChoice(screen: readonly string[]): Choice | null {
  const lines = screen.map(strip);
  const cursorRow = lines.findLastIndex((line) => line.trimStart().startsWith(CURSOR));
  if (cursorRow === -1) return null;
  const column = lines[cursorRow]!.indexOf(CURSOR);
  const optionAt = (line: string): string | null => {
    const lead = line.slice(0, column + 2);
    const rest = line.slice(column + 2);
    if (lead.trim() !== "" && lead.trim() !== CURSOR) return null;
    if (rest.trim() && !rest.startsWith(" ")) return rest.trim();
    return ACTION_ROW.test(rest.trim()) ? rest.trim() : null;
  };
  let top = cursorRow;
  while (top > 0 && (optionAt(lines[top - 1]!) !== null || isDescription(lines[top - 1]!, column))) top--;
  while (top < cursorRow && optionAt(lines[top]!) === null) top++;
  const options: ChoiceOption[] = [];
  let cursor = -1;
  let row = top;
  for (; row < lines.length; row++) {
    const line = lines[row]!;
    const label = optionAt(line);
    if (label !== null) {
      if (line.trimStart().startsWith(CURSOR)) cursor = options.length;
      const numbered = label.replace(/^\d+\.\s+/, "");
      const box = CHECKBOX.exec(numbered);
      const text = box ? numbered.slice(box[0].length) : numbered;
      options.push({ label: text, description: "", freeText: FREE_TEXT.test(text), ...(box ? { checked: box[1] !== " " } : {}) });
    } else if (RULE.test(line) && optionAt(lines[row + 1] ?? "") !== null) {
      continue;
    } else if (options.length && isDescription(line, column)) {
      const last = options[options.length - 1]!;
      last.description = `${last.description} ${line.trim()}`.trim();
    } else {
      break;
    }
  }
  const footerRows = lines.slice(row, row + 5);
  const footer = footerRows.find((line) => FOOTER.test(line));
  // A narrow terminal wraps the footer onto later rows.
  const footerText = footer === undefined ? "" : footerRows.slice(footerRows.indexOf(footer)).join(" ");
  // The review screen draws no footer, only its Submit answers row.
  const reviewing = options.some((option) => /^submit answers$/i.test(option.label));
  if ((!footer && !reviewing) || options.length < 2 || cursor === -1) return null;
  const [first, ...rest] = questionAbove(lines, top).split("\n\n");
  const tabbed = first !== undefined && TAB_BAR.test(first);
  const chipped = first !== undefined && CHIP.test(first);
  const header = chipped ? first.replace(CHIP, "") : "";
  const question = (chipped || tabbed ? rest : [first ?? "", ...rest]).join("\n\n").trim();
  const multi = options.some((option) => option.checked !== undefined);
  return { header, question, options, cursor, cancellable: /Esc to/.test(footerText), multi };
}

function isDescription(line: string, column: number): boolean {
  const indent = line.length - line.trimStart().length;
  return line.trim() !== "" && indent > column + 2;
}

// Paragraphs between the nearest rule and the options, blank lines kept as breaks.
function questionAbove(lines: readonly string[], top: number): string {
  const found: string[] = [];
  let taken = 0;
  for (let row = top - 1; row >= 0 && taken < QUESTION_LINES; row--) {
    const line = lines[row]!;
    if (RULE.test(line)) break;
    if (SCROLL_MARK.test(line.trim())) continue;
    if (line.trim() === "") {
      if (found.length && found[0] !== "") found.unshift("");
      continue;
    }
    found.unshift(line.trim());
    taken++;
  }
  while (found[0] === "") found.shift();
  // The review screen lists "● question" and "→ answer" rows, one per line.
  const hard = (line: string): boolean => /^[●→]\s/.test(line);
  return found
    .reduce((text, line) => (line === "" ? `${text}\n` : text.endsWith("\n") || !text ? `${text}${line}` : `${text}${hard(line) ? "\0" : " "}${line}`), "")
    .replace(/\n/g, "\n\n")
    .replace(/\0/g, "\n")
    .trim();
}

const ADVANCE = /^(next|submit( answers)?|done)$/i;

/** The row that moves a multi-step menu on, or -1. The card pins it. */
export function advanceIndex(choice: Choice): number {
  return choice.multi ? -1 : choice.options.findIndex((option) => !option.freeText && ADVANCE.test(option.label));
}

/** A multi-select's unnumbered Next or Submit row, or -1. */
export function nextRowIndex(choice: Choice): number {
  return choice.multi ? choice.options.findIndex((option) => option.checked === undefined && !option.freeText && ADVANCE.test(option.label)) : -1;
}

const DOWN = "\x1b[B";
const UP = "\x1b[A";

/** Key chunks for a native multi-select. See the choices reference. */
export function keysForPicks(choice: Choice, picks: number[], text = ""): string[] {
  const chunks: string[] = [];
  let at = choice.cursor;
  const move = (to: number): void => {
    if (to === at) return;
    chunks.push((to > at ? DOWN : UP).repeat(Math.abs(to - at)));
    at = to;
  };
  choice.options.forEach((option, index) => {
    if (option.checked === undefined) return;
    const wanted = picks.includes(index);
    if (option.freeText) {
      if (wanted && text) {
        move(index);
        chunks.push(text);
      } else if (!wanted && option.checked) {
        move(index);
        chunks.push("\r");
      }
    } else if (wanted !== option.checked) {
      move(index);
      chunks.push("\r");
    }
  });
  const next = nextRowIndex(choice);
  if (next !== -1) {
    move(next);
    chunks.push("\r");
  }
  return chunks;
}

/** Key chunks to reach `target`, type `text`, and confirm. See the choices reference. */
export function keysFor(choice: Choice, target: number, text = ""): string[] {
  const steps = target - choice.cursor;
  const arrow = steps > 0 ? "\x1b[B" : "\x1b[A";
  const chunks = [arrow.repeat(Math.abs(steps)), text, "\r"];
  return chunks.filter((chunk) => chunk !== "");
}

export const CANCEL = ["\x1b"];

/** An `ask_choice` from the daemon, in the card's shape. "Other" becomes a typed row. */
export function choiceFromAsk(ask: Ask): Choice {
  const options = ask.options.map((option) => ({ label: option.label, description: option.description, freeText: false }));
  if (ask.allowOther) options.push({ label: "Type something.", description: "", freeText: true });
  return { header: ask.header, question: ask.question, options, cursor: 0, cancellable: true, multi: ask.multi };
}

/** Card picks as the daemon wants them: its own options, with "Other" as text only. */
export function askAnswer(ask: Ask, picks: number[], text?: string): { picks: number[]; text?: string } {
  const own = picks.filter((pick) => pick < ask.options.length);
  const typed = ask.allowOther && picks.some((pick) => pick >= ask.options.length) ? text?.trim() : undefined;
  return typed ? { picks: own, text: typed } : { picks: own };
}
