// Carrying an unlock across a redial. The daemon holds the assertion per connection
// and hands back a one-use proof (COI-3305). The proof is a bearer secret. It lives in
// the host object's memory, never in storage, a log, or an event.

/** `welcome.features` names this only when the daemon runs with a resume window. */
export const RESUME_FEATURE = "resume";

/** The one `error.reason` for a proof that failed: expired, spent, unknown, revoked. */
export const RESUME_ENDED_REASON = "resume_ended";

/** `resuming` while the proof is out, `ended` once the daemon says it no longer holds. */
export type ResumeState = "resuming" | "ended";

/** The proof an unlock reply carries, or undefined when the daemon sent none. */
export function resumeProof(frame: { resume?: unknown }): string | undefined {
  return typeof frame.resume === "string" && frame.resume !== "" ? frame.resume : undefined;
}

/** A refused proof. The connection stays locked, so the person unlocks as before. */
export class ResumeEndedError extends Error {
  constructor() {
    super("The unlock ended while this device was away.");
  }
}

/** What the lock panel says while the proof is out, or after it failed. */
export function resumeNotice(state: ResumeState): string {
  return state === "resuming"
    ? "Checking that your last unlock still holds."
    : "Your unlock ended while this device was away. Unlock again to type.";
}
