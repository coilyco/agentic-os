// Reads a harness's own select menu off the screen so the client can offer
// native buttons for it. Shape and safeguards: the choices reference.
import type { Ask } from "./protocol";

export interface ChoiceOption {
  label: string;
  description: string;
  /** Claude Code's "Type something." row takes a typed answer instead. */
  freeText: boolean;
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
const SCROLL_MARK = /^[↑↓]\s/;
const FREE_TEXT = /^type something\.?$/i;
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
    return rest.trim() && !rest.startsWith(" ") ? rest.trim() : null;
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
      const text = label.replace(/^\d+\.\s+/, "");
      options.push({ label: text, description: "", freeText: FREE_TEXT.test(text) });
    } else if (RULE.test(line) && optionAt(lines[row + 1] ?? "") !== null) {
      continue;
    } else if (options.length && isDescription(line, column)) {
      const last = options[options.length - 1]!;
      last.description = `${last.description} ${line.trim()}`.trim();
    } else {
      break;
    }
  }
  const footer = lines.slice(row, row + 3).find((line) => FOOTER.test(line));
  if (!footer || options.length < 2 || cursor === -1) return null;
  const [first, ...rest] = questionAbove(lines, top).split("\n\n");
  const chipped = first !== undefined && CHIP.test(first);
  const header = chipped ? first.replace(CHIP, "") : "";
  const question = (chipped ? rest : [first ?? "", ...rest]).join("\n\n").trim();
  return { header, question, options, cursor, cancellable: /Esc to/.test(footer), multi: false };
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
  return found.reduce((text, line) => (line === "" ? `${text}\n` : text.endsWith("\n") || !text ? `${text}${line}` : `${text} ${line}`), "").replace(/\n/g, "\n\n").trim();
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
