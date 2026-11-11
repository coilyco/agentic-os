// Which seats this device shows with long lines wrapped to the screen (COI-3544), and
// the device default for the ones nobody chose (COI-3612). Per seat and per device.
// Nothing here reaches the daemon: a wrap is how this client draws a PTY another client
// holds wide, so no other view changes.
const KEY = "aterm.terminal-wrap.v1";
// Seats a person turned off, so a default that wraps does not take their choice back.
const OFF_KEY = "aterm.terminal-wrap-off.v1";
const DEFAULT_KEY = "aterm.terminal-wrap-default.v1";
// A seat that ended leaves its name behind, so each list keeps only the newest.
const KEEP = 64;

/** Auto wraps on a phone or narrow window, not a desktop. On and Off ignore it. */
export type WrapDefault = "auto" | "on" | "off";

function read(key: string): string[] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(key) ?? "[]");
    return Array.isArray(stored) ? stored.filter((name): name is string => typeof name === "string") : [];
  } catch {
    return [];
  }
}

function write(key: string, names: string[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(names.slice(-KEEP)));
  } catch {
    // Storage can be off (a private window). The switch holds until reload.
  }
}

export function loadWrap(session: string): boolean {
  return read(KEY).includes(session);
}

/** What a person chose for this seat with its button, or nothing if never pressed. */
export function loadWrapPick(session: string): boolean | undefined {
  if (read(KEY).includes(session)) return true;
  return read(OFF_KEY).includes(session) ? false : undefined;
}

export function storeWrap(session: string, on: boolean): void {
  write(KEY, [...read(KEY).filter((name) => name !== session), ...(on ? [session] : [])]);
  write(OFF_KEY, [...read(OFF_KEY).filter((name) => name !== session), ...(on ? [] : [session])]);
}

export function loadWrapDefault(): WrapDefault {
  try {
    const stored = localStorage.getItem(DEFAULT_KEY);
    return stored === "on" || stored === "off" ? stored : "auto";
  } catch {
    return "auto";
  }
}

export function storeWrapDefault(choice: WrapDefault): void {
  try {
    localStorage.setItem(DEFAULT_KEY, choice);
  } catch {
    // Storage can be off (a private window). The choice holds until reload.
  }
}

/** Whether a seat nobody chose for wraps itself once its lines run past the screen. */
export function wrapsByDefault(choice: WrapDefault, handheld: boolean): boolean {
  return choice === "on" || (choice === "auto" && handheld);
}
