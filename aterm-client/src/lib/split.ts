// The side panel's width as Kai set it. Drag and arrow keys share one clamp.

export const SIDE_MIN = 280;
/** The seat's own terminal never gets narrower than this. */
export const MAIN_MIN = 360;
export const KEY_STEP = 24;
export const KEY_STEP_BIG = 96;

export function clampSide(width: number, container: number): number {
  const max = Math.max(SIDE_MIN, container - MAIN_MIN);
  return Math.round(Math.min(max, Math.max(SIDE_MIN, width)));
}

/** The width a key asks for, or null for a key the handle ignores. Left grows it. */
export function keyedSide(key: string, shift: boolean, current: number, container: number): number | null {
  const step = shift ? KEY_STEP_BIG : KEY_STEP;
  if (key === "ArrowLeft") return clampSide(current + step, container);
  if (key === "ArrowRight") return clampSide(current - step, container);
  if (key === "Home") return clampSide(Number.POSITIVE_INFINITY, container);
  if (key === "End") return SIDE_MIN;
  return null;
}

const KEY = "aterm.side-width.v1";

export function loadSide(storage: Pick<Storage, "getItem"> | undefined = safeStorage()): number | null {
  try {
    const width = Number(storage?.getItem(KEY));
    return Number.isFinite(width) && width >= SIDE_MIN ? width : null;
  } catch {
    return null;
  }
}

/** `null` forgets it, so the panel goes back to its own default width. */
export function storeSide(width: number | null, storage: Pick<Storage, "setItem" | "removeItem"> | undefined = safeStorage()): void {
  try {
    if (width === null) storage?.removeItem(KEY);
    else storage?.setItem(KEY, String(width));
  } catch {
    // Storage can be off. The width holds until reload.
  }
}

function safeStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage;
  } catch {
    return undefined;
  }
}
