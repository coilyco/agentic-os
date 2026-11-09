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
