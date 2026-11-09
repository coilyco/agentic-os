// When a seat is idle enough to offer Close (COI-3488). One mistaken tap ends a seat, so
// every signal must say idle, and a signal the daemon does not send reads as not idle.
import type { Session } from "./protocol";

export interface SeatContext {
  /** An ask card concerns it, or a page it left is waiting on Kai. */
  needsKai: boolean;
  /** It finished a turn Kai has not opened yet, so its reply is unread. */
  unseen: boolean;
}

export function fullyIdle(session: Session, { needsKai, unseen }: SeatContext): boolean {
  if (needsKai || unseen) return false;
  if (session.state !== "idle" || session.starting) return false;
  // The daemon's own reading of the screen. A prompt or a busy seat is not idle.
  if (session.screen !== "idle") return false;
  if (session.pending > 0 || session.drafting) return false;
  // Background work (CI, a monitor) reads idle on the screen. No field means unknown.
  if (session.background !== 0) return false;
  return session.ref !== undefined;
}

/** What the list says once a seat is closed. Reopen only where the daemon has it. */
export function closedWords(identity: string, roleName: string, canReopen: boolean): string {
  return canReopen
    ? `Closed ${identity}. Reopen it to pick its conversation up again, or start ${roleName} fresh from Start.`
    : `Closed ${identity}. It does not reopen from here. Start ${roleName} again from Start.`;
}

/** Where a seat started, in the words a screen reader and the icon's tooltip share. */
export function originWords(origin: Session["origin"]): string {
  if (origin === "window") return "started in a terminal window";
  if (origin === "headless") return "started with no window";
  return "";
}
