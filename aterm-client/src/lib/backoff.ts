// How long the client keeps redialing a daemon that stopped answering. A phone client
// exists to stay attached, so an hour of tries is the job, not overkill (COI-3352).
export const RETRY_SPAN_MS = 3_600_000;
const FIRST_WAIT_MS = 1000;
// A restart of a few minutes must not cost a long wait after the daemon is back.
export const MAX_WAIT_MS = 300_000;

/** The waits between tries: 1 s doubling to the cap, the last trimmed to end the span. */
export const RETRY_WAITS_MS: readonly number[] = (() => {
  const waits: number[] = [];
  let spent = 0;
  for (let wait = FIRST_WAIT_MS; spent + wait < RETRY_SPAN_MS; wait = Math.min(MAX_WAIT_MS, wait * 2)) {
    waits.push(wait);
    spent += wait;
  }
  waits.push(RETRY_SPAN_MS - spent);
  return waits;
})();

/** Wait before redial `attempt`, or null once the hour is spent. Each wait is spread 20%
 * either way, never past the cap, so tabs that lost one daemon do not redial in step. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number | null {
  const base = RETRY_WAITS_MS[attempt];
  return base === undefined ? null : Math.min(MAX_WAIT_MS, Math.round(base * (0.8 + 0.4 * random())));
}

/** Whole seconds left until `at`, never below zero, for the countdown on screen. */
export function secondsUntil(at: number, now: number): number {
  return Math.max(0, Math.ceil((at - now) / 1000));
}

/** Whether the answering daemon is another build than the one that served this page.
 * Releases compare by number, so a rollback is not newer. Anything else counts. */
export function differentBuild(served: string | undefined, running: string | undefined): boolean {
  if (!served || !running || served === running) return false;
  const parts = (version: string) => version.replace(/^v/, "").split(".").map(Number);
  const [before, after] = [parts(served), parts(running)];
  if ([...before, ...after].some(Number.isNaN)) return true;
  for (let index = 0; index < Math.max(before.length, after.length); index++) {
    const step = (after[index] ?? 0) - (before[index] ?? 0);
    if (step !== 0) return step > 0;
  }
  return false;
}
