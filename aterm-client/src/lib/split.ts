// The side panel's width and open state. Drag and arrow keys share one clamp.

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

const OPEN_KEY = "aterm.side-open.v1";
/** Per seat since COI-3820. A seat with none falls back to the device-wide choice. */
const SEAT_OPEN_KEY = "aterm.side-open.v2";
const SEATS_KEPT = 100;

function seatChoices(storage: Pick<Storage, "getItem"> | undefined): Record<string, boolean> {
  const parsed: unknown = JSON.parse(storage?.getItem(SEAT_OPEN_KEY) ?? "{}");
  return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? (parsed as Record<string, boolean>) : {};
}

/** Put away until a person opens it: an empty panel costs the terminal 360px. */
export function loadSideOpen(seat: string, storage: Pick<Storage, "getItem"> | undefined = safeStorage()): boolean {
  try {
    const chosen = seatChoices(storage)[seat];
    if (typeof chosen === "boolean") return chosen;
    return storage?.getItem(OPEN_KEY) === "open";
  } catch {
    return false;
  }
}

export function storeSideOpen(seat: string, open: boolean, storage: Pick<Storage, "getItem" | "setItem"> | undefined = safeStorage()): void {
  try {
    const choices = seatChoices(storage);
    delete choices[seat];
    choices[seat] = open;
    const kept = Object.entries(choices).slice(-SEATS_KEPT);
    storage?.setItem(SEAT_OPEN_KEY, JSON.stringify(Object.fromEntries(kept)));
  } catch {
    // Storage can be off. The choice holds until reload.
  }
}

function safeStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage;
  } catch {
    return undefined;
  }
}
