// Rows asked of the PTY for the harness box (COI-3576). A new PTY size makes the harness
// redraw, so a notice that comes and goes must not move it each time.
export const SHRINK_HOLD_MS = 20_000;

export interface SteadyBox {
  asked: number;
  /** When the box first read smaller than what is asked, or null. */
  since: number | null;
}

export const NO_BOX: SteadyBox = { asked: 0, since: null };

/** Grows at once, so the box is never cut. Shrinks after a smaller box lasts the hold. */
export function steadyBox(prev: SteadyBox, seen: number, now: number): SteadyBox {
  if (seen >= prev.asked) return { asked: seen, since: null };
  const since = prev.since ?? now;
  return now - since >= SHRINK_HOLD_MS ? { asked: seen, since: null } : { asked: prev.asked, since };
}
