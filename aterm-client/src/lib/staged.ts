// A command a seat put on the shell beside it, and what clears it (COI-3306).

export interface StagedLine {
  /** The shell the line sits in. */
  terminal: string;
  /** The command, already on that shell's input line. */
  body: string;
  /** The seat's one-line note, possibly empty. It is never typed. */
  text: string;
}

const PASTE = /\x1b\[200~[\s\S]*?(?:\x1b\[201~|$)/g;

/** Whether typed bytes empty the line, as the daemon counts it: Enter, Ctrl-C or Ctrl-U.
 *  A paste is text, so a newline inside one does not. */
export function clearsLine(data: string): boolean {
  return /[\r\n\x03\x15]/.test(data.replace(PASTE, ""));
}
