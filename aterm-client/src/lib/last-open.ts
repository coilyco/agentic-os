// What this device last had open, so a reload returns to it. Ids and a name only.
const KEY = "aterm.last-open.v1";

export interface LastOpen {
  host: string;
  /** Absent when the host's own page was the last thing on screen. */
  seat?: { id: string; who: string };
}

export function parseLastOpen(text: string | null): LastOpen | null {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null || typeof (raw as LastOpen).host !== "string") return null;
    const { host, seat } = raw as LastOpen;
    return typeof seat?.id === "string" && typeof seat.who === "string" ? { host, seat: { id: seat.id, who: seat.who } } : { host };
  } catch {
    return null;
  }
}

export function loadLastOpen(): LastOpen | null {
  try {
    return parseLastOpen(localStorage.getItem(KEY));
  } catch {
    return null;
  }
}

export function storeLastOpen(value: LastOpen): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(value));
  } catch {
    // Storage can be off (a private window). The next launch opens on the host list.
  }
}

/** Once the host lists its seats: open the remembered one, or say it is gone. */
export function seatToRestore(last: LastOpen, listed: readonly { id: string }[]): { open: string } | { gone: string } | null {
  if (!last.seat) return null;
  return listed.some((each) => each.id === last.seat?.id) ? { open: last.seat.id } : { gone: last.seat.who };
}
