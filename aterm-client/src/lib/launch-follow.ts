// After a launch the client opens the seat that appears (COI-3480). `launched` names
// no session, so the roster is watched for a new one of the role.

/** Past this the new seat stays in the list and is not opened under a person. */
export const LAUNCH_WATCH_MS = 60_000;

export interface PendingLaunch {
  role: string;
  /** Every session listed when the launch began. A new seat is one not in here. */
  known: ReadonlySet<string>;
  since: number;
}

export function beginLaunch(role: string, listed: readonly { id: string }[], now: number): PendingLaunch {
  return { role, known: new Set(listed.map((each) => each.id)), since: now };
}

/** The first listed session of the role that was not there at the tap. */
export function newSeatFor(pending: PendingLaunch, listed: readonly { id: string; role: string; state: string }[], now: number): string | null {
  if (now - pending.since > LAUNCH_WATCH_MS) return null;
  // A failed session is the launch's failure report, which the Start panel shows.
  return listed.find((each) => each.role === pending.role && each.state !== "failed" && !pending.known.has(each.id))?.id ?? null;
}
