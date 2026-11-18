// A harness select menu draws its footer on the last rows of the screen. The card reader
// can miss a menu it does not know, and this still sees that one is open (COI-3689).
const FOOTER = /\b(?:Enter to (?:select|confirm)|Esc to (?:cancel|go back))\b/i;
// A narrow terminal wraps the footer over a few rows, and the input box sits above it.
const LAST_ROWS = 4;

/** Whether a menu footer is on the last rows. A reply that quotes one sits higher. */
export function menuFooterShowing(screen: readonly string[]): boolean {
  const last = screen.filter((row) => row.trim() !== "").slice(-LAST_ROWS);
  return FOOTER.test(last.join(" "));
}
