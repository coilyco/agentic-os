// A seat's unsent words outlive a closed app (COI-3533). Per device, by host and session.
// A reused name is retired for a day, so an older draft is dropped, not given to another
// seat. Images are not kept here.
const KEY = "aterm.drafts.v1";

export const KEEP_MS = 24 * 60 * 60 * 1000;
export const MAX_DRAFTS = 20;
/** A longer draft stays in memory only, so storage is never the reason a send fails. */
export const MAX_CHARS = 20000;

export interface Stored {
  [key: string]: { text: string; at: number };
}

export const draftKey = (hostId: string, sessionId: string): string => `${hostId}/${sessionId}`;

export function parseDrafts(text: string | null, now: number): Stored {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null) return {};
    const kept: Stored = {};
    for (const [key, value] of Object.entries(raw)) {
      const entry = value as { text?: unknown; at?: unknown };
      if (typeof entry?.text !== "string" || typeof entry.at !== "number") continue;
      if (!entry.text || now - entry.at > KEEP_MS) continue;
      kept[key] = { text: entry.text, at: entry.at };
    }
    return kept;
  } catch {
    return {};
  }
}

/** The store after a draft changes: empty removes it, the oldest go past the cap. */
export function withDraft(stored: Stored, key: string, text: string, now: number): Stored {
  const next: Stored = { ...stored };
  delete next[key];
  if (text && text.length <= MAX_CHARS) next[key] = { text, at: now };
  const keys = Object.keys(next).sort((a, b) => (next[b]?.at ?? 0) - (next[a]?.at ?? 0));
  for (const old of keys.slice(MAX_DRAFTS)) delete next[old];
  return next;
}

export function loadDraft(key: string, now = Date.now()): string {
  try {
    return parseDrafts(localStorage.getItem(KEY), now)[key]?.text ?? "";
  } catch {
    return "";
  }
}

export function storeDraft(key: string, text: string, now = Date.now()): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(withDraft(parseDrafts(localStorage.getItem(KEY), now), key, text, now)));
  } catch {
    // Storage can be off or full (a private window). The draft lasts till the page ends.
  }
}
