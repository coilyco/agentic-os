// Screens a demo seat draws as a harness would, so the crop and choice card run in Demo.
// Shapes follow the captured screens in harness.test.ts and choices.test.ts.
const DIM = "\x1b[2m";
const RESET = "\x1b[0m";

export type Frame = (cols: number) => string[];

/** A Claude Code seat at rest: the ruled input box and a status row. */
export const CLAUDE_BOX: Frame = (cols) => {
  const status = "  ⏵⏵ auto mode on (shift+tab to cycle)".slice(0, cols);
  return ["─".repeat(Math.max(cols - 7, 3)) + " seat ─", "❯ ", "─".repeat(cols), "", `${DIM}${status}${RESET}`];
};

/** A permission menu, which the crop must never hide. */
export const CLAUDE_DIALOG: Frame = (cols) =>
  [
    "─".repeat(cols),
    " Run this command?",
    "",
    "   just aterm-client-gate",
    "",
    " ❯ 1. Yes",
    "   2. Yes, and don't ask again this session",
    "   3. No, and tell Claude what to do differently",
    "",
    " Enter to confirm · Esc to cancel",
  ].map((row) => row.slice(0, cols));

/** A harness repaints on resize: rules to the new width, frame on the bottom row. */
export function drawHarness(transcript: readonly string[], frame: Frame, rows = 0, cols = 70): string {
  const lines = frame(cols);
  const blank = Math.max(rows - transcript.length - lines.length, 1);
  return "\x1b[H\x1b[2J" + [...transcript, ...Array<string>(blank).fill(""), ...lines].join("\r\n");
}
