// The view of a seat this device shows: a level, and whether the screen is up (COI-3532).
// Kept per seat on the device. Nothing here reaches the host.
import { LEVELS, type Level } from "./transcript";

const KEY = "aterm.seat-view.v1";
// A seat that ended leaves its name behind, so only the newest are kept.
const KEEP = 64;

export interface SeatView {
  level: Level;
  /** The screen is showing, which Verbose offers from inside it. */
  raw: boolean;
}

/** A phone reads Focus. A desktop keeps the screen it has always had. */
export const defaultView = (phone: boolean): SeatView => (phone ? { level: "focus", raw: false } : { level: "verbose", raw: true });

function read(): [string, SeatView][] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    if (!Array.isArray(stored)) return [];
    return stored.flatMap((entry) => {
      const [session, view] = Array.isArray(entry) ? entry : [];
      const level = (view as SeatView | undefined)?.level;
      if (typeof session !== "string" || !LEVELS.includes(level as Level)) return [];
      return [[session, { level: level as Level, raw: (view as SeatView).raw === true }] as [string, SeatView]];
    });
  } catch {
    return [];
  }
}

export function loadView(session: string, phone: boolean): SeatView {
  return read().find(([name]) => name === session)?.[1] ?? defaultView(phone);
}

export function storeView(session: string, view: SeatView): void {
  const rest = read().filter(([name]) => name !== session);
  try {
    localStorage.setItem(KEY, JSON.stringify([...rest, [session, view]].slice(-KEEP)));
  } catch {
    // Storage can be off (a private window). The choice holds until reload.
  }
}
