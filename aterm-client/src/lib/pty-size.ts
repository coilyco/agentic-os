// What a terminal tells the daemon about its own box.

export interface Box {
  rows: number;
  cols: number;
}

// An unmeasurable host has no box: xterm's size is the PTY's, not this client's.
export function boxToReport(proposed: Box | undefined, panning: boolean, own: Box): Box | null {
  if (proposed) return proposed;
  return panning ? null : own;
}

// Rows asked beyond the box while a dialog is clipped (COI-3447). See sizing.md.
export const TALL_STEP = 24;
export const TALL_MAX = 96;
export const TALL_RELEASE_MS = 1500;

export interface TallView {
  /** A dialog is on the screen. */
  dialog: boolean;
  /** Options sit above the top row, where nobody can read them. */
  clipped: boolean;
  /** xterm is as tall as the last ask. A raw client elsewhere can cap the rows. */
  honored: boolean;
  /** How long no dialog has been on the screen. */
  idleMs: number;
}

/** Rows beyond the box: grows while a dialog is clipped, drops once it is gone. */
export function nextTall(tall: number, view: TallView): number {
  if (view.clipped && view.honored) return Math.min(TALL_MAX, tall + TALL_STEP);
  if (!view.dialog && view.idleMs >= TALL_RELEASE_MS) return 0;
  return tall;
}
