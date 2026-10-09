// The mobile signal for a seat that starts waiting: one toast, never a jump.

/** How long a toast stays before it dismisses itself. */
export const TOAST_MS = 6_000;

export interface ToastSeat {
  sessionId: string;
  identity: string;
  kind: "asking" | "done";
}

export interface Toast {
  /** The seat a tap opens: the first one waiting. */
  sessionId: string;
  /** Seats the toast stands for, so the count can be read without the words. */
  count: number;
  text: string;
}

/** Waiting seats but the one on screen, as one toast: first named, rest counted. */
export function toastFor(waiting: readonly ToastSeat[], open: string | null): Toast | null {
  const others = waiting.filter((seat) => seat.sessionId !== open);
  const first = others[0];
  if (!first) return null;
  const said = `${first.identity} is ${first.kind === "asking" ? "asking" : "done"}`;
  const more = others.length - 1;
  return { sessionId: first.sessionId, count: others.length, text: more > 0 ? `${said}, and ${more} more waiting` : said };
}
