// Finding the link a tap landed on when rows are folded to the screen (COI-3564).
// xterm maps a pointer to a cell by cell size, which a folded row breaks, so the
// tap is mapped through the text under the finger instead.

// xterm's own addon pattern, so a folded row links what an unfolded one does.
const URL_PATTERN = /(https?|HTTPS?):[/]{2}[^\s"'!*(){}|\\^<>`]*[^\s"':,.!?{}|\\^~[\]`()<>]/g;

/** The address in `text` that covers character `offset`, or null. */
export function urlAt(text: string, offset: number): string | null {
  for (const match of text.matchAll(URL_PATTERN)) {
    const start = match.index ?? 0;
    if (offset >= start && offset < start + match[0].length) return match[0];
  }
  return null;
}

/** `chain` is the rows of one wrapped line, `row` the one tapped, `offset` a character in it. */
export function urlAtChain(chain: readonly string[], row: number, offset: number): string | null {
  const before = chain.slice(0, row).reduce((sum, text) => sum + text.length, 0);
  return urlAt(chain.join(""), before + offset);
}

interface Cell {
  getChars(): string;
  getWidth(): number;
}
interface Line {
  length: number;
  getCell(x: number): Cell | undefined;
}

/** A buffer row as text with one character per cell, a wide one once and an empty one as a space. */
export function lineChars(line: Line): string {
  let text = "";
  for (let x = 0; x < line.length; x++) {
    const cell = line.getCell(x);
    if (!cell || cell.getWidth() === 0) continue;
    text += cell.getChars() || " ";
  }
  return text;
}
