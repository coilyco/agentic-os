// How long a seat has been quiet, in the words a seat row has room for.
const MIN_SHOWN_S = 10;
const WORKING_QUIET_S = 30;

/** Seconds, minutes, hours or days, rounded down. Empty under ten seconds. */
export function ageText(seconds: number): string {
  if (seconds < MIN_SHOWN_S) return "";
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

/** When the seat last wrote: the daemon's reading backdated, or newer output seen. */
export function quietSince(quietSeconds: number | undefined, receivedAt: number, lastOutput: number | undefined): number | undefined {
  const reading = quietSeconds === undefined ? undefined : receivedAt - quietSeconds * 1000;
  if (lastOutput !== undefined && (reading === undefined || lastOutput > reading)) return lastOutput;
  return reading;
}

/** The row's state with its quiet time. A busy seat shows it only once it goes silent. */
export function stateWithAge(state: string, since: number | undefined, now: number): string {
  if (since === undefined) return state;
  const quiet = Math.max(0, (now - since) / 1000);
  if (state === "working") return quiet >= WORKING_QUIET_S ? `working, quiet ${ageText(quiet)}` : state;
  const age = ageText(quiet);
  return age ? `${state} ${age}` : state;
}
