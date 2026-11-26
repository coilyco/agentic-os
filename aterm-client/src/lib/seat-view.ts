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

const DEFAULT_KEY = "aterm.seat-view-default.v1";

/** Where a seat opens until a view is picked for it. Auto is the device's own default. */
export type ViewDefault = "auto" | Level;

export function loadViewDefault(): ViewDefault {
  try {
    const stored = localStorage.getItem(DEFAULT_KEY);
    return LEVELS.includes(stored as Level) ? (stored as Level) : "auto";
  } catch {
    return "auto";
  }
}

export function storeViewDefault(choice: ViewDefault): void {
  try {
    localStorage.setItem(DEFAULT_KEY, choice);
  } catch {
    // Storage can be off (a private window). The choice holds until reload.
  }
}

/** Auto is Focus on a phone and a desktop's screen. A person's choice wins over both. */
export function defaultView(phone: boolean): SeatView {
  const chosen = loadViewDefault();
  if (chosen !== "auto") return { level: chosen, raw: false };
  return phone ? { level: "focus", raw: false } : { level: "verbose", raw: true };
}

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

/** One of the five views a seat's View button offers (COI-3757). */
export type ViewPick = Level | "raw-wrapped" | "raw-unwrapped";

export const CHOICES: readonly { choice: ViewPick; label: string; detail: string }[] = [
  { choice: "focus", label: "Focus", detail: "Each prompt, one line per step, and the last reply." },
  { choice: "default", label: "Default", detail: "Every reply, with the first lines of each step." },
  { choice: "verbose", label: "Verbose", detail: "All of it, every step in full." },
  { choice: "raw-wrapped", label: "Raw wrapped", detail: "The seat's own screen, long lines folded to fit." },
  { choice: "raw-unwrapped", label: "Raw unwrapped", detail: "The seat's own screen as drawn. Long lines pan sideways." },
];

export function choiceLabel(choice: ViewPick): string {
  return CHOICES.find((each) => each.choice === choice)?.label ?? choice;
}

/** A stored view and its screen's wrap, as one of the five. Upgrades keep their view. */
export function choiceOf(view: SeatView, screen: boolean, wrapping: boolean): ViewPick {
  if (screen || view.raw) return wrapping ? "raw-wrapped" : "raw-unwrapped";
  return view.level;
}
