// Cues for a seat that starts waiting on you. The glow is on until switched off, sound
// stays off until switched on. Settings live in this browser only.

export interface AttentionSettings {
  sound: boolean;
  visual: boolean;
}

export const ATTENTION_DEFAULT: AttentionSettings = { sound: false, visual: true };

const KEY = "aterm.attention.v1";

/** Asks replay on subscribe as separate frames, so attach is not news. */
export const SETTLE_MS = 1500;

/** Stored settings. Missing or malformed reads as the default. A stored opt-out wins. */
export function parseAttention(raw: string | null): AttentionSettings {
  try {
    const parsed: unknown = JSON.parse(raw ?? "null");
    if (typeof parsed !== "object" || parsed === null) return ATTENTION_DEFAULT;
    const { sound, visual } = parsed as { sound?: unknown; visual?: unknown };
    return { sound: sound === true, visual: visual !== false };
  } catch {
    return ATTENTION_DEFAULT;
  }
}

export function loadAttention(): AttentionSettings {
  try {
    return parseAttention(localStorage.getItem(KEY));
  } catch {
    return ATTENTION_DEFAULT;
  }
}

export function storeAttention(settings: AttentionSettings): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(settings));
  } catch {
    // Storage can be off (a private window). The switches work until reload.
  }
}

/** One cue per moment seats enter waiting. `known` null is a baseline, no fire. */
export function detectCue(known: ReadonlySet<string> | null, waiting: readonly string[], open: string | null): { known: Set<string>; fire: boolean } {
  const now = new Set(waiting);
  if (known === null) return { known: now, fire: false };
  const fire = waiting.some((id) => !known.has(id) && id !== open);
  return { known: now, fire };
}
