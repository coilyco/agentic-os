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
