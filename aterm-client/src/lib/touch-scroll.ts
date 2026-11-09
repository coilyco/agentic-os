// A finger's vertical drag as whole terminal rows (COI-3253). xterm has no touch
// scrolling and the pan box swallows a vertical drag, so the client reads it.

/** Adds a drag to the carry and returns its whole rows. Dragging up is positive. */
export function takeRows(carry: number, dragPx: number, rowPx: number): { rows: number; carry: number } {
  if (!(rowPx > 0)) return { rows: 0, carry: 0 };
  const total = carry + dragPx;
  const rows = Math.trunc(total / rowPx);
  return { rows, carry: total - rows * rowPx };
}

/** Rows to coast after a flick, from its px per ms. A slow lift does not coast. */
export function coastRows(speedPxPerMs: number, rowPx: number): number {
  if (!(rowPx > 0) || Math.abs(speedPxPerMs) < 0.4) return 0;
  return Math.round((speedPxPerMs * 220) / rowPx);
}
