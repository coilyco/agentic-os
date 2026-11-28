// Search in a seat's whole transcript (COI-3797). The host finds the matches, and this
// steps between them and marks them on screen.

import { wantsFind, type Chord } from "./terminal-extras";

/** Cmd+F, Ctrl+Shift+F, and plain Ctrl+F, as no shell listens under a transcript. */
export function wantsTranscriptFind(chord: Chord): boolean {
  if (wantsFind(chord)) return true;
  return chord.key.toLowerCase() === "f" && chord.ctrlKey && !chord.metaKey && !chord.altKey && !chord.shiftKey;
}

/** The match one step older or newer, wrapping. -1 when there are none. */
export function stepMatch(at: number, count: number, older: boolean): number {
  if (count <= 0) return -1;
  if (at < 0 || at >= count) return older ? count - 1 : 0;
  return (at + (older ? -1 : 1) + count) % count;
}

/** What the bar says. It counts from the newest match, where a search starts. */
export function searchStatusText(state: { query: string; phase: "idle" | "searching" | "done" | "failed"; at: number; count: number; total: number; partial: boolean }): string {
  if (!state.query.trim()) return "";
  if (state.phase === "searching") return "Searching";
  if (state.phase === "failed") return "Search failed";
  if (state.phase !== "done") return "";
  if (state.count === 0) return state.partial ? "No matches in the part read" : "No matches";
  const place = `${state.count - state.at} of ${state.total}`;
  return state.total > state.count ? `${place}, newest ${state.count} shown` : place;
}

/** Every case-insensitive occurrence of query in the text under root, as ranges. */
export function matchRanges(root: Node, query: string): Range[] {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle || typeof document === "undefined") return [];
  const ranges: Range[] = [];
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const text = (node.nodeValue ?? "").toLocaleLowerCase();
    // Lowering can change a length, and then an index would miss the original.
    if (text.length !== (node.nodeValue ?? "").length) continue;
    for (let at = text.indexOf(needle); at >= 0; at = text.indexOf(needle, at + needle.length)) {
      const range = document.createRange();
      range.setStart(node, at);
      range.setEnd(node, at + needle.length);
      ranges.push(range);
    }
  }
  return ranges;
}

interface HighlightRegistry {
  set(name: string, value: unknown): void;
  delete(name: string): void;
}

const registry = (): { registry: HighlightRegistry; make: (ranges: Range[]) => unknown } | null => {
  const css = (globalThis as { CSS?: { highlights?: HighlightRegistry } }).CSS;
  const Make = (globalThis as { Highlight?: new (...ranges: Range[]) => unknown }).Highlight;
  return css?.highlights && Make ? { registry: css.highlights, make: (ranges) => new Make(...ranges) } : null;
};

export const MATCH_HIGHLIGHT = "transcript-match";
export const CURRENT_HIGHLIGHT = "transcript-current";

/** Marks every match, the current one more strongly. No Highlight API, no marks. */
export function paintMatches(all: Range[], current: Range[]): void {
  const found = registry();
  if (!found) return;
  found.registry.set(MATCH_HIGHLIGHT, found.make(all));
  found.registry.set(CURRENT_HIGHLIGHT, found.make(current));
}

export function clearMatches(): void {
  const found = registry();
  found?.registry.delete(MATCH_HIGHLIGHT);
  found?.registry.delete(CURRENT_HIGHLIGHT);
}
