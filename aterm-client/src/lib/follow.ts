// How far a terminal view sits above its newest line, so the client can offer a way back.
export interface BufferView {
  baseY: number;
  viewportY: number;
}

export interface Follow {
  away: boolean;
  unseen: number;
}

export const AT_END: Follow = { away: false, unseen: 0 };
const COUNT_CAP = 999;

/** Lines from the top of the view to the live bottom. Zero is the end. */
export function linesAbove(view: BufferView): number {
  return Math.max(0, view.baseY - view.viewportY);
}

/** The view moved. Away while above the end, and the count starts over on each return. */
export function scrolled(current: Follow, above: number): Follow {
  if (above === 0) return AT_END;
  return current.away ? current : { away: true, unseen: 0 };
}

/** A write landed. While away, every line it pushed past the view is a new one. */
export function wrote(current: Follow, before: number, after: number): Follow {
  if (!current.away || after <= before) return current;
  return { away: true, unseen: current.unseen + (after - before) };
}

export function jumpLabel(unseen: number): { count: string; spoken: string } {
  if (unseen <= 0) return { count: "", spoken: "Jump to latest output" };
  const count = unseen > COUNT_CAP ? `${COUNT_CAP}+` : String(unseen);
  return { count, spoken: `Jump to latest output, ${unseen} new ${unseen === 1 ? "line" : "lines"}` };
}
