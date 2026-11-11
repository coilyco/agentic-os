// The seat overview (COI-3518): seats sorted by what they want of Kai, not by role.
// Needs you, then working, then done. The host is a label on the screen, never a group.
import type { Session } from "./protocol";

export type GroupKey = "needs" | "working" | "done";

export const GROUP_TITLES: Record<GroupKey, string> = { needs: "Needs you", working: "Working", done: "Done" };

export interface SeatGroup {
  key: GroupKey;
  title: string;
  seats: Session[];
}

export interface OverviewContext {
  /** Seats an ask card or a waiting page concerns. */
  needs: ReadonlySet<string>;
  /** Seats that finished a turn Kai has not opened. */
  unseen: Readonly<Record<string, boolean>>;
}

/** Which group a seat is in. A seat that asks is Kai's first, whatever its state says. */
export function groupOf(session: Session, { needs }: OverviewContext): GroupKey {
  if (needs.has(session.id)) return "needs";
  return session.state === "working" ? "working" : "done";
}

/** Longest in its state first, since it has waited most. No reading sorts last. */
function longestWaiting(a: Session, b: Session): number {
  // Not a subtraction: two missing readings would be Infinity - Infinity, which is NaN.
  const x = a.stateSince ?? Infinity;
  const y = b.stateSince ?? Infinity;
  return x === y ? 0 : x < y ? -1 : 1;
}

/** The three groups in order, an empty one left out so no heading sits over nothing.
 * `seats` arrives in roster order and that order breaks every tie. */
export function groupSeats(seats: readonly Session[], context: OverviewContext): SeatGroup[] {
  const by: Record<GroupKey, Session[]> = { needs: [], working: [], done: [] };
  for (const seat of seats) by[groupOf(seat, context)].push(seat);
  by.needs.sort(longestWaiting);
  // A reply Kai has not read goes before one she has, since it is the one she came for.
  by.done.sort((a, b) => Number(!!context.unseen[b.id]) - Number(!!context.unseen[a.id]) || longestWaiting(a, b));
  return (["needs", "working", "done"] as const).filter((key) => by[key].length > 0).map((key) => ({ key, title: GROUP_TITLES[key], seats: by[key] }));
}
