// How long the client waits before redialing a daemon that stopped answering.
export const BACKOFF_BASE_MS = 1000;
export const BACKOFF_CAP_MS = 30000;

/** Wait before redial `attempt`: 1, 2, 4, 8, 16 s, then 30 s, each spread 20% either way
 * so tabs that lost one daemon together do not redial in step. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(BACKOFF_CAP_MS, BACKOFF_BASE_MS * 2 ** attempt);
  return Math.min(BACKOFF_CAP_MS, Math.round(base * (0.8 + 0.4 * random())));
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
