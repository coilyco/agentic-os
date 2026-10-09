// On a phone the composer is the one input, so a harness's own input box is hidden.
// Only when the bottom of the screen is exactly that box. See architecture.md.
import { detectChoice } from "./choices";

const RULE = /^\s*[╭╰┌└─━]{8,}/;
const PROMPT = /^\s*(?:│\s*)?[❯>▌›]\s?/;
const NUMBERED = /^\s*(?:│\s*)?[❯>]?\s*\d+[.)]\s/;
const STATUS_ROWS = 3;

/** Bottom rows that are the harness's input box and status lines. 0 shows everything. */
export function harnessInputRows(screen: readonly string[]): number {
  if (detectChoice(screen) !== null) return 0;
  // Blank rows under the box belong to it.
  let end = screen.length;
  while (end > 0 && screen[end - 1]!.trim() === "") end--;
  for (let status = 0; status <= STATUS_ROWS; status++) {
    const bottom = end - 1 - status;
    if (bottom < 2 || !RULE.test(screen[bottom]!)) continue;
    const prompt = screen[bottom - 1]!;
    if (!PROMPT.test(prompt) || NUMBERED.test(prompt) || !RULE.test(screen[bottom - 2]!)) continue;
    const footer = screen.slice(bottom + 1, end);
    if (footer.some((row) => RULE.test(row) || PROMPT.test(row))) continue;
    return screen.length - (bottom - 2);
  }
  return 0;
}
