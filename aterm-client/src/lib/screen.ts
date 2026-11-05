/** Where the harness is drawing, whatever the reader has scrolled to. */
export interface RowSource {
  baseY: number;
  getLine(row: number): { translateToString(trimRight: boolean): string } | undefined;
}

/** The live screen's rows. Scrollback rows would close the card. */
export function screenRows(buffer: RowSource, rows: number): string[] {
  return Array.from({ length: rows }, (_, at) => buffer.getLine(buffer.baseY + at)?.translateToString(true) ?? "");
}

/** Nothing drawn on the live screen yet, as before a seat prints its first line. */
export function isBlank(rows: readonly string[]): boolean {
  return rows.every((row) => row.trim() === "");
}

/** A row of the live screen with its cells, for reading what the text does not say. */
export interface CellRow {
  length: number;
  isWrapped: boolean;
  translateToString(trimRight: boolean): string;
  getCell(x: number): { isBgDefault(): boolean } | undefined;
}

export interface CellRowSource {
  baseY: number;
  getLine(row: number): CellRow | undefined;
}

export interface TabBarCells {
  text: string;
  /** Columns whose cells have a background: how Claude Code marks the current tab. */
  highlighted: number[];
}

/** The dialog's tab bar and its highlighted cells, or null when none is showing. */
export function tabBarReading(buffer: CellRowSource, rows: number): TabBarCells | null {
  for (let at = rows - 1; at >= 0; at--) {
    const line = buffer.getLine(buffer.baseY + at);
    if (!line || line.isWrapped || !/^\s*←\s+[☐☒✔✓■□]/u.test(line.translateToString(true))) continue;
    // A bar wider than the terminal wraps, so its rows are read as one line.
    let text = "";
    const highlighted: number[] = [];
    let next: CellRow | undefined = line;
    for (let row = at; next; row++, next = buffer.getLine(buffer.baseY + row)) {
      if (row > at && !next.isWrapped) break;
      const offset = text.length;
      const part = next.translateToString(false);
      for (let x = 0; x < part.length; x++) if (next.getCell(x)?.isBgDefault() === false) highlighted.push(offset + x);
      text += part;
    }
    return { text: text.trimEnd(), highlighted };
  }
  return null;
}
