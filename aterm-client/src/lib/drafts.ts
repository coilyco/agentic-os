// What a person typed, kept where a lock, a seat change, or a remount cannot take it.

/** Input has no reply, so a lock this soon after Send is that send refused. */
export const REFUSAL_WINDOW_MS = 15_000;

/** The draft after a refusal: the sent text comes back ahead of what was typed since. */
export function restoreRefused(draft: string, sent: { body: string; at: number } | null, now: number): { draft: string; restored: boolean } {
  if (!sent || now - sent.at > REFUSAL_WINDOW_MS) return { draft, restored: false };
  return { draft: draft ? `${sent.body}\n${draft}` : sent.body, restored: true };
}
