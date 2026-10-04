// The context meter's words. The count shows for every harness, the percent only
// where the window is known.
import type { ContextReading } from "./protocol";

/** 842, 4.3k, 535k, 1.2M: short enough for a header pill and exact to two figures. */
export function formatTokens(tokens: number): string {
  if (tokens < 1000) return String(Math.max(0, Math.round(tokens)));
  if (tokens < 10_000) return `${(tokens / 1000).toFixed(1)}k`;
  if (tokens < 999_500) return `${Math.round(tokens / 1000)}k`;
  return `${(tokens / 1_000_000).toFixed(1)}M`;
}

/** Whole percent of the window, or null where the window is not known. */
export function contextPercent(reading: ContextReading): number | null {
  return reading.window ? Math.round((reading.tokens / reading.window) * 100) : null;
}

/** What the pill and its accessible name say. */
export function contextText(reading: ContextReading): string {
  const percent = contextPercent(reading);
  const count = `${formatTokens(reading.tokens)} tokens`;
  if (percent === null || !reading.window) return count;
  // A small count in a large window rounds to 0, which would read as empty.
  const share = percent === 0 && reading.tokens > 0 ? "<1%" : `${percent}%`;
  return `${count} // ${share} of ${formatTokens(reading.window)}`;
}
