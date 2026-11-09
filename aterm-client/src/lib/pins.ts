// Seats pinned to the top of the list (COI-3346). Per device, keyed by session name: a
// reused name is retired for a day, so a pin on a gone seat is inert.
const KEY = "aterm.pins.v1";

export interface Pins {
  pinned: string[];
  /** Seats a person unpinned, so a role's default pin does not come back on them. */
  unpinned: string[];
}

export const NO_PINS: Pins = { pinned: [], unpinned: [] };

const names = (value: unknown): string[] => (Array.isArray(value) ? value.filter((each): each is string => typeof each === "string") : []);

export function parsePins(text: string | null): Pins {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null) return NO_PINS;
    return { pinned: names((raw as Pins).pinned), unpinned: names((raw as Pins).unpinned) };
  } catch {
    return NO_PINS;
  }
}

export function loadPins(): Pins {
  try {
    return parsePins(localStorage.getItem(KEY));
  } catch {
    return NO_PINS;
  }
}

export function storePins(pins: Pins): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(pins));
  } catch {
    // Storage can be off (a private window). The pins last until reload.
  }
}

/** A person's own pin or unpin wins. With neither, the role's default decides. */
export function isPinned(id: string, pins: Pins, roleDefault: boolean): boolean {
  if (pins.pinned.includes(id)) return true;
  if (pins.unpinned.includes(id)) return false;
  return roleDefault;
}

/** The pins after a person flips one seat. */
export function flipPin(id: string, pinnedNow: boolean, pins: Pins): Pins {
  const rest = (list: string[]) => list.filter((each) => each !== id);
  return pinnedNow ? { pinned: rest(pins.pinned), unpinned: [...rest(pins.unpinned), id] } : { pinned: [...rest(pins.pinned), id], unpinned: rest(pins.unpinned) };
}
