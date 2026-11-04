// The small decisions behind links and find in a seat's terminal.
interface Chord {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  altKey: boolean;
}

/** Cmd+F, or Ctrl+Shift+F. Plain Ctrl+F is the shell's, so it passes through. */
export function wantsFind(chord: Chord): boolean {
  if (chord.key.toLowerCase() !== "f" || chord.altKey) return false;
  return (chord.metaKey && !chord.ctrlKey) || (chord.ctrlKey && chord.shiftKey);
}

/** The address to open, or null for anything but an http or https link. */
export function linkTarget(uri: string): string | null {
  try {
    const url = new URL(uri);
    return url.protocol === "https:" || url.protocol === "http:" ? url.href : null;
  } catch {
    return null;
  }
}

/** A mouse needs Cmd or Ctrl, so selecting text never opens a link. Touch has none. */
export function opensLink(click: { metaKey: boolean; ctrlKey: boolean }, touch: boolean): boolean {
  return touch || click.metaKey || click.ctrlKey;
}

/** What the find bar says about the matches for `query`. */
export function findCountText(query: string, results: { resultIndex: number; resultCount: number }): string {
  if (!query) return "";
  if (results.resultCount === 0) return "No matches";
  if (results.resultIndex < 0) return `${results.resultCount}+`;
  return `${results.resultIndex + 1} of ${results.resultCount}`;
}
