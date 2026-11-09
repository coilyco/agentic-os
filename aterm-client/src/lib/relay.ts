// Where a worker's question is shown when it was relayed to a director's pane (COI-3241).
import type { Ask } from "./protocol";

/** The session whose pane shows the card: the director it was sent to, else the asker. */
export function cardSeatOf(ask: Ask, live: ReadonlySet<string>): string {
  return ask.cardSeat && live.has(ask.cardSeat) ? ask.cardSeat : ask.session;
}

/** What a row says about an ask that concerns it, from the row's own seat. */
export function askerLine(ask: Ask, seat: string, live: ReadonlySet<string>, nameOf: (id: string) => string): string {
  const shownOn = cardSeatOf(ask, live);
  if (shownOn === seat && ask.session !== seat) return `${nameOf(ask.session)} asks you`;
  if (ask.session === seat && shownOn !== seat) return `asking you, on ${nameOf(shownOn)}`;
  return "asking you";
}
