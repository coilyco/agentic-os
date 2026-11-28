// A tap on a match in the every-seat search opens that seat at the match (COI-3797).
// The seat's view takes the jump once, when it mounts.
import type { TranscriptMatch } from "./transcript";

export interface SearchJump {
  session: string;
  match: TranscriptMatch;
  query: string;
}

export const searchJump = $state<{ next: SearchJump | null }>({ next: null });

export function jumpTo(jump: SearchJump): void {
  searchJump.next = jump;
}

/** The jump waiting for this seat, taken so it happens once. */
export function takeJump(session: string): SearchJump | null {
  const next = searchJump.next;
  if (!next || next.session !== session) return null;
  searchJump.next = null;
  return next;
}
