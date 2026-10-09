// What a person typed, kept where a lock, a seat change, or a remount cannot take it.

/** Input has no reply, so a lock this soon after Send is that send refused. */
export const REFUSAL_WINDOW_MS = 15_000;

/** A soft keyboard or paste menu blurs the page for a blink. Not an absence. */
export const AWAY_MS = 2_000;

/** The draft after a refusal: the sent text comes back ahead of what was typed since. */
export function restoreRefused(draft: string, sent: { body: string; at: number } | null, now: number): { draft: string; restored: boolean } {
  if (!sent || now - sent.at > REFUSAL_WINDOW_MS) return { draft, restored: false };
  return { draft: draft ? `${sent.body}\n${draft}` : sent.body, restored: true };
}

/** Whether returning should jump seats. Not under a draft, not for a blink. */
export function worthJumping(leftAt: number | null, now: number, hasDraft: boolean): boolean {
  return leftAt !== null && now - leftAt >= AWAY_MS && !hasDraft;
}
