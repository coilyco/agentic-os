// Which seats this device shows with long lines wrapped to the screen (COI-3544).
// Per seat and per device. Nothing here reaches the daemon: a wrap is how this
// client draws a PTY another client holds wide, so no other view changes.
const KEY = "aterm.terminal-wrap.v1";
// A seat that ended leaves its name behind, so the list keeps only the newest.
const KEEP = 64;

function read(): string[] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    return Array.isArray(stored) ? stored.filter((name): name is string => typeof name === "string") : [];
  } catch {
    return [];
  }
}

export function loadWrap(session: string): boolean {
  return read().includes(session);
}

export function storeWrap(session: string, on: boolean): void {
  const rest = read().filter((name) => name !== session);
  try {
    localStorage.setItem(KEY, JSON.stringify((on ? [...rest, session] : rest).slice(-KEEP)));
  } catch {
    // Storage can be off (a private window). The switch holds until reload.
  }
}
