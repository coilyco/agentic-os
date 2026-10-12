// On a phone the composer is the one input, so a harness's own input box is hidden.
// Only when the bottom of the screen is exactly that box. See architecture.md.
import { detectChoice } from "./choices";

const RULE = /^\s*[╭╰┌└─━]{8,}/;
const PROMPT = /^\s*(?:│\s*)?[❯>▌›]\s?/;
const NUMBERED = /^\s*(?:│\s*)?[❯>]?\s*\d+[.)]\s/;
const STATUS_ROWS = 3;
// opencode draws a left-rail box with no rules: rail rows, a cap row, then a hint row.
const RAIL = /^\s*┃/;
const RAIL_CAP = /^\s*╹▀{8,}/;
const RAIL_MODEL = /^\s*┃\s+\S+ · /;
const RAIL_HINT = /ctrl\+p commands/;
const RAIL_PATH = /\d+\.\d+\.\d+$/;
const RAIL_ROWS = 12;
// A prompt a seat is waiting on, in a harness whose menus detectChoice does not read.
const WAITING = /\b(?:allow|reject|deny|approve)\b|press enter to confirm|[›❯>]\s*\d+[.)]\s/i;
// codex draws a bare prompt row with a status row and a hint row under it.
const CODEX_PROMPT = /^\s*›\s/;
const CODEX_HINT = /^\s*\? for shortcuts/;
const CODEX_ROWS = 4;
// goose draws a context gauge row, then one prompt row that says how to send.
const GOOSE_GAUGE = /^\s*╌{8,}\s+\d+%/;
const GOOSE_PROMPT = /^>\s+Enter to send/;

/** Bottom rows that are the harness's input box and status lines. 0 shows everything. */
export function harnessInputRows(screen: readonly string[]): number {
  if (detectChoice(screen) !== null) return 0;
  return ruledBoxRows(screen) || railBoxRows(screen) || codexBoxRows(screen) || gooseBoxRows(screen);
}

function ruledBoxRows(screen: readonly string[]): number {
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

/** opencode's box. Model row, cap and hint must all show, so a dialog never matches. */
function railBoxRows(screen: readonly string[]): number {
  let end = screen.length;
  while (end > 0 && screen[end - 1]!.trim() === "") end--;
  // The hint row ends the box, with at most a path and version row under it.
  let hint = end - 1;
  if (RAIL_PATH.test(screen[hint]!)) {
    hint--;
    while (hint >= 0 && screen[hint]!.trim() === "") hint--;
  }
  if (hint < 2 || !RAIL_HINT.test(screen[hint]!) || !RAIL_CAP.test(screen[hint - 1]!) || !RAIL_MODEL.test(screen[hint - 2]!)) return 0;
  let top = hint - 2;
  while (top > 0 && hint - top < RAIL_ROWS && RAIL.test(screen[top - 1]!)) top--;
  if (screen.slice(top, hint).some((row) => WAITING.test(row))) return 0;
  return screen.length - top;
}

/** codex's box. Only its own hint row ends it, so a permission prompt never matches. */
function codexBoxRows(screen: readonly string[]): number {
  let end = screen.length;
  while (end > 0 && screen[end - 1]!.trim() === "") end--;
  const hint = end - 1;
  if (hint < 1 || !CODEX_HINT.test(screen[hint]!)) return 0;
  for (let at = hint - 1; at >= Math.max(0, hint - CODEX_ROWS); at--) {
    const row = screen[at]!;
    if (CODEX_PROMPT.test(row)) return WAITING.test(row) ? 0 : screen.length - at;
    if (WAITING.test(row)) return 0;
  }
  return 0;
}

/** goose's box, only while its send hint is the prompt, so a typed line never matches. */
function gooseBoxRows(screen: readonly string[]): number {
  let end = screen.length;
  while (end > 0 && screen[end - 1]!.trim() === "") end--;
  if (end < 2 || !GOOSE_PROMPT.test(screen[end - 1]!)) return 0;
  let gauge = end - 2;
  while (gauge >= 0 && screen[gauge]!.trim() === "") gauge--;
  return gauge >= 0 && GOOSE_GAUGE.test(screen[gauge]!) ? screen.length - gauge : 0;
}
