// Where a line that starts with "!" runs, said while it is being typed (COI-3802).
// Which harness does what was observed by pasting "! echo" as the composer does:
// the skill's references/bang-hint.md.

/** Harnesses that run a pasted leading "!" as a command in the seat's own shell. */
const SHELL_MODE = ["claude", "codex"];
/** Harnesses that send the same paste to the model as a message. */
const AS_MESSAGE = ["opencode", "goose"];

export const TERMINAL_HINT = "Runs in a plain terminal. No agent sees it.";

/** Whether a draft starts with "!" once the spaces a send trims are gone. */
export function startsWithBang(text: string): boolean {
  return text.trimStart().startsWith("!");
}

/** The seat composer's line for a "!" draft, in the harness's own terms. */
export function seatHint(harness: string, identity: string): string {
  if (SHELL_MODE.includes(harness)) return `Runs in ${identity}'s shell. The seat sees the output.`;
  if (AS_MESSAGE.includes(harness)) return `Sent to ${identity} as a message. ${harness} has no ! shell mode.`;
  return `Sent to ${identity}. Whether ${harness} runs a ! line as a shell command is not checked.`;
}

const PASTE_OPEN = "\x1b[200~";
const PASTE_CLOSE = "\x1b[201~";

/** A shell's input line as far as typed bytes tell, or null once an arrow, Tab or other
 *  editing key makes it unknowable. Enter, Ctrl-C and Ctrl-U start a fresh line. */
export function trackLine(line: string | null, data: string): string | null {
  let at = line;
  let i = 0;
  while (i < data.length) {
    if (data.startsWith(PASTE_OPEN, i)) {
      const end = data.indexOf(PASTE_CLOSE, i + PASTE_OPEN.length);
      const body = data.slice(i + PASTE_OPEN.length, end < 0 ? data.length : end);
      const lines = body.split(/\r\n|\r|\n/);
      at = lines.length > 1 ? (lines.at(-1) ?? "") : at === null ? null : at + body;
      i = end < 0 ? data.length : end + PASTE_CLOSE.length;
      continue;
    }
    const ch = data.charAt(i);
    i += 1;
    if (ch === "\r" || ch === "\n" || ch === "\x03" || ch === "\x15") at = "";
    else if (ch === "\x7f" || ch === "\b") at = at === null ? null : at.slice(0, -1);
    else if (ch === "\x1b") {
      // An escape sequence moves the cursor or recalls history. Skip the rest of it.
      at = null;
      const seq = /^(\[[0-9;?]*[ -/]*[@-~]|O.|.)/.exec(data.slice(i));
      i += seq ? seq[0].length : 0;
    } else if (ch < " ") at = null;
    else if (at !== null) at += ch;
  }
  return at;
}
