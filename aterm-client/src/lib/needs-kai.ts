// The seats that need Kai (COI-3342). A seat that only finished its turn is not one:
// with many seats cycling, that count was nearly all of the number and told her nothing.
import type { Ask, SeatArtifact } from "./protocol";
import { cardSeatOf } from "./relay";

export interface NeedsKai {
  sessionId: string;
  role: string;
  identity: string;
  kind: "asking" | "artifact" | "opened";
  /** The first waiting page of the seat, for an artifact entry. */
  pageId?: string;
}

interface Seat {
  id: string;
  role: string;
  identity: string;
}

/** Live seats with an ask card, a page marked waiting, then a page card (COI-3328). */
export function seatsNeedingKai(asks: readonly Ask[], artifacts: readonly SeatArtifact[], sessions: readonly Seat[], opened: readonly { sessionId: string }[] = []): NeedsKai[] {
  const bySession = new Map(sessions.map((seat) => [seat.id, seat]));
  const live = new Set(bySession.keys());
  // A standing page outlives its seat. With no seat on screen there is nothing to open.
  const pages = artifacts.filter((page) => page.waiting && !page.seatEnded).map((page) => ({ id: page.session, kind: "artifact" as const, pageId: page.id }));
  const cards = opened.map((page) => ({ id: page.sessionId, kind: "opened" as const, pageId: undefined }));
  const questions = asks.map((ask) => ({ id: cardSeatOf(ask, live), kind: "asking" as const, pageId: undefined }));
  const seen = new Set<string>();
  const found: NeedsKai[] = [];
  for (const { id, kind, pageId } of [...questions, ...pages, ...cards]) {
    const seat = bySession.get(id);
    if (!seat || seen.has(id)) continue;
    seen.add(id);
    found.push({ sessionId: id, role: seat.role, identity: seat.identity, kind, ...(pageId ? { pageId } : {}) });
  }
  return found;
}
