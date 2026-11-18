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

export type LoginStage = "method" | "code" | "error";

// A made-up address. The card reads host and path, so it offers it like a real one.
const DEMO_LINK = "https://claude.com/cai/oauth/authorize?code=true&client_id=demo&response_type=code&redirect_uri=https%3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback&state=demo-not-real";

/** Breaks a row at spaces to `cols`, later rows indented, as the harness does. */
function wrapRow(row: string, cols: number): string[] {
  const out: string[] = [];
  let line = /^ */.exec(row)![0];
  for (const word of row.trimStart().split(" ")) {
    if (line.trim() && line.length + 1 + word.length > cols) {
      out.push(line);
      line = `     ${word}`;
    } else line = line.trim() ? `${line} ${word}` : `${line}${word}`;
  }
  return [...out, line];
}

/** A link has no spaces, so the harness breaks it at the width with nothing added. */
const chunks = (text: string, width: number): string[] => text.match(new RegExp(`.{1,${Math.max(width, 8)}}`, "g")) ?? [];

/** A seat with no login, in the three screens the card reads (shapes: login.test.ts). */
export const loginFrame =
  (stage: LoginStage, typed = ""): Frame =>
  (cols) => {
    const edge = "─".repeat(Math.max(cols - 4, 4));
    const field = [`╭${edge}╮`, `│ ${typed}`.padEnd(Math.max(cols - 3, 5)).slice(0, cols - 2) + "│", `╰${edge}╯`];
    if (stage === "code") return [" Browser didn't open? Use the URL below to sign in (c to copy)", "", ...chunks(DEMO_LINK, cols), "", " Paste code here if prompted", ...field, "", " Esc to go back"];
    const rows =
      stage === "method"
        ? [" Select login method:", "", " ❯ 1. Claude account with subscription · Individual, Team, or Enterprise", "   2. Anthropic Console account · API usage billing", "   3. 3rd-party platform · Bedrock, Microsoft Foundry, Google Vertex AI"]
        : [" OAuth error: That code wasn't accepted. It may be incomplete, expired, already used, or from an earlier sign-in attempt. Sign in again to get a new code.", "", " Press Enter to retry. (Esc to continue without logging in)"];
    return rows.flatMap((row) => wrapRow(row, cols));
  };
