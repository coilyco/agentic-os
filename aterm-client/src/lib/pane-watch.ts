// A pane that drew nothing asks the host for the seat's screen again.

/** Waits before each ask. An idle seat sends nothing, so a lost replay is not made up. */
const WATCH_MS = [2_500, 5_000, 10_000];

/** How long to wait before the next ask, or null once they are spent. */
export function watchDelay(asked: number): number | null {
  return WATCH_MS[asked] ?? null;
}

// A step run once the bytes written so far are parsed, since xterm resizes at once.
// See references/sizing.md in the tooling-aterm-client skill.
export function afterQueued(term: { write(data: string, done: () => void): void }, label: string, step: () => void): void {
  term.write("", () => safely(label, step));
}

/** Runs a step that must not stop the next. A throwing xterm write callback wedges it. */
export function safely(label: string, step: () => void): void {
  try {
    step();
  } catch (error) {
    console.error(`aterm: ${label} failed`, error);
  }
}
