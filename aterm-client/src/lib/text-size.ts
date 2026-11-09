// The terminal's text size, remembered on this device (COI-3481). Not per seat.
const KEY = "aterm.terminal-text-size.v1";

export const TEXT_SIZES = [9, 10, 11, 12, 13, 14, 16, 18] as const;
export const DEFAULT_TEXT_SIZE = 14;

/** The stored size, or 14 when none is stored or the value is not a known step. */
export function loadTextSize(): number {
  try {
    const stored = Number(localStorage.getItem(KEY));
    return TEXT_SIZES.find((size) => size === stored) ?? DEFAULT_TEXT_SIZE;
  } catch {
    return DEFAULT_TEXT_SIZE;
  }
}

export function storeTextSize(size: number): void {
  try {
    localStorage.setItem(KEY, String(size));
  } catch {
    // Storage can be off (a private window). The size holds until reload.
  }
}

/** One step smaller (-1) or larger (+1), stopping at the ends. */
export function stepTextSize(size: number, direction: 1 | -1): number {
  const last = TEXT_SIZES.length - 1;
  const at = TEXT_SIZES.findIndex((step) => step >= size);
  // Past the largest step, or between two: the next step in the direction asked.
  const next = at === -1 ? last : TEXT_SIZES[at] === size ? at + direction : direction === 1 ? at : at - 1;
  return TEXT_SIZES[Math.min(last, Math.max(0, next))]!;
}
